// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonboulle/clockwork"
)

const (
	testServiceAccountUserID  = "111111111111111111"
	testServiceAccountKeyID   = "222222222222222222"
	testServiceAccountProject = "333333333333333333"

	// defaultTokenPath is the token endpoint the fake provider advertises when
	// a test names none.
	defaultTokenPath = "/oauth/v2/token"

	// discoveryPath is the well-known path that [discover] reads.
	discoveryPath = "/.well-known/openid-configuration"
)

// serviceAccountServerConfig fixes the behaviour of a
// [serviceAccountServer]. The handler goroutines read every field, so a test
// sets them at construction and never writes them again.
type serviceAccountServerConfig struct {
	// tokenPath is the path that the discovery document advertises as the
	// token endpoint.
	tokenPath string
	// accessToken is the access_token that a successful exchange returns.
	accessToken string
	// expiresIn is the expires_in that a successful exchange returns. A zero
	// value omits the field.
	expiresIn int64
	// status, when it is not zero and not 200, is the status that the token
	// endpoint returns instead of a success.
	status int
	// errorBody is the body that accompanies status.
	errorBody string
	// echoAssertion appends the posted assertion to errorBody, as a provider
	// that echoes its request parameters would.
	echoAssertion bool
	// redirectTokenTo, when it is set, makes the token endpoint answer with a
	// 307 to that URL, as a misconfigured reverse proxy or an open redirect on
	// the provider would. The discovery document still advertises the https
	// endpoint.
	redirectTokenTo string
	// blockToken, when it is set, holds the token endpoint until the channel
	// is closed. A test uses it to keep an exchange in flight.
	blockToken <-chan struct{}
	// tokenReached, when it is set, is closed once the token endpoint has
	// been reached, before blockToken holds it. A test uses it to know that
	// an exchange is in flight.
	tokenReached chan struct{}
}

// serviceAccountServer is a fake Zitadel that serves a discovery document and
// an RFC 7523 token endpoint.
type serviceAccountServer struct {
	*httptest.Server

	cfg serviceAccountServerConfig
	// tokenRequests counts the requests that reach the token endpoint.
	tokenRequests atomic.Int32
	// reachedOnce closes cfg.tokenReached for the first request only.
	reachedOnce sync.Once

	mu sync.Mutex
	// forms holds the form of every token request, in order.
	forms []url.Values
	// paths holds the path of every token request, in order.
	paths []string
}

