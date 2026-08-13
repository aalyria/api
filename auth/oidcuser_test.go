// Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonboulle/clockwork"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/credentials"
)

// newIssuerServer starts a TLS test server that serves the OIDC discovery
// document that docFn builds from the server's own URL.
func newIssuerServer(t *testing.T, docFn func(issuer string) string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, docFn(srv.URL))
	})
	return srv
}

// fullDiscoveryDoc is a discovery document with every endpoint we read.
func fullDiscoveryDoc(issuer string) string {
	return fmt.Sprintf(`{
		"issuer": %q,
		"device_authorization_endpoint": "%s/oauth/v2/device_authorization",
		"token_endpoint": "%s/oauth/v2/token",
		"revocation_endpoint": "%s/oauth/v2/revoke"
	}`, issuer, issuer, issuer, issuer)
}

func TestDiscover_Success(t *testing.T) {
	t.Parallel()

	srv := newIssuerServer(t, fullDiscoveryDoc)

	got, err := Discover(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("Discover returned an unexpected error: %v", err)
	}
	if got.Issuer != srv.URL {
		t.Errorf("Issuer: got %q, want %q", got.Issuer, srv.URL)
	}
	if want := srv.URL + "/oauth/v2/device_authorization"; got.DeviceAuthorizationEndpoint != want {
		t.Errorf("DeviceAuthorizationEndpoint: got %q, want %q", got.DeviceAuthorizationEndpoint, want)
	}
	if want := srv.URL + "/oauth/v2/token"; got.TokenEndpoint != want {
		t.Errorf("TokenEndpoint: got %q, want %q", got.TokenEndpoint, want)
	}
	if want := srv.URL + "/oauth/v2/revoke"; got.RevocationEndpoint != want {
		t.Errorf("RevocationEndpoint: got %q, want %q", got.RevocationEndpoint, want)
	}
}

func TestDiscover_NoRevocationEndpoint(t *testing.T) {
	t.Parallel()

	srv := newIssuerServer(t, func(issuer string) string {
		return fmt.Sprintf(`{"issuer": %q, "token_endpoint": "%s/token"}`, issuer, issuer)
	})

	got, err := Discover(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("Discover returned an unexpected error: %v", err)
	}
	if got.RevocationEndpoint != "" {
		t.Errorf("RevocationEndpoint: got %q, want an empty string", got.RevocationEndpoint)
	}
}

func TestDiscover_Errors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		docFn    func(issuer string) string
		wantPart string
	}{
		{
			name: "issuer mismatch",
			docFn: func(issuer string) string {
				return fmt.Sprintf(`{"issuer": "https://impostor.example.com", "token_endpoint": "%s/token"}`, issuer)
			},
			wantPart: "does not match the configured issuer",
		},
		{
			name: "no token endpoint",
			docFn: func(issuer string) string {
				return fmt.Sprintf(`{"issuer": %q}`, issuer)
			},
			wantPart: "no token_endpoint",
		},
		{
			name:     "malformed body",
			docFn:    func(string) string { return "{not json" },
			wantPart: "parsing the discovery document",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newIssuerServer(t, tc.docFn)

			_, err := Discover(context.Background(), srv.URL, srv.Client())
			if err == nil {
				t.Fatalf("Discover succeeded, but an error that contains %q was expected", tc.wantPart)
			}
			if !strings.Contains(err.Error(), tc.wantPart) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantPart)
			}
		})
	}
}

func TestDiscover_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	_, err := Discover(context.Background(), srv.URL, srv.Client())
	if err == nil {
		t.Fatal("Discover succeeded against a server that returns HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not name the status code", err.Error())
	}
}

func TestDiscover_RejectsNonHTTPSIssuer(t *testing.T) {
	t.Parallel()

	_, err := Discover(context.Background(), "http://zitadel.example.com", nil)
	if err == nil {
		t.Fatal("Discover accepted an http:// issuer")
	}
	if !strings.Contains(err.Error(), "must start with https://") {
		t.Errorf("error %q does not explain the https requirement", err.Error())
	}
}

func TestDiscover_RejectsNonHTTPSEndpoints(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		docFn    func(issuer string) string
		wantPart string
	}{
		{
			name: "token endpoint",
			docFn: func(issuer string) string {
				return fmt.Sprintf(`{"issuer": %q, "token_endpoint": "http://insecure.example.com/token"}`, issuer)
			},
			wantPart: "token_endpoint",
		},
		{
			name: "device authorization endpoint",
			docFn: func(issuer string) string {
				return fmt.Sprintf(`{"issuer": %q, "token_endpoint": "%s/token", "device_authorization_endpoint": "http://insecure.example.com/device"}`, issuer, issuer)
			},
			wantPart: "device_authorization_endpoint",
		},
		{
			name: "revocation endpoint",
			docFn: func(issuer string) string {
				return fmt.Sprintf(`{"issuer": %q, "token_endpoint": "%s/token", "revocation_endpoint": "http://insecure.example.com/revoke"}`, issuer, issuer)
			},
			wantPart: "revocation_endpoint",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newIssuerServer(t, tc.docFn)

			_, err := Discover(context.Background(), srv.URL, srv.Client())
			if err == nil {
				t.Fatalf("Discover accepted an http:// %s", tc.wantPart)
			}
			if !strings.Contains(err.Error(), tc.wantPart) {
				t.Errorf("error %q does not name the %s", err.Error(), tc.wantPart)
			}
			if !strings.Contains(err.Error(), "https://") {
				t.Errorf("error %q does not explain the https requirement", err.Error())
			}
		})
	}
}

