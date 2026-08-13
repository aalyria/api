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

package nbictl

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/credentials"

	"aalyria.com/spacetime/auth"
	"aalyria.com/spacetime/tools/nbictl/nbictlpb"
)

func TestFileTokenStore_SaveAndLoad(t *testing.T) {
	t.Parallel()

	store := newFileTokenStore(t.TempDir(), "dev")
	want := &auth.Tokens{
		IDToken:      "id-token-1",
		RefreshToken: "refresh-token-1",
		Expiry:       time.Date(2026, time.August, 11, 13, 0, 0, 0, time.UTC),
	}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned an unexpected error: %v", err)
	}
	if got.IDToken != want.IDToken {
		t.Errorf("IDToken: got %q, want %q", got.IDToken, want.IDToken)
	}
	if got.RefreshToken != want.RefreshToken {
		t.Errorf("RefreshToken: got %q, want %q", got.RefreshToken, want.RefreshToken)
	}
	if !got.Expiry.Equal(want.Expiry) {
		t.Errorf("Expiry: got %v, want %v", got.Expiry, want.Expiry)
	}
}

func TestFileTokenStore_FileModeIs0600(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	store := newFileTokenStore(confDir, "dev")
	if err := store.Save(&auth.Tokens{IDToken: "id-token-1"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatalf("the token file is missing: %v", err)
	}
	if want := filepath.Join(confDir, "tokens"); filepath.Dir(store.path) != want {
		t.Errorf("the token directory is %q, want %q", filepath.Dir(store.path), want)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode: got %o, want %o", got, 0o600)
	}
}

func TestFileTokenStore_LeavesNoTemporaryFile(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	store := newFileTokenStore(confDir, "dev")
	if err := store.Save(&auth.Tokens{IDToken: "id-token-1"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(confDir, "tokens"))
	if err != nil {
		t.Fatalf("reading the token directory: %v", err)
	}
	if len(entries) != 1 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("the token directory holds %v, but only %s was expected", names, filepath.Base(store.path))
	}
}

func TestFileTokenStore_LoadWithNoFile(t *testing.T) {
	t.Parallel()

	got, err := newFileTokenStore(t.TempDir(), "dev").Load()
	if err != nil {
		t.Fatalf("Load returned an unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("Load: got %v, want nil when the user is not logged in", got)
	}
}

func TestFileTokenStore_Delete(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	store := newFileTokenStore(confDir, "dev")
	if err := store.Save(&auth.Tokens{IDToken: "id-token-1"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete returned an unexpected error: %v", err)
	}
	if _, err := os.Stat(store.path); !os.IsNotExist(err) {
		t.Error("the token file still exists after Delete")
	}

	// A second delete must succeed.
	if err := store.Delete(); err != nil {
		t.Errorf("the second Delete returned an unexpected error: %v", err)
	}
}

// TestFileTokenStore_SanitisesTheProfileName makes sure that a profile name
// cannot escape the token directory.
func TestFileTokenStore_SanitisesTheProfileName(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	store := newFileTokenStore(confDir, "../../escape")
	if err := store.Save(&auth.Tokens{IDToken: "id-token-1"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	wantDir := filepath.Join(confDir, "tokens")
	if !strings.HasPrefix(store.path, wantDir+string(filepath.Separator)) {
		t.Errorf("the token path %q is outside %q", store.path, wantDir)
	}
}

// TestFileTokenStore_DistinguishesProfilesThatSanitiseAlike makes sure that
// two profile names that reduce to the same safe characters keep separate
// token files. The two profiles can name different issuers and different NBI
// hosts, so a shared file would send one profile's ID token to the other
// profile's endpoint.
func TestFileTokenStore_DistinguishesProfilesThatSanitiseAlike(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	first := newFileTokenStore(confDir, "prod/x")
	second := newFileTokenStore(confDir, "prod_x")

	if first.path == second.path {
		t.Fatalf("the profiles %q and %q share the token file %q", "prod/x", "prod_x", first.path)
	}

	if err := first.Save(&auth.Tokens{IDToken: "id-token-first"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}
	if err := second.Save(&auth.Tokens{IDToken: "id-token-second"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	got, err := first.Load()
	if err != nil {
		t.Fatalf("Load returned an unexpected error: %v", err)
	}
	if got.IDToken != "id-token-first" {
		t.Errorf("IDToken: got %q, want %q; the second profile overwrote the first", got.IDToken, "id-token-first")
	}
}

// TestFileTokenStore_NameIsStable makes sure that two stores built from the
// same profile name agree on the path. The login command and the dial path
// each build their own store, so a name that varied between them would look
// to the user like a login that never took effect.
func TestFileTokenStore_NameIsStable(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	if first, second := newFileTokenStore(confDir, "dev"), newFileTokenStore(confDir, "dev"); first.path != second.path {
		t.Errorf("two stores for the same profile disagree: %q and %q", first.path, second.path)
	}
}

// testLoginKey signs the test ID tokens. Nothing under test verifies the
// signature — the Spacetime server does that — so a fixed HMAC key is enough
// and avoids an RSA keygen at package init.
var testLoginKey = []byte("nbictl-login-test-key")

func testLoginIDToken(t *testing.T, email string, expiry time.Time) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "user-1",
		"email": email,
		"exp":   jwt.NewNumericDate(expiry),
	}).SignedString(testLoginKey)
	if err != nil {
		t.Fatalf("signing the test ID token: %v", err)
	}
	return signed
}

// fakeIssuerConfig fixes the behaviour of a fakeIssuer. The handler
// goroutines read every field, so a test sets them at construction and never
// writes them again.
type fakeIssuerConfig struct {
	// idToken is returned by the token endpoint.
	idToken string
	// refreshToken is returned by the token endpoint. An empty value omits
	// the field.
	refreshToken string
	// noRevocationEndpoint removes revocation_endpoint from the discovery
	// document.
	noRevocationEndpoint bool
	// revocationStatus is the status code that the revocation endpoint
	// returns. Zero selects http.StatusOK.
	revocationStatus int
	// onRevoke runs inside the revocation handler, before the response is
	// written. It lets a test observe the state of the world at the moment
	// the provider is contacted.
	onRevoke func()
}

// fakeIssuer is a test identity provider that supports the device grant and
// RFC 7009 revocation.
type fakeIssuer struct {
	*httptest.Server

	cfg fakeIssuerConfig
	// revocations counts the requests to the revocation endpoint.
	revocations atomic.Int32
}

func newFakeIssuer(t *testing.T, cfg fakeIssuerConfig) *fakeIssuer {
	t.Helper()

	cfg.revocationStatus = cmp.Or(cfg.revocationStatus, http.StatusOK)
	fi := &fakeIssuer{cfg: cfg}
	mux := http.NewServeMux()
	fi.Server = httptest.NewTLSServer(mux)
	t.Cleanup(fi.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		doc := map[string]string{
			"issuer":                        fi.URL,
			"device_authorization_endpoint": fi.URL + "/device_authorization",
			"token_endpoint":                fi.URL + "/token",
		}
		if !fi.cfg.noRevocationEndpoint {
			doc["revocation_endpoint"] = fi.URL + "/revoke"
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(doc); err != nil {
			t.Errorf("writing the discovery document: %v", err)
		}
	})
	mux.HandleFunc("/device_authorization", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"device_code": "device-code-1",
			"user_code": "WXYZ-1234",
			"verification_uri": "%s/device",
			"expires_in": 600,
			"interval": 1
		}`, fi.URL)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		body := map[string]any{
			"access_token": "access-token-1",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     fi.cfg.idToken,
		}
		if fi.cfg.refreshToken != "" {
			body["refresh_token"] = fi.cfg.refreshToken
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("writing the token response: %v", err)
		}
	})
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, _ *http.Request) {
		fi.revocations.Add(1)
		if fi.cfg.onRevoke != nil {
			fi.cfg.onRevoke()
		}
		w.WriteHeader(fi.cfg.revocationStatus)
	})
	return fi
}

// newIssuerApp builds a test app that reaches fi, with a profile named "dev"
// that uses the oidc_user strategy.
func newIssuerApp(t *testing.T, fi *fakeIssuer, confDir string) testApp {
	t.Helper()

	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:         "dev",
		Url:          "nbi.example.com",
		AuthStrategy: oidcUserAuthStrategy(fi.URL, "client-1"),
	}, filepath.Join(confDir, confFileName)))

	app := newTestApp()
	app.Metadata = map[string]any{"oidcHTTPClient": fi.Client()}
	return app
}

func TestLogin_StoresTheTokens(t *testing.T) {
	t.Parallel()

	expiry := time.Now().Add(time.Hour).Truncate(time.Second)
	fi := newFakeIssuer(t, fakeIssuerConfig{
		idToken:      testLoginIDToken(t, "joey@aalyria.com", expiry),
		refreshToken: "refresh-token-1",
	})

	confDir := t.TempDir()
	app := newIssuerApp(t, fi, confDir)

	checkErr(t, app.Run([]string{"nbictl", "--config_dir", confDir, "--profile", "dev", "login"}))

	if got := app.stderr.String(); !strings.Contains(got, "WXYZ-1234") {
		t.Errorf("the error output %q does not hold the user code", got)
	}
	if got := app.stderr.String(); !strings.Contains(got, fi.URL+"/device") {
		t.Errorf("the error output %q does not hold the verification URI", got)
	}
	if got := app.stdout.String(); !strings.Contains(got, "joey@aalyria.com") {
		t.Errorf("the output %q does not name the user", got)
	}
	if got := app.stdout.String(); strings.Contains(got, "WXYZ-1234") {
		t.Errorf("the user code reached the output writer; the prompt must go to the error writer only. stdout: %q", got)
	}
	if got := app.stdout.String(); strings.Contains(got, "Waiting for the login") {
		t.Errorf("the wait message reached the output writer; it must go to the error writer only. stdout: %q", got)
	}

	stored, err := newFileTokenStore(confDir, "dev").Load()
	checkErr(t, err)
	if stored == nil {
		t.Fatal("login stored no tokens")
	}
	if stored.RefreshToken != "refresh-token-1" {
		t.Errorf("RefreshToken: got %q, want %q", stored.RefreshToken, "refresh-token-1")
	}
	if !stored.Expiry.Equal(expiry) {
		t.Errorf("Expiry: got %v, want %v", stored.Expiry, expiry)
	}
}

func TestLogin_WarnsWithNoRefreshToken(t *testing.T) {
	t.Parallel()

	fi := newFakeIssuer(t, fakeIssuerConfig{
		idToken: testLoginIDToken(t, "joey@aalyria.com", time.Now().Add(time.Hour)),
	})

	confDir := t.TempDir()
	app := newIssuerApp(t, fi, confDir)

	checkErr(t, app.Run([]string{"nbictl", "--config_dir", confDir, "--profile", "dev", "login"}))

	if got := app.stderr.String(); !strings.Contains(got, "no refresh token") {
		t.Errorf("the error output %q holds no warning about the missing refresh token", got)
	}
	if got := app.stdout.String(); !strings.Contains(got, "Logged in as") {
		t.Errorf("the output %q does not hold the success line", got)
	}
}

func TestLogout_RevokesAndDeletes(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	store := newFileTokenStore(confDir, "dev")

	tokenExistedAtRevocation := &atomic.Bool{}
	fi := newFakeIssuer(t, fakeIssuerConfig{onRevoke: func() {
		_, err := os.Stat(store.path)
		tokenExistedAtRevocation.Store(err == nil)
	}})
	app := newIssuerApp(t, fi, confDir)

	checkErr(t, store.Save(&auth.Tokens{IDToken: "id-token-1", RefreshToken: "refresh-token-1"}))

	checkErr(t, app.Run([]string{"nbictl", "--config_dir", confDir, "--profile", "dev", "logout"}))

	if got := fi.revocations.Load(); got != 1 {
		t.Errorf("the revocation endpoint received %d requests, but exactly 1 was expected", got)
	}
	loaded, err := store.Load()
	checkErr(t, err)
	if loaded != nil {
		t.Error("the token file still exists after logout")
	}
	if !tokenExistedAtRevocation.Load() {
		t.Error("the token file was already deleted when the revocation ran; logout must revoke first and delete second")
	}
}

func TestLogout_DeletesWhenTheRevocationFails(t *testing.T) {
	t.Parallel()

	fi := newFakeIssuer(t, fakeIssuerConfig{revocationStatus: http.StatusInternalServerError})

	confDir := t.TempDir()
	app := newIssuerApp(t, fi, confDir)

	store := newFileTokenStore(confDir, "dev")
	checkErr(t, store.Save(&auth.Tokens{IDToken: "id-token-1", RefreshToken: "refresh-token-1"}))

	checkErr(t, app.Run([]string{"nbictl", "--config_dir", confDir, "--profile", "dev", "logout"}))

	loaded, err := store.Load()
	checkErr(t, err)
	if loaded != nil {
		t.Error("the token file still exists after a failed revocation")
	}
	if got := app.stderr.String(); !strings.Contains(got, "revocation failed") {
		t.Errorf("the error output %q does not report the failed revocation", got)
	}
}

func TestLogout_DeletesWhenThereIsNoRevocationEndpoint(t *testing.T) {
	t.Parallel()

	fi := newFakeIssuer(t, fakeIssuerConfig{noRevocationEndpoint: true})

	confDir := t.TempDir()
	app := newIssuerApp(t, fi, confDir)

	store := newFileTokenStore(confDir, "dev")
	checkErr(t, store.Save(&auth.Tokens{IDToken: "id-token-1", RefreshToken: "refresh-token-1"}))

	checkErr(t, app.Run([]string{"nbictl", "--config_dir", confDir, "--profile", "dev", "logout"}))

	loaded, err := store.Load()
	checkErr(t, err)
	if loaded != nil {
		t.Error("the token file still exists after logout")
	}
	if got := fi.revocations.Load(); got != 0 {
		t.Errorf("the revocation endpoint received %d requests, but none was expected", got)
	}
	if got := app.stderr.String(); !strings.Contains(got, "no revocation endpoint") {
		t.Errorf("the error output %q does not name the missing revocation endpoint", got)
	}
}

func TestLogout_WithNoTokenFile(t *testing.T) {
	t.Parallel()

	fi := newFakeIssuer(t, fakeIssuerConfig{})
	confDir := t.TempDir()
	app := newIssuerApp(t, fi, confDir)

	checkErr(t, app.Run([]string{"nbictl", "--config_dir", confDir, "--profile", "dev", "logout"}))

	if got := app.stdout.String(); !strings.Contains(got, "Not logged in") {
		t.Errorf("the output %q does not report that the user is not logged in", got)
	}
	if got := fi.revocations.Load(); got != 0 {
		t.Errorf("the revocation endpoint received %d requests, but none was expected", got)
	}
}

// TestLogout_DeletesWhenTheStrategyChanged makes sure that a token file is
// never orphaned. A user who moves the profile to another strategy still has
// a live refresh token on disk, and `logout` is the command that removes it.
func TestLogout_DeletesWhenTheStrategyChanged(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	store := newFileTokenStore(confDir, "dev")
	checkErr(t, store.Save(&auth.Tokens{IDToken: "id-token-1", RefreshToken: "refresh-token-1"}))

	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:         "dev",
		Url:          "nbi.example.com",
		AuthStrategy: jwtAuthStrategy("someone@example.com", "key-1", "/tmp/key.pem"),
	}, filepath.Join(confDir, confFileName)))

	app := newTestApp()
	checkErr(t, app.Run([]string{"nbictl", "--config_dir", confDir, "--profile", "dev", "logout"}))

	loaded, err := store.Load()
	checkErr(t, err)
	if loaded != nil {
		t.Error("logout left the token file on disk although the profile no longer uses oidc_user")
	}
	if got := app.stderr.String(); !strings.Contains(got, "oidc_user") {
		t.Errorf("the error output %q does not explain that the profile no longer uses oidc_user, so nothing was revoked", got)
	}
}

// TestLogin_ShowsWhenTheCodeExpires makes sure that the prompt tells the user
// how long they have to approve the login.
func TestLogin_ShowsWhenTheCodeExpires(t *testing.T) {
	t.Parallel()

	fi := newFakeIssuer(t, fakeIssuerConfig{
		idToken:      testLoginIDToken(t, "joey@aalyria.com", time.Now().Add(time.Hour)),
		refreshToken: "refresh-token-1",
	})

	confDir := t.TempDir()
	app := newIssuerApp(t, fi, confDir)

	checkErr(t, app.Run([]string{"nbictl", "--config_dir", confDir, "--profile", "dev", "login"}))

	if got := app.stderr.String(); !strings.Contains(got, "expires") {
		t.Errorf("the error output %q does not say when the code expires", got)
	}
}

func TestLogin_RejectsAProfileThatIsNotOidcUser(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:         "dev",
		AuthStrategy: jwtAuthStrategy("someone@example.com", "key-1", "/tmp/key.pem"),
	}, filepath.Join(confDir, confFileName)))

	err := newTestApp().Run([]string{"nbictl", "--config_dir", confDir, "--profile", "dev", "login"})
	if err == nil {
		t.Fatal("login succeeded against a profile that uses the jwt strategy")
	}
	for _, want := range []string{"config set", "--auth_strategy oidc_user", "--issuer", "--client_id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not contain %q, so it is not a command the user can run. error: %v", want, err)
		}
	}
}

func TestResolvePerRPCCredentials_OidcUser(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	setting := &nbictlpb.Config{
		Name:         "dev",
		Url:          "nbi.example.com",
		AuthStrategy: oidcUserAuthStrategy("https://zitadel.example.com", "client-1"),
	}

	creds, err := resolvePerRPCCredentials(context.Background(), setting, confDir, nil, false, false)
	checkErr(t, err)
	if creds == nil {
		t.Fatal("resolvePerRPCCredentials returned no credentials for the oidc_user strategy")
	}
	if !creds.RequireTransportSecurity() {
		t.Error("RequireTransportSecurity: got false, want true")
	}
}

// TestResolvePerRPCCredentials_OidcUserNotLoggedIn makes sure that the error
// names the login command of the profile.
func TestResolvePerRPCCredentials_OidcUserNotLoggedIn(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	setting := &nbictlpb.Config{
		Name:         "dev",
		Url:          "nbi.example.com",
		AuthStrategy: oidcUserAuthStrategy("https://zitadel.example.com", "client-1"),
	}

	creds, err := resolvePerRPCCredentials(context.Background(), setting, confDir, nil, true, false)
	checkErr(t, err)

	ctx := credentials.NewContextWithRequestInfo(context.Background(), credentials.RequestInfo{
		Method: "/test.Service/Method",
	})
	_, err = creds.GetRequestMetadata(ctx, "https://nbi.example.com/test.Service")
	if err == nil {
		t.Fatal("GetRequestMetadata succeeded while no token is stored")
	}
	if !strings.Contains(err.Error(), "nbictl --profile=dev login") {
		t.Errorf("the error %q does not name the login command", err.Error())
	}
}