func newServiceAccountServer(t *testing.T, cfg serviceAccountServerConfig) *serviceAccountServer {
	t.Helper()

	if cfg.tokenPath == "" {
		cfg.tokenPath = defaultTokenPath
	}
	sas := &serviceAccountServer{cfg: cfg}

	mux := http.NewServeMux()
	sas.Server = httptest.NewTLSServer(mux)
	t.Cleanup(sas.Server.Close)

	mux.HandleFunc(discoveryPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"issuer": %q, "token_endpoint": "%s%s"}`, sas.URL, sas.URL, cfg.tokenPath)
	})
	mux.HandleFunc(cfg.tokenPath, sas.serveToken)
	return sas
}

func (sas *serviceAccountServer) serveToken(w http.ResponseWriter, r *http.Request) {
	sas.tokenRequests.Add(1)
	if sas.cfg.tokenReached != nil {
		sas.reachedOnce.Do(func() { close(sas.cfg.tokenReached) })
	}
	if sas.cfg.blockToken != nil {
		<-sas.cfg.blockToken
	}
	if sas.cfg.redirectTokenTo != "" {
		http.Redirect(w, r, sas.cfg.redirectTokenTo, http.StatusTemporaryRedirect)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	sas.mu.Lock()
	sas.forms = append(sas.forms, r.PostForm)
	sas.paths = append(sas.paths, r.URL.Path)
	sas.mu.Unlock()

	if sas.cfg.status != 0 && sas.cfg.status != http.StatusOK {
		body := sas.cfg.errorBody
		if sas.cfg.echoAssertion {
			body += r.PostFormValue("assertion")
		}
		w.WriteHeader(sas.cfg.status)
		fmt.Fprint(w, body)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if sas.cfg.expiresIn == 0 {
		fmt.Fprintf(w, `{"access_token": %q, "token_type": "Bearer"}`, sas.cfg.accessToken)
		return
	}
	fmt.Fprintf(w, `{"access_token": %q, "token_type": "Bearer", "expires_in": %d}`,
		sas.cfg.accessToken, sas.cfg.expiresIn)
}

// form returns the form of the i-th token request.
func (sas *serviceAccountServer) form(t *testing.T, i int) url.Values {
	t.Helper()

	sas.mu.Lock()
	defer sas.mu.Unlock()
	if i >= len(sas.forms) {
		t.Fatalf("the token endpoint saw %d requests, but request %d was asked for", len(sas.forms), i)
	}
	return sas.forms[i]
}

// path returns the path of the i-th token request.
func (sas *serviceAccountServer) path(t *testing.T, i int) string {
	t.Helper()

	sas.mu.Lock()
	defer sas.mu.Unlock()
	if i >= len(sas.paths) {
		t.Fatalf("the token endpoint saw %d requests, but request %d was asked for", len(sas.paths), i)
	}
	return sas.paths[i]
}

// testServiceAccountConfig points a service account config at the fake
// Zitadel.
func testServiceAccountConfig(sas *serviceAccountServer, clock clockwork.Clock) OIDCServiceAccountConfig {
	return OIDCServiceAccountConfig{
		Clock:                 clock,
		Issuer:                sas.URL,
		ProjectID:             testServiceAccountProject,
		KeyID:                 testServiceAccountKeyID,
		UserID:                testServiceAccountUserID,
		PrivateKey:            bytes.NewBuffer(testKey.privatePEM),
		HTTPClient:            sas.Client(),
		SkipTransportSecurity: true,
	}
}

// assertionClaims parses the assertion of the i-th token request without
// verifying its signature, and returns the parsed token.
func assertionClaims(t *testing.T, sas *serviceAccountServer, i int) (*jwt.Token, jwt.MapClaims) {
	t.Helper()

	assertion := sas.form(t, i).Get("assertion")
	if assertion == "" {
		t.Fatalf("token request %d carries no assertion field", i)
	}
	claims := jwt.MapClaims{}
	token, _, err := jwt.NewParser().ParseUnverified(assertion, claims)
	if err != nil {
		t.Fatalf("parsing the assertion of token request %d: %v", i, err)
	}
	return token, claims
}

func TestServiceAccountAssertionShape(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	accessToken := testIDToken(t, "", clock.Now().Add(12*time.Hour))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		accessToken: accessToken,
		expiresIn:   43200,
	})

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), testServiceAccountConfig(sas, clock))
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	md, err := requestMetadata(context.Background(), creds)
	if err != nil {
		t.Fatalf("GetRequestMetadata returned an unexpected error: %v", err)
	}
	if got, want := md[authorizationHeader], "Bearer "+accessToken; got != want {
		t.Errorf("authorization header: got %q, want %q", got, want)
	}

	token, claims := assertionClaims(t, sas, 0)
	if got := token.Header["alg"]; got != "RS256" {
		t.Errorf("assertion alg: got %v, want RS256", got)
	}
	if got := token.Header["kid"]; got != testServiceAccountKeyID {
		t.Errorf("assertion kid: got %v, want %q", got, testServiceAccountKeyID)
	}
	if got := claims["iss"]; got != testServiceAccountUserID {
		t.Errorf("assertion iss: got %v, want %q", got, testServiceAccountUserID)
	}
	if got := claims["sub"]; got != testServiceAccountUserID {
		t.Errorf("assertion sub: got %v, want %q", got, testServiceAccountUserID)
	}
	if got := claims["aud"]; got != strings.TrimSuffix(sas.URL, "/") {
		t.Errorf("assertion aud: got %v, want the issuer %q", got, sas.URL)
	}

	issuedAt, err := claims.GetIssuedAt()
	if err != nil || issuedAt == nil {
		t.Fatalf("reading the iat claim: %v", err)
	}
	if want := clock.Now().Add(-30 * time.Second); !issuedAt.Time.Equal(want) {
		t.Errorf("assertion iat: got %v, want %v", issuedAt.Time, want)
	}
	expiresAt, err := claims.GetExpirationTime()
	if err != nil || expiresAt == nil {
		t.Fatalf("reading the exp claim: %v", err)
	}
	if want := clock.Now().Add(300 * time.Second); !expiresAt.Time.Equal(want) {
		t.Errorf("assertion exp: got %v, want %v", expiresAt.Time, want)
	}
	if got, _ := claims["jti"].(string); got == "" {
		t.Error("the assertion carries no jti claim")
	}

	form := sas.form(t, 0)
	if got, want := form.Get("grant_type"), "urn:ietf:params:oauth:grant-type:jwt-bearer"; got != want {
		t.Errorf("grant_type: got %q, want %q", got, want)
	}
	wantScopes := []string{
		"openid",
		"urn:zitadel:iam:org:project:id:" + testServiceAccountProject + ":aud",
		"urn:zitadel:iam:org:projects:roles",
	}
	gotScopes := strings.Split(form.Get("scope"), " ")
	if len(gotScopes) != len(wantScopes) {
		t.Fatalf("scope: got %q, want %q", form.Get("scope"), strings.Join(wantScopes, " "))
	}
	for i, want := range wantScopes {
		if gotScopes[i] != want {
			t.Errorf("scope %d: got %q, want %q", i, gotScopes[i], want)
		}
	}
	if form.Has("client_id") {
		t.Errorf("the form carries a client_id %q; the jwt-bearer grant has none", form.Get("client_id"))
	}
	if form.Has("client_assertion") {
		t.Error("the form carries a client_assertion; the jwt-bearer grant sends the assertion field")
	}
}

func TestServiceAccountUsesDiscoveredTokenEndpoint(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	const tokenPath = "/not/the/default/token/path"
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		tokenPath:   tokenPath,
		accessToken: testIDToken(t, "", clock.Now().Add(12*time.Hour)),
		expiresIn:   43200,
	})

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), testServiceAccountConfig(sas, clock))
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}
	if _, err := requestMetadata(context.Background(), creds); err != nil {
		t.Fatalf("GetRequestMetadata returned an unexpected error: %v", err)
	}

	if got := sas.path(t, 0); got != tokenPath {
		t.Errorf("the exchange hit %q, but the discovery document advertises %q", got, tokenPath)
	}
}

func TestServiceAccountRefusesAnOpaqueAccessToken(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		accessToken string
	}{
		{name: "opaque", accessToken: "opaque-not-a-jwt"},
		{name: "two segments", accessToken: "aGVhZGVy.cGF5bG9hZA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
			sas := newServiceAccountServer(t, serviceAccountServerConfig{
				accessToken: tc.accessToken,
				expiresIn:   43200,
			})

			creds, err := NewOIDCServiceAccountCredentials(context.Background(), testServiceAccountConfig(sas, clock))
			if err != nil {
				t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
			}

			_, err = requestMetadata(context.Background(), creds)
			if err == nil {
				t.Fatal("GetRequestMetadata accepted an access token that is not a JWT")
			}
			if !strings.Contains(err.Error(), "Access Token Type") {
				t.Errorf("error %q does not name the service account's Access Token Type setting", err.Error())
			}
			if !strings.Contains(err.Error(), "JWT") {
				t.Errorf("error %q does not name the JWT value of that setting", err.Error())
			}
		})
	}
}

func TestServiceAccountErrorDoesNotEchoTheAssertion(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		status:        http.StatusBadRequest,
		errorBody:     strings.Repeat("p", 4096),
		echoAssertion: true,
	})

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), testServiceAccountConfig(sas, clock))
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	_, err = requestMetadata(context.Background(), creds)
	if err == nil {
		t.Fatal("GetRequestMetadata succeeded against a token endpoint that returns HTTP 400")
	}

	_, claims := assertionClaims(t, sas, 0)
	if claims["jti"] == nil {
		t.Fatal("the token endpoint captured no assertion")
	}
	assertion := sas.form(t, 0).Get("assertion")
	if strings.Contains(err.Error(), assertion) {
		t.Error("the error echoes the assertion, which puts a credential on a log line")
	}
	if !strings.Contains(err.Error(), "... (truncated)") {
		t.Errorf("error %q does not say that the provider body was truncated", err.Error())
	}
}

func TestServiceAccountWrapsTheOpaqueGrantError(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		status:    http.StatusBadRequest,
		errorBody: `{"error":"invalid_grant","error_description":"invalid assertion"}`,
	})

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), testServiceAccountConfig(sas, clock))
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	_, err = requestMetadata(context.Background(), creds)
	if err == nil {
		t.Fatal("GetRequestMetadata succeeded against a token endpoint that returns invalid_grant")
	}

	// Zitadel reports every one of these causes as invalid_grant, so the
	// error must list all four.
	for _, want := range []string{"clock", "private key", "UserID", "1 hour"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name the possible cause %q", err.Error(), want)
		}
	}
}

func TestServiceAccountRefreshesAStaleToken(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		accessToken: testIDToken(t, "", clock.Now().Add(24*time.Hour)),
		expiresIn:   43200,
	})

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), testServiceAccountConfig(sas, clock))
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	if _, err := requestMetadata(context.Background(), creds); err != nil {
		t.Fatalf("the first GetRequestMetadata returned an unexpected error: %v", err)
	}
	if got := sas.tokenRequests.Load(); got != 1 {
		t.Fatalf("the token endpoint saw %d requests after the first call, want 1", got)
	}

	// A second call while the token is fresh must not exchange again.
	if _, err := requestMetadata(context.Background(), creds); err != nil {
		t.Fatalf("the cached GetRequestMetadata returned an unexpected error: %v", err)
	}
	if got := sas.tokenRequests.Load(); got != 1 {
		t.Fatalf("the token endpoint saw %d requests while the token was fresh, want 1", got)
	}

	// Move past expiresAt - tokenExpirationWindow.
	clock.Advance(12*time.Hour - tokenExpirationWindow + time.Minute)
	if _, err := requestMetadata(context.Background(), creds); err != nil {
		t.Fatalf("the refreshing GetRequestMetadata returned an unexpected error: %v", err)
	}
	if got := sas.tokenRequests.Load(); got != 2 {
		t.Fatalf("the token endpoint saw %d requests after the token went stale, want 2", got)
	}

	_, first := assertionClaims(t, sas, 0)
	_, second := assertionClaims(t, sas, 1)
	if first["iat"] == second["iat"] {
		t.Errorf("both assertions carry iat %v; the second must be signed afresh", first["iat"])
	}
	if first["jti"] == second["jti"] {
		t.Errorf("both assertions carry jti %v; the second must be signed afresh", first["jti"])
	}
}

// useDefaultHTTPClient points [http.DefaultClient] at the fake identity
// provider for the rest of the test, and restores it afterwards. The
// nil-pointer path falls back to [http.DefaultClient], and the fake provider
// serves TLS with a certificate that the process-wide default client does not
// trust.
//
// A test that calls this must not call t.Parallel. The testing package runs
// every sequential test while every parallel test is paused, so no other test
// body observes the swap.
func useDefaultHTTPClient(t *testing.T, client *http.Client) {
	t.Helper()

	saved := http.DefaultClient
	http.DefaultClient = client
	t.Cleanup(func() { http.DefaultClient = saved })
}

// TestServiceAccountNilHTTPClientPointer makes sure that a nil *http.Client
// never reaches the token exchange. HTTPClient is an interface, so a nil
// *http.Client stored in it is not a nil interface, and a plain nil check
// takes it for a client that the caller injected. The first Do call on it
// panics, which is what nbictl did in production.
func TestServiceAccountNilHTTPClientPointer(t *testing.T) {
	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	accessToken := testIDToken(t, "", clock.Now().Add(12*time.Hour))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		accessToken: accessToken,
		expiresIn:   43200,
	})
	useDefaultHTTPClient(t, sas.Client())

	c := testServiceAccountConfig(sas, clock)
	c.HTTPClient = (*http.Client)(nil)

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), c)
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	md, err := requestMetadata(context.Background(), creds)
	if err != nil {
		t.Fatalf("GetRequestMetadata returned an unexpected error: %v", err)
	}
	if got, want := md[authorizationHeader], "Bearer "+accessToken; got != want {
		t.Errorf("authorization header: got %q, want %q", got, want)
	}
}

func TestNewOIDCServiceAccountCredentials_Validation(t *testing.T) {
	t.Parallel()

	valid := func() OIDCServiceAccountConfig {
		return OIDCServiceAccountConfig{
			Clock:      clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)),
			Issuer:     "https://zitadel.example.com",
			ProjectID:  testServiceAccountProject,
			KeyID:      testServiceAccountKeyID,
			UserID:     testServiceAccountUserID,
			PrivateKey: bytes.NewBuffer(testKey.privatePEM),
		}
	}

	for _, tc := range []struct {
		name   string
		mutate func(*OIDCServiceAccountConfig)
		want   []string
	}{
		{
			name:   "missing clock",
			mutate: func(c *OIDCServiceAccountConfig) { c.Clock = nil },
			want:   []string{"missing required field 'Clock'"},
		},
		{
			name:   "missing issuer",
			mutate: func(c *OIDCServiceAccountConfig) { c.Issuer = "" },
			want:   []string{"missing required field 'Issuer'"},
		},
		{
			name:   "missing project id",
			mutate: func(c *OIDCServiceAccountConfig) { c.ProjectID = "" },
			want:   []string{"missing required field 'ProjectID'"},
		},
		{
			name:   "missing key id",
			mutate: func(c *OIDCServiceAccountConfig) { c.KeyID = "" },
			want:   []string{"missing required field 'KeyID'"},
		},
		{
			name:   "missing user id",
			mutate: func(c *OIDCServiceAccountConfig) { c.UserID = "" },
			want:   []string{"missing required field 'UserID'"},
		},
		{
			name:   "missing private key",
			mutate: func(c *OIDCServiceAccountConfig) { c.PrivateKey = nil },
			want:   []string{"missing required field 'PrivateKey'"},
		},
		{
			// One round trip must report every missing field, not the
			// first one.
			name: "four missing fields at once",
			mutate: func(c *OIDCServiceAccountConfig) {
				c.Clock = nil
				c.Issuer = ""
				c.UserID = ""
				c.PrivateKey = nil
			},
			want: []string{
				"missing required field 'Clock'",
				"missing required field 'Issuer'",
				"missing required field 'UserID'",
				"missing required field 'PrivateKey'",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := valid()
			tc.mutate(&c)
			_, err := NewOIDCServiceAccountCredentials(context.Background(), c)
			if err == nil {
				t.Fatalf("NewOIDCServiceAccountCredentials accepted the config %+v", c)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not report %q", err.Error(), want)
				}
			}
		})
	}
}

func TestServiceAccountRequireTransportSecurity(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		accessToken: testIDToken(t, "", clock.Now().Add(12*time.Hour)),
		expiresIn:   43200,
	})

	c := testServiceAccountConfig(sas, clock)
	c.SkipTransportSecurity = false
	creds, err := NewOIDCServiceAccountCredentials(context.Background(), c)
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}
	if !creds.RequireTransportSecurity() {
		t.Error("RequireTransportSecurity returned false while SkipTransportSecurity is false")
	}

	c = testServiceAccountConfig(sas, clock)
	creds, err = NewOIDCServiceAccountCredentials(context.Background(), c)
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}
	if creds.RequireTransportSecurity() {
		t.Error("RequireTransportSecurity returned true while SkipTransportSecurity is true")
	}
}

// TestServiceAccountRefusesARedirect pins the transport of the assertion. Go
// replays a POST body on a 307 or a 308 and permits an https to http
// downgrade, so following a redirect would put a complete, signed, replayable
// assertion on the wire in cleartext. The discover function refuses a
// token_endpoint that is not https, and a followed redirect would void that
// check.
func TestServiceAccountRefusesARedirect(t *testing.T) {
	t.Parallel()

	var sinkRequests atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sinkRequests.Add(1)
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token": "a.b.c", "token_type": "Bearer"}`)
	}))
	t.Cleanup(sink.Close)

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{redirectTokenTo: sink.URL})

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), testServiceAccountConfig(sas, clock))
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	md, err := requestMetadata(context.Background(), creds)
	if err == nil {
		t.Fatalf("GetRequestMetadata followed a redirect and returned %v", md)
	}
	if n := sinkRequests.Load(); n != 0 {
		t.Errorf("the redirect target received %d requests, so the assertion left the identity provider", n)
	}
}