func TestDiscover_TruncatesALongErrorBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, strings.Repeat("x", 10_000))
	}))
	t.Cleanup(srv.Close)

	_, err := Discover(context.Background(), srv.URL, srv.Client())
	if err == nil {
		t.Fatal("Discover succeeded against a server that returns HTTP 500")
	}
	if len(err.Error()) > 2_000 {
		t.Errorf("the error is %d bytes long; a provider body must be truncated before it reaches an error", len(err.Error()))
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("error %q does not say that the body was truncated", err.Error())
	}
}

// testIDToken builds a signed JWT. Nothing under test verifies the
// signature, but the token must be well formed.
func testIDToken(t *testing.T, email string, expiresAt time.Time) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub": "user-1",
		"exp": jwt.NewNumericDate(expiresAt),
		"iat": jwt.NewNumericDate(expiresAt.Add(-time.Hour)),
	}
	if email != "" {
		claims["email"] = email
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(testKey.privateKey)
	if err != nil {
		t.Fatalf("signing the test ID token: %v", err)
	}
	return signed
}

// deviceServerConfig fixes the behaviour of a deviceServer. The handler
// goroutines read every field, so a test sets them at construction and never
// writes them again. A write after the server starts is a data race, and the
// race detector reports it.
type deviceServerConfig struct {
	// pendingPolls is the number of authorization_pending responses that the
	// token endpoint returns before it succeeds.
	pendingPolls int32
	// tokenResponse is the JSON body that the token endpoint returns on
	// success.
	tokenResponse string
	// terminalError, when not empty, is the OAuth error code that the token
	// endpoint returns instead of a success.
	terminalError string
}

// deviceServer is a fake identity provider that supports the device grant.
type deviceServer struct {
	*httptest.Server

	cfg deviceServerConfig
	// pending counts down the remaining authorization_pending responses.
	pending atomic.Int32
	// tokenRequests counts the requests that reach the token endpoint.
	tokenRequests atomic.Int32
	// lastTokenForm holds the form of the most recent token request.
	lastTokenForm atomic.Pointer[url.Values]
}

func newDeviceServer(t *testing.T, cfg deviceServerConfig) *deviceServer {
	t.Helper()

	ds := &deviceServer{cfg: cfg}
	ds.pending.Store(cfg.pendingPolls)
	mux := http.NewServeMux()
	ds.Server = httptest.NewTLSServer(mux)
	t.Cleanup(ds.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, fullDiscoveryDoc(ds.URL))
	})
	mux.HandleFunc("/oauth/v2/device_authorization", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"device_code": "device-code-1",
			"user_code": "ABCD-EFGH",
			"verification_uri": "%s/device",
			"verification_uri_complete": "%s/device?user_code=ABCD-EFGH",
			"expires_in": 600,
			"interval": 1
		}`, ds.URL, ds.URL)
	})
	mux.HandleFunc("/oauth/v2/token", func(w http.ResponseWriter, r *http.Request) {
		ds.tokenRequests.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("parsing the token request form: %v", err)
		}
		form := r.PostForm
		ds.lastTokenForm.Store(&form)

		w.Header().Set("Content-Type", "application/json")
		switch {
		case ds.pending.Add(-1) >= 0:
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": "authorization_pending"}`)
		case ds.cfg.terminalError != "":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error": %q, "error_description": "the provider said no"}`, ds.cfg.terminalError)
		default:
			fmt.Fprint(w, ds.cfg.tokenResponse)
		}
	})
	return ds
}

// successBody builds a token endpoint success body.
func successBody(idToken, refreshToken string) string {
	body := map[string]any{
		"access_token": "access-token-1",
		"token_type":   "Bearer",
		"expires_in":   3600,
		"id_token":     idToken,
	}
	if refreshToken != "" {
		body["refresh_token"] = refreshToken
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func testUserConfig(ds *deviceServer) OIDCUserConfig {
	return OIDCUserConfig{
		Clock:      clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)),
		Issuer:     ds.URL,
		ClientID:   "client-1",
		HTTPClient: ds.Client(),
		// The credential tests in Task 3 reuse this config and call
		// GetRequestMetadata, which rejects a request that carries no
		// AuthInfo unless the check is skipped. auth_test.go uses the same
		// method.
		SkipTransportSecurity: true,
	}
}