// TestServiceAccountClampsExpiresIn pins the cache against a provider that
// reports a lifetime large enough to overflow a time.Duration. The product
// wraps negative, which puts the expiry in the past, makes every cached token
// stale on arrival, and re-exchanges on every RPC.
func TestServiceAccountClampsExpiresIn(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		accessToken: testIDToken(t, "", clock.Now().Add(12*time.Hour)),
		expiresIn:   9223372037,
	})

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), testServiceAccountConfig(sas, clock))
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	for i := range 2 {
		if _, err := requestMetadata(context.Background(), creds); err != nil {
			t.Fatalf("GetRequestMetadata %d returned an unexpected error: %v", i, err)
		}
	}
	if n := sas.tokenRequests.Load(); n != 1 {
		t.Errorf("the token endpoint saw %d requests for 2 calls, want 1; the cached token is stale on arrival", n)
	}
}

// TestServiceAccountExchangeIgnoresCallerCancellation pins the context that
// the exchange runs under. One exchange serves every concurrent caller, and at
// cold start every RPC misses the cache at once. If the exchange inherited the
// first caller's context, that one caller giving up would abort the exchange
// and fail all the others with a misleading "context canceled".
//
// The cancellation lands while the exchange is in flight, and the exchange
// then has to finish and fill the cache. A second call that needs no second
// round trip is what proves it did.
func TestServiceAccountExchangeIgnoresCallerCancellation(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	reached := make(chan struct{})
	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	accessToken := testIDToken(t, "", clock.Now().Add(12*time.Hour))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		accessToken:  accessToken,
		expiresIn:    43200,
		blockToken:   release,
		tokenReached: reached,
	})

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), testServiceAccountConfig(sas, clock))
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderReturned := make(chan struct{})
	go func() {
		defer close(leaderReturned)
		// The leader's own result does not matter: it asked to stop.
		_, _ = requestMetadata(leaderCtx, creds)
	}()

	<-reached
	cancelLeader()
	<-leaderReturned
	close(release)

	md, err := requestMetadata(context.Background(), creds)
	if err != nil {
		t.Fatalf("the exchange the leader started did not survive its cancellation: %v", err)
	}
	if got, want := md[authorizationHeader], "Bearer "+accessToken; got != want {
		t.Errorf("authorization header: got %q, want %q", got, want)
	}
	if n := sas.tokenRequests.Load(); n != 1 {
		t.Errorf("the token endpoint saw %d requests, want 1; the leader's cancellation threw away the exchange", n)
	}
}