func TestDeviceLogin_Success(t *testing.T) {
	t.Parallel()

	expiry := time.Date(2026, time.August, 11, 13, 0, 0, 0, time.UTC)
	ds := newDeviceServer(t, deviceServerConfig{
		pendingPolls:  1,
		tokenResponse: successBody(testIDToken(t, "joey@aalyria.com", expiry), "refresh-token-1"),
	})

	var prompts []DevicePrompt
	tokens, err := DeviceLogin(context.Background(), testUserConfig(ds), func(p DevicePrompt) error {
		prompts = append(prompts, p)
		return nil
	})
	if err != nil {
		t.Fatalf("DeviceLogin returned an unexpected error: %v", err)
	}

	if len(prompts) != 1 {
		t.Fatalf("the prompt ran %d times, but exactly 1 was expected", len(prompts))
	}
	if want := ds.URL + "/device"; prompts[0].VerificationURI != want {
		t.Errorf("VerificationURI: got %q, want %q", prompts[0].VerificationURI, want)
	}
	if prompts[0].UserCode != "ABCD-EFGH" {
		t.Errorf("UserCode: got %q, want %q", prompts[0].UserCode, "ABCD-EFGH")
	}
	if prompts[0].VerificationURIComplete == "" {
		t.Error("VerificationURIComplete is empty, but the server supplied one")
	}
	if tokens.RefreshToken != "refresh-token-1" {
		t.Errorf("RefreshToken: got %q, want %q", tokens.RefreshToken, "refresh-token-1")
	}
	if !tokens.Expiry.Equal(expiry) {
		t.Errorf("Expiry: got %v, want %v (the exp claim of the ID token)", tokens.Expiry, expiry)
	}

	form := ds.lastTokenForm.Load()
	if got := form.Get("grant_type"); got != "urn:ietf:params:oauth:grant-type:device_code" {
		t.Errorf("grant_type: got %q, want the device code grant", got)
	}
}

func TestDeviceLogin_RequestsOfflineAccess(t *testing.T) {
	t.Parallel()

	ds := newDeviceServer(t, deviceServerConfig{
		tokenResponse: successBody(testIDToken(t, "joey@aalyria.com", time.Now().Add(time.Hour)), "refresh-token-1"),
	})

	if _, err := DeviceLogin(context.Background(), testUserConfig(ds), func(DevicePrompt) error { return nil }); err != nil {
		t.Fatalf("DeviceLogin returned an unexpected error: %v", err)
	}

	scope := ds.lastTokenForm.Load().Get("scope")
	for _, want := range []string{"openid", "profile", "email", "offline_access"} {
		if !strings.Contains(scope, want) {
			t.Errorf("scope %q does not contain %q", scope, want)
		}
	}
}

func TestDeviceLogin_NoRefreshTokenStillSucceeds(t *testing.T) {
	t.Parallel()

	ds := newDeviceServer(t, deviceServerConfig{
		tokenResponse: successBody(testIDToken(t, "joey@aalyria.com", time.Now().Add(time.Hour)), ""),
	})

	tokens, err := DeviceLogin(context.Background(), testUserConfig(ds), func(DevicePrompt) error { return nil })
	if err != nil {
		t.Fatalf("DeviceLogin returned an unexpected error: %v", err)
	}
	if tokens.RefreshToken != "" {
		t.Errorf("RefreshToken: got %q, want an empty string", tokens.RefreshToken)
	}
}

func TestDeviceLogin_NoIDToken(t *testing.T) {
	t.Parallel()

	ds := newDeviceServer(t, deviceServerConfig{
		tokenResponse: `{"access_token": "a", "token_type": "Bearer", "expires_in": 3600}`,
	})

	_, err := DeviceLogin(context.Background(), testUserConfig(ds), func(DevicePrompt) error { return nil })
	if err == nil {
		t.Fatal("DeviceLogin accepted a token response that has no ID token")
	}
	if !strings.Contains(err.Error(), "no ID token") {
		t.Errorf("error %q does not explain that the ID token is missing", err.Error())
	}
}

// TestDeviceLogin_ReadsStructuredOAuthError makes sure that the error code
// comes from the *oauth2.RetrieveError fields and not from the text of the
// response body.
func TestDeviceLogin_ReadsStructuredOAuthError(t *testing.T) {
	t.Parallel()

	ds := newDeviceServer(t, deviceServerConfig{terminalError: "access_denied"})

	_, err := DeviceLogin(context.Background(), testUserConfig(ds), func(DevicePrompt) error { return nil })
	if err == nil {
		t.Fatal("DeviceLogin succeeded against a server that denies the request")
	}

	retrieveErr := &oauth2.RetrieveError{}
	if !errors.As(err, &retrieveErr) {
		t.Fatalf("error %v does not wrap an *oauth2.RetrieveError", err)
	}
	if retrieveErr.ErrorCode != "access_denied" {
		t.Errorf("ErrorCode: got %q, want %q", retrieveErr.ErrorCode, "access_denied")
	}
	if retrieveErr.ErrorDescription != "the provider said no" {
		t.Errorf("ErrorDescription: got %q, want %q", retrieveErr.ErrorDescription, "the provider said no")
	}
}