// TestServiceAccountHonoursTheCallerDeadline is the other half of
// TestServiceAccountExchangeIgnoresCallerCancellation. Detaching the exchange
// must not detach the caller from it: a caller that gives up has to return,
// and only the exchange carries on in the background for whoever still wants
// it. Without this, one unresponsive provider holds every caller for the whole
// exchange timeout.
func TestServiceAccountHonoursTheCallerDeadline(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		accessToken: testIDToken(t, "", clock.Now().Add(12*time.Hour)),
		expiresIn:   43200,
		blockToken:  release,
	})
	// Registered after the server, so that it runs before the server's own
	// cleanup: closing a server whose handler is still blocked hangs.
	t.Cleanup(func() { close(release) })

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), testServiceAccountConfig(sas, clock))
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	returned := make(chan error, 1)
	go func() {
		_, err := requestMetadata(ctx, creds)
		returned <- err
	}()

	select {
	case err := <-returned:
		if err == nil {
			t.Error("GetRequestMetadata returned no error although its caller's deadline passed")
		}
	case <-time.After(10 * time.Second):
		// The exchange timeout is 30 seconds, so a caller still waiting here
		// is waiting on the exchange and not on its own deadline.
		t.Fatal("GetRequestMetadata outlived its caller's deadline by ten seconds")
	}
}

// fakeServiceAccountTokenStore is an in-memory [ServiceAccountTokenStore]
// whose faults a test turns on. Every method takes the lock, because the
// concurrency test calls them from several goroutines at once.
type fakeServiceAccountTokenStore struct {
	mu      sync.Mutex
	token   *ServiceAccountToken
	loads   int
	saves   int
	loadErr error
	saveErr error
}

func (s *fakeServiceAccountTokenStore) Load() (*ServiceAccountToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads++
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.token, nil
}

func (s *fakeServiceAccountTokenStore) Save(token *ServiceAccountToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.token = token
	return nil
}

func (s *fakeServiceAccountTokenStore) held() *ServiceAccountToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

func (s *fakeServiceAccountTokenStore) saveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saves
}

// TestServiceAccountUsesTheCachedToken is the whole point of the cache: a
// second nbictl invocation inside the token's life reaches no network at all.
// The deletion test is that removing the store lookup from getToken makes the
// token endpoint see one request instead of none.
func TestServiceAccountUsesTheCachedToken(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	cached := testIDToken(t, "", clock.Now().Add(12*time.Hour))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		accessToken: testIDToken(t, "", clock.Now().Add(12*time.Hour)),
		expiresIn:   43200,
	})

	store := &fakeServiceAccountTokenStore{
		token: &ServiceAccountToken{AccessToken: cached, ExpiresAt: clock.Now().Add(12 * time.Hour)},
	}
	cfg := testServiceAccountConfig(sas, clock)
	cfg.TokenStore = store

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	md, err := requestMetadata(context.Background(), creds)
	if err != nil {
		t.Fatalf("GetRequestMetadata returned an unexpected error: %v", err)
	}
	if got, want := md[authorizationHeader], "Bearer "+cached; got != want {
		t.Errorf("authorization header: got %q, want %q", got, want)
	}
	if n := sas.tokenRequests.Load(); n != 0 {
		t.Errorf("the token endpoint saw %d requests although a fresh token was cached", n)
	}
	if n := store.saveCount(); n != 0 {
		t.Errorf("the store was written %d times although nothing was exchanged", n)
	}
}