func TestDeviceLogin_NoEmailClaim(t *testing.T) {
	t.Parallel()

	ds := newDeviceServer(t, deviceServerConfig{
		tokenResponse: successBody(testIDToken(t, "", time.Now().Add(time.Hour)), "refresh-token-1"),
	})

	_, err := DeviceLogin(context.Background(), testUserConfig(ds), func(DevicePrompt) error { return nil })
	if err == nil {
		t.Fatal("DeviceLogin accepted an ID token that has no email claim")
	}
	if !strings.Contains(err.Error(), "email") {
		t.Errorf("error %q does not name the email claim", err.Error())
	}
}

func TestDeviceLogin_NoDeviceEndpoint(t *testing.T) {
	t.Parallel()

	srv := newIssuerServer(t, func(issuer string) string {
		return fmt.Sprintf(`{"issuer": %q, "token_endpoint": "%s/token"}`, issuer, issuer)
	})

	c := OIDCUserConfig{
		Clock:      clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)),
		Issuer:     srv.URL,
		ClientID:   "client-1",
		HTTPClient: srv.Client(),
	}
	_, err := DeviceLogin(context.Background(), c, func(DevicePrompt) error { return nil })
	if err == nil {
		t.Fatal("DeviceLogin succeeded against an issuer that has no device endpoint")
	}
	if !strings.Contains(err.Error(), "device authorization grant") {
		t.Errorf("error %q does not explain that the device grant is unsupported", err.Error())
	}
}

// TestDeviceLogin_ExpiredCodeIsActionable covers the failure-mode table of
// the design: the user who never approves the login must be told to run the
// login again, not shown a raw oauth2 error.
func TestDeviceLogin_ExpiredCodeIsActionable(t *testing.T) {
	t.Parallel()

	ds := newDeviceServer(t, deviceServerConfig{terminalError: "expired_token"})
	c := testUserConfig(ds)
	c.LoginHint = "nbictl --profile=dev login"

	_, err := DeviceLogin(context.Background(), c, func(DevicePrompt) error { return nil })
	if err == nil {
		t.Fatal("DeviceLogin succeeded against a server that reports expired_token")
	}
	if !errors.Is(err, ErrCodeExpired) {
		t.Errorf("error: got %v, want it to wrap ErrCodeExpired", err)
	}
	if !strings.Contains(err.Error(), "nbictl --profile=dev login") {
		t.Errorf("error %q does not name the login command", err.Error())
	}
}

// TestDeviceLogin_DeadlineIsActionable covers the same row of the table by
// the other route: the oauth2 package stops polling at the code's expiry and
// returns context.DeadlineExceeded rather than an OAuth error.
func TestDeviceLogin_DeadlineIsActionable(t *testing.T) {
	t.Parallel()

	ds := newDeviceServer(t, deviceServerConfig{pendingPolls: 1 << 30})
	c := testUserConfig(ds)
	c.LoginHint = "nbictl --profile=dev login"

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := DeviceLogin(ctx, c, func(DevicePrompt) error { return nil })
	if err == nil {
		t.Fatal("DeviceLogin succeeded although the deadline passed")
	}
	if !errors.Is(err, ErrCodeExpired) {
		t.Errorf("error: got %v, want it to wrap ErrCodeExpired", err)
	}
	if !strings.Contains(err.Error(), "nbictl --profile=dev login") {
		t.Errorf("error %q does not name the login command", err.Error())
	}
}

// TestDeviceLogin_CancellationIsNotReportedAsAnExpiredCode keeps Ctrl-C
// distinct from an expiry. The design says Ctrl-C stops the poll immediately;
// telling that user that "the code expired" would be wrong.
func TestDeviceLogin_CancellationIsNotReportedAsAnExpiredCode(t *testing.T) {
	t.Parallel()

	ds := newDeviceServer(t, deviceServerConfig{pendingPolls: 1 << 30})
	ctx, cancel := context.WithCancel(context.Background())

	_, err := DeviceLogin(ctx, testUserConfig(ds), func(DevicePrompt) error {
		cancel()
		return nil
	})
	if err == nil {
		t.Fatal("DeviceLogin succeeded although the context was cancelled")
	}
	if errors.Is(err, ErrCodeExpired) {
		t.Errorf("a cancelled login reported an expired code: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error: got %v, want it to wrap context.Canceled", err)
	}
}