// TestServiceAccountReplacesAStaleCachedToken pins the staleness rule. A
// cached token inside tokenExpirationWindow must not be sent.
func TestServiceAccountReplacesAStaleCachedToken(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	fresh := testIDToken(t, "", clock.Now().Add(12*time.Hour))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		accessToken: fresh,
		expiresIn:   43200,
	})

	// One minute of life left, which is inside the five minute window.
	store := &fakeServiceAccountTokenStore{
		token: &ServiceAccountToken{AccessToken: "stale.token.value", ExpiresAt: clock.Now().Add(time.Minute)},
	}
	cfg := testServiceAccountConfig(sas, clock)
	cfg.TokenStore = store

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	md, err := requestMetadata(context.Background(), creds)
	if err != nil {
		t.Fatalf("GetRequestMetadata returned an unexpected error: %v", err)
	}
	if got, want := md[authorizationHeader], "Bearer "+fresh; got != want {
		t.Errorf("authorization header: got %q, want %q", got, want)
	}
	if n := sas.tokenRequests.Load(); n != 1 {
		t.Errorf("the token endpoint saw %d requests, want 1", n)
	}
	held := store.held()
	if held == nil || held.AccessToken != fresh {
		t.Errorf("the store holds %+v, want the freshly exchanged token", held)
	}
}

// TestServiceAccountSurvivesAStoreFailure pins that the cache is an
// optimization. A broken cache costs a round trip and nothing else, because
// failing the RPC would make a bad cache file worse than no cache at all.
func TestServiceAccountSurvivesAStoreFailure(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		store *fakeServiceAccountTokenStore
	}{
		{name: "load fails", store: &fakeServiceAccountTokenStore{loadErr: errors.New("unreadable cache")}},
		{name: "save fails", store: &fakeServiceAccountTokenStore{saveErr: errors.New("read-only disk")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
			fresh := testIDToken(t, "", clock.Now().Add(12*time.Hour))
			sas := newServiceAccountServer(t, serviceAccountServerConfig{
				accessToken: fresh,
				expiresIn:   43200,
			})

			cfg := testServiceAccountConfig(sas, clock)
			cfg.TokenStore = tc.store

			creds, err := NewOIDCServiceAccountCredentials(context.Background(), cfg)
			if err != nil {
				t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
			}

			md, err := requestMetadata(context.Background(), creds)
			if err != nil {
				t.Fatalf("GetRequestMetadata failed although only the cache was broken: %v", err)
			}
			if got, want := md[authorizationHeader], "Bearer "+fresh; got != want {
				t.Errorf("authorization header: got %q, want %q", got, want)
			}
		})
	}
}