func TestDeviceLogin_PromptErrorStopsTheLogin(t *testing.T) {
	t.Parallel()

	ds := newDeviceServer(t, deviceServerConfig{
		tokenResponse: successBody(testIDToken(t, "joey@aalyria.com", time.Now().Add(time.Hour)), "r"),
	})

	wantErr := errors.New("the writer is broken")
	_, err := DeviceLogin(context.Background(), testUserConfig(ds), func(DevicePrompt) error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Errorf("error: got %v, want it to wrap %v", err, wantErr)
	}
}

func TestParseIDTokenClaims(t *testing.T) {
	t.Parallel()

	expiry := time.Date(2026, time.August, 11, 13, 0, 0, 0, time.UTC)
	claims, err := ParseIDTokenClaims(testIDToken(t, "joey@aalyria.com", expiry))
	if err != nil {
		t.Fatalf("ParseIDTokenClaims returned an unexpected error: %v", err)
	}
	if claims.Email != "joey@aalyria.com" {
		t.Errorf("Email: got %q, want %q", claims.Email, "joey@aalyria.com")
	}
	if !claims.ExpiresAt.Equal(expiry) {
		t.Errorf("ExpiresAt: got %v, want %v", claims.ExpiresAt, expiry)
	}
}

func TestParseIDTokenClaims_Malformed(t *testing.T) {
	t.Parallel()

	if _, err := ParseIDTokenClaims("this is not a jwt"); err == nil {
		t.Fatal("ParseIDTokenClaims accepted a malformed token")
	}
}

// memoryStore is an in-memory TokenStore. No test in auth/ touches a file.
type memoryStore struct {
	mu     sync.Mutex
	tokens *Tokens
	saves  int
	loads  int
	// onLoad runs at the start of every Load. It lets a test act as a second
	// nbictl process that writes the token file between two loads. A test
	// sets it before the credentials run, and never writes it again.
	onLoad func()
}

func (s *memoryStore) Load() (*Tokens, error) {
	s.mu.Lock()
	s.loads++
	onLoad := s.onLoad
	s.mu.Unlock()

	// Call the hook without the lock, so that it can call Save.
	if onLoad != nil {
		onLoad()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokens == nil {
		return nil, nil
	}
	copied := *s.tokens
	return &copied, nil
}

func (s *memoryStore) Save(t *Tokens) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := *t
	s.tokens = &copied
	s.saves++
	return nil
}

func (s *memoryStore) saveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saves
}

func (s *memoryStore) loadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loads
}

// requestMetadata calls GetRequestMetadata with the request info that gRPC
// supplies at call time. The request carries no AuthInfo, so every config
// that reaches this helper must set SkipTransportSecurity. The existing
// tests in auth_test.go use the same method.
func requestMetadata(ctx context.Context, creds credentials.PerRPCCredentials) (map[string]string, error) {
	ctx = credentials.NewContextWithRequestInfo(ctx, credentials.RequestInfo{
		Method: "/test.Service/Method",
	})
	return creds.GetRequestMetadata(ctx, "https://nbi.example.com/test.Service")
}