// TestServiceAccountCollapsesConcurrentExchangesWithAStore pins that adding
// the store did not defeat the singleflight. Without the collapse, every
// caller that misses the cache exchanges and writes its own token.
func TestServiceAccountCollapsesConcurrentExchangesWithAStore(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	reached := make(chan struct{})
	release := make(chan struct{})
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		accessToken:  testIDToken(t, "", clock.Now().Add(12*time.Hour)),
		expiresIn:    43200,
		tokenReached: reached,
		blockToken:   release,
	})

	store := &fakeServiceAccountTokenStore{}
	cfg := testServiceAccountConfig(sas, clock)
	cfg.TokenStore = store

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}

	const callers = 8
	errs := make(chan error, callers)
	for range callers {
		go func() {
			_, err := requestMetadata(context.Background(), creds)
			errs <- err
		}()
	}

	// Let the callers pile up on the one in-flight exchange before it answers.
	<-reached
	close(release)

	for range callers {
		if err := <-errs; err != nil {
			t.Errorf("GetRequestMetadata returned an unexpected error: %v", err)
		}
	}
	if n := sas.tokenRequests.Load(); n != 1 {
		t.Errorf("the token endpoint saw %d requests, want 1", n)
	}
	if n := store.saveCount(); n != 1 {
		t.Errorf("the store was written %d times, want 1", n)
	}
}

// countingDoer is an [HTTPDoer] that is deliberately not an [http.Client], so
// that a test can prove which requests reached the injected client and which
// escaped to the process-wide default.
type countingDoer struct {
	inner *http.Client
	mu    sync.Mutex
	paths []string
}

func (d *countingDoer) Do(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	d.paths = append(d.paths, req.URL.Path)
	d.mu.Unlock()
	return d.inner.Do(req)
}

func (d *countingDoer) seen() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.paths)
}

// TestServiceAccountDiscoveryUsesTheInjectedDoer pins that every request goes
// through the configured client. HTTPClient is an [HTTPDoer], so a caller may
// inject something that is not an [http.Client]: a fake in a test, or a Doer
// that pins a certificate or carries mTLS. If discovery reaches past it to
// [http.DefaultClient], that caller gets a mocked exchange and a real network
// call, and a pinning Doer is silently bypassed.
func TestServiceAccountDiscoveryUsesTheInjectedDoer(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	sas := newServiceAccountServer(t, serviceAccountServerConfig{
		accessToken: testIDToken(t, "", clock.Now().Add(12*time.Hour)),
		expiresIn:   43200,
	})

	doer := &countingDoer{inner: sas.Client()}
	cfg := testServiceAccountConfig(sas, clock)
	cfg.HTTPClient = doer

	creds, err := NewOIDCServiceAccountCredentials(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewOIDCServiceAccountCredentials returned an unexpected error: %v", err)
	}
	if _, err := requestMetadata(context.Background(), creds); err != nil {
		t.Fatalf("GetRequestMetadata returned an unexpected error: %v", err)
	}

	seen := doer.seen()
	if !slices.Contains(seen, discoveryPath) {
		t.Errorf("the discovery request did not go through the injected client; it saw %v", seen)
	}
	if !slices.Contains(seen, defaultTokenPath) {
		t.Errorf("the token request did not go through the injected client; it saw %v", seen)
	}
}