func TestNewOIDCUserCredentials_Validation(t *testing.T) {
	t.Parallel()

	valid := OIDCUserConfig{
		Clock:    clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)),
		Issuer:   "https://zitadel.example.com",
		ClientID: "client-1",
	}

	for _, tc := range []struct {
		name   string
		mutate func(*OIDCUserConfig)
		store  TokenStore
		want   string
	}{
		{
			name:   "missing clock",
			mutate: func(c *OIDCUserConfig) { c.Clock = nil },
			store:  &memoryStore{},
			want:   "missing required field 'Clock'",
		},
		{
			name:   "missing issuer",
			mutate: func(c *OIDCUserConfig) { c.Issuer = "" },
			store:  &memoryStore{},
			want:   "missing required field 'Issuer'",
		},
		{
			name:   "missing client ID",
			mutate: func(c *OIDCUserConfig) { c.ClientID = "" },
			store:  &memoryStore{},
			want:   "missing required field 'ClientID'",
		},
		{
			name:   "missing store",
			mutate: func(*OIDCUserConfig) {},
			store:  nil,
			want:   "missing required token store",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := valid
			tc.mutate(&c)

			_, err := NewOIDCUserCredentials(context.Background(), c, tc.store)
			if err == nil {
				t.Fatalf("NewOIDCUserCredentials succeeded, but %q was expected", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestOIDCUserCredentials_SendsValidToken(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC))
	idToken := testIDToken(t, "joey@aalyria.com", clock.Now().Add(time.Hour))
	store := &memoryStore{tokens: &Tokens{IDToken: idToken, RefreshToken: "r", Expiry: clock.Now().Add(time.Hour)}}

	creds, err := NewOIDCUserCredentials(context.Background(), OIDCUserConfig{
		Clock:                 clock,
		Issuer:                "https://zitadel.example.com",
		ClientID:              "client-1",
		SkipTransportSecurity: true,
	}, store)
	if err != nil {
		t.Fatalf("NewOIDCUserCredentials returned an unexpected error: %v", err)
	}

	md, err := requestMetadata(context.Background(), creds)
	if err != nil {
		t.Fatalf("GetRequestMetadata returned an unexpected error: %v", err)
	}
	if want := "Bearer " + idToken; md["authorization"] != want {
		t.Errorf("authorization header: got %q, want %q", md["authorization"], want)
	}
	if store.saveCount() != 0 {
		t.Errorf("the store was written %d times, but a valid token needs no write", store.saveCount())
	}
}

// TestOIDCUserCredentials_CachesAValidToken makes sure that a valid token is
// held in memory. This is a PerRPCCredentials, so GetRequestMetadata runs on
// every outbound RPC, and nbictl fans out to --max-concurrency RPCs sharing
// one credentials object. Reading the store each time would be a file read and
// a JSON decode per RPC. oidcCredentials caches the same way (auth.go).
func TestOIDCUserCredentials_CachesAValidToken(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC))
	idToken := testIDToken(t, "joey@aalyria.com", clock.Now().Add(time.Hour))
	store := &memoryStore{tokens: &Tokens{
		IDToken:      idToken,
		RefreshToken: "refresh-token-1",
		Expiry:       clock.Now().Add(time.Hour),
	}}

	creds, err := NewOIDCUserCredentials(context.Background(), OIDCUserConfig{
		Clock:                 clock,
		Issuer:                "https://zitadel.example.com",
		ClientID:              "client-1",
		SkipTransportSecurity: true,
	}, store)
	if err != nil {
		t.Fatalf("NewOIDCUserCredentials returned an unexpected error: %v", err)
	}

	const rpcs = 5
	for i := range rpcs {
		md, err := requestMetadata(context.Background(), creds)
		if err != nil {
			t.Fatalf("GetRequestMetadata returned an unexpected error on RPC %d: %v", i, err)
		}
		if want := "Bearer " + idToken; md["authorization"] != want {
			t.Fatalf("authorization header on RPC %d: got %q, want %q", i, md["authorization"], want)
		}
	}

	// The first RPC reads the file, so every command still starts from the
	// state on disk. The rest are served from memory.
	if got := store.loadCount(); got != 1 {
		t.Errorf("the store was read %d times for %d RPCs, want exactly 1", got, rpcs)
	}
}

func TestOIDCUserCredentials_NotLoggedIn(t *testing.T) {
	t.Parallel()

	creds, err := NewOIDCUserCredentials(context.Background(), OIDCUserConfig{
		Clock:                 clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)),
		Issuer:                "https://zitadel.example.com",
		ClientID:              "client-1",
		LoginHint:             "nbictl --profile=dev login",
		SkipTransportSecurity: true,
	}, &memoryStore{})
	if err != nil {
		t.Fatalf("NewOIDCUserCredentials returned an unexpected error: %v", err)
	}

	_, err = requestMetadata(context.Background(), creds)
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("error: got %v, want it to wrap ErrNotLoggedIn", err)
	}
	if !strings.Contains(err.Error(), "nbictl --profile=dev login") {
		t.Errorf("error %q does not name the login command", err.Error())
	}
}

func TestOIDCUserCredentials_RefreshesAndSaves(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC))
	refreshedIDToken := testIDToken(t, "joey@aalyria.com", clock.Now().Add(time.Hour))
	ds := newDeviceServer(t, deviceServerConfig{
		tokenResponse: successBody(refreshedIDToken, "refresh-token-2"),
	})

	// The stored token expires inside tokenExpirationWindow, so the
	// credentials must refresh it.
	store := &memoryStore{tokens: &Tokens{
		IDToken:      testIDToken(t, "joey@aalyria.com", clock.Now().Add(time.Minute)),
		RefreshToken: "refresh-token-1",
		Expiry:       clock.Now().Add(time.Minute),
	}}

	c := testUserConfig(ds)
	c.Clock = clock
	creds, err := NewOIDCUserCredentials(context.Background(), c, store)
	if err != nil {
		t.Fatalf("NewOIDCUserCredentials returned an unexpected error: %v", err)
	}

	md, err := requestMetadata(context.Background(), creds)
	if err != nil {
		t.Fatalf("GetRequestMetadata returned an unexpected error: %v", err)
	}
	if want := "Bearer " + refreshedIDToken; md["authorization"] != want {
		t.Errorf("authorization header: got %q, want the refreshed ID token", md["authorization"])
	}

	form := ds.lastTokenForm.Load()
	if got := form.Get("grant_type"); got != "refresh_token" {
		t.Errorf("grant_type: got %q, want %q", got, "refresh_token")
	}
	if got := form.Get("refresh_token"); got != "refresh-token-1" {
		t.Errorf("refresh_token: got %q, want %q", got, "refresh-token-1")
	}

	saved, err := store.Load()
	if err != nil {
		t.Fatalf("the store returned an unexpected error: %v", err)
	}
	if saved.IDToken != refreshedIDToken {
		t.Error("the store does not hold the refreshed ID token")
	}
	if saved.RefreshToken != "refresh-token-2" {
		t.Errorf("the stored refresh token is %q, want %q", saved.RefreshToken, "refresh-token-2")
	}
}

// TestOIDCUserCredentials_ExpiredSessionIsActionable covers the failure-mode
// table of the design: a refresh that fails must tell the user to log in
// again, not only report that the session expired.
func TestOIDCUserCredentials_ExpiredSessionIsActionable(t *testing.T) {
	t.Parallel()

	ds := newDeviceServer(t, deviceServerConfig{terminalError: "invalid_grant"})

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC))
	store := &memoryStore{tokens: &Tokens{
		IDToken:      testIDToken(t, "joey@aalyria.com", clock.Now().Add(time.Minute)),
		RefreshToken: "refresh-token-1",
		Expiry:       clock.Now().Add(time.Minute),
	}}

	c := testUserConfig(ds)
	c.Clock = clock
	c.LoginHint = "nbictl --profile=dev login"
	creds, err := NewOIDCUserCredentials(context.Background(), c, store)
	if err != nil {
		t.Fatalf("NewOIDCUserCredentials returned an unexpected error: %v", err)
	}

	_, err = requestMetadata(context.Background(), creds)
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("error: got %v, want it to wrap ErrSessionExpired", err)
	}
	if !strings.Contains(err.Error(), "nbictl --profile=dev login") {
		t.Errorf("error %q does not name the login command", err.Error())
	}
}

// TestOIDCUserCredentials_RefreshLostToAnotherProcess covers the refresh-token
// rotation race between two nbictl processes. The provider rotates the refresh
// token, so the process that arrives second holds a dead one. If another
// process has already stored a fresh ID token, use it instead of telling the
// user that a healthy session expired.
func TestOIDCUserCredentials_RefreshLostToAnotherProcess(t *testing.T) {
	t.Parallel()

	ds := newDeviceServer(t, deviceServerConfig{terminalError: "invalid_grant"})

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC))
	freshIDToken := testIDToken(t, "joey@aalyria.com", clock.Now().Add(time.Hour))
	store := &memoryStore{tokens: &Tokens{
		IDToken:      testIDToken(t, "joey@aalyria.com", clock.Now().Add(time.Minute)),
		RefreshToken: "refresh-token-1",
		Expiry:       clock.Now().Add(time.Minute),
	}}
	// Once the provider has rejected this process's refresh token, the other
	// process has already rotated it and stored the result.
	store.onLoad = func() {
		if ds.tokenRequests.Load() == 0 {
			return
		}
		if err := store.Save(&Tokens{
			IDToken:      freshIDToken,
			RefreshToken: "refresh-token-2",
			Expiry:       clock.Now().Add(time.Hour),
		}); err != nil {
			t.Errorf("saving the tokens of the other process: %v", err)
		}
	}

	c := testUserConfig(ds)
	c.Clock = clock
	creds, err := NewOIDCUserCredentials(context.Background(), c, store)
	if err != nil {
		t.Fatalf("NewOIDCUserCredentials returned an unexpected error: %v", err)
	}

	md, err := requestMetadata(context.Background(), creds)
	if err != nil {
		t.Fatalf("GetRequestMetadata reported a failure although another process stored a fresh token: %v", err)
	}
	if want := "Bearer " + freshIDToken; md["authorization"] != want {
		t.Errorf("authorization header: got %q, want the token that the other process stored", md["authorization"])
	}
}

func TestOIDCUserCredentials_NoRefreshTokenReportsExpiry(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC))
	store := &memoryStore{tokens: &Tokens{
		IDToken: testIDToken(t, "joey@aalyria.com", clock.Now().Add(time.Minute)),
		Expiry:  clock.Now().Add(time.Minute),
	}}

	creds, err := NewOIDCUserCredentials(context.Background(), OIDCUserConfig{
		Clock:                 clock,
		Issuer:                "https://zitadel.example.com",
		ClientID:              "client-1",
		SkipTransportSecurity: true,
	}, store)
	if err != nil {
		t.Fatalf("NewOIDCUserCredentials returned an unexpected error: %v", err)
	}

	_, err = requestMetadata(context.Background(), creds)
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("error: got %v, want it to wrap ErrSessionExpired", err)
	}
}

// TestOIDCUserCredentials_ConcurrentRefresh makes sure that the singleflight
// group collapses concurrent refreshes into one. Run it with the race
// detector.
func TestOIDCUserCredentials_ConcurrentRefresh(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC))
	ds := newDeviceServer(t, deviceServerConfig{
		tokenResponse: successBody(testIDToken(t, "joey@aalyria.com", clock.Now().Add(time.Hour)), "refresh-token-2"),
	})

	store := &memoryStore{tokens: &Tokens{
		IDToken:      testIDToken(t, "joey@aalyria.com", clock.Now().Add(time.Minute)),
		RefreshToken: "refresh-token-1",
		Expiry:       clock.Now().Add(time.Minute),
	}}

	c := testUserConfig(ds)
	c.Clock = clock
	creds, err := NewOIDCUserCredentials(context.Background(), c, store)
	if err != nil {
		t.Fatalf("NewOIDCUserCredentials returned an unexpected error: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			<-start
			_, err := requestMetadata(context.Background(), creds)
			errs <- err
		}()
	}
	close(start)
	for range 8 {
		if err := <-errs; err != nil {
			t.Errorf("GetRequestMetadata returned an unexpected error: %v", err)
		}
	}

	if got := ds.tokenRequests.Load(); got != 1 {
		t.Errorf("the token endpoint received %d requests, but exactly 1 was expected", got)
	}
}

func TestOIDCUserCredentials_RequireTransportSecurity(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC))
	base := OIDCUserConfig{Clock: clock, Issuer: "https://zitadel.example.com", ClientID: "client-1"}

	creds, err := NewOIDCUserCredentials(context.Background(), base, &memoryStore{})
	if err != nil {
		t.Fatalf("NewOIDCUserCredentials returned an unexpected error: %v", err)
	}
	if !creds.RequireTransportSecurity() {
		t.Error("RequireTransportSecurity: got false, want true by default")
	}

	skipped := base
	skipped.SkipTransportSecurity = true
	creds, err = NewOIDCUserCredentials(context.Background(), skipped, &memoryStore{})
	if err != nil {
		t.Fatalf("NewOIDCUserCredentials returned an unexpected error: %v", err)
	}
	if creds.RequireTransportSecurity() {
		t.Error("RequireTransportSecurity: got true, want false when SkipTransportSecurity is set")
	}
}

func TestRevoke_Success(t *testing.T) {
	t.Parallel()

	gotForm := &atomic.Pointer[url.Values]{}
	mux := http.NewServeMux()
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, fullDiscoveryDoc(srv.URL))
	})
	mux.HandleFunc("/oauth/v2/revoke", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parsing the revocation form: %v", err)
		}
		form := r.PostForm
		gotForm.Store(&form)
		w.WriteHeader(http.StatusOK)
	})

	c := OIDCUserConfig{
		Clock:      clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)),
		Issuer:     srv.URL,
		ClientID:   "client-1",
		HTTPClient: srv.Client(),
	}
	if err := Revoke(context.Background(), c, "refresh-token-1"); err != nil {
		t.Fatalf("Revoke returned an unexpected error: %v", err)
	}

	form := gotForm.Load()
	if got := form.Get("token"); got != "refresh-token-1" {
		t.Errorf("token: got %q, want %q", got, "refresh-token-1")
	}
	if got := form.Get("token_type_hint"); got != "refresh_token" {
		t.Errorf("token_type_hint: got %q, want %q", got, "refresh_token")
	}
	if got := form.Get("client_id"); got != "client-1" {
		t.Errorf("client_id: got %q, want %q", got, "client-1")
	}
}

func TestRevoke_NoRevocationEndpoint(t *testing.T) {
	t.Parallel()

	srv := newIssuerServer(t, func(issuer string) string {
		return fmt.Sprintf(`{"issuer": %q, "token_endpoint": "%s/token"}`, issuer, issuer)
	})

	c := OIDCUserConfig{
		Clock:      clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)),
		Issuer:     srv.URL,
		ClientID:   "client-1",
		HTTPClient: srv.Client(),
	}
	if err := Revoke(context.Background(), c, "refresh-token-1"); !errors.Is(err, ErrNoRevocationEndpoint) {
		t.Fatalf("error: got %v, want ErrNoRevocationEndpoint", err)
	}
}

func TestRevoke_ServerError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, fullDiscoveryDoc(srv.URL))
	})
	mux.HandleFunc("/oauth/v2/revoke", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := OIDCUserConfig{
		Clock:      clockwork.NewFakeClockAt(time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)),
		Issuer:     srv.URL,
		ClientID:   "client-1",
		HTTPClient: srv.Client(),
	}
	err := Revoke(context.Background(), c, "refresh-token-1")
	if err == nil {
		t.Fatal("Revoke succeeded against a server that returns HTTP 500")
	}
	if errors.Is(err, ErrNoRevocationEndpoint) {
		t.Error("a server fault must not report a missing revocation endpoint")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not name the status code", err.Error())
	}
}
