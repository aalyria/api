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

package auth // import "aalyria.com/spacetime/auth"

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonboulle/clockwork"
	"golang.org/x/oauth2"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc/credentials"
)

// providerMetadata is the subset of the OIDC discovery document that nbictl
// reads. The field names are the discovery document's own, so [discover]
// decodes the response straight into it.
type providerMetadata struct {
	Issuer                      string `json:"issuer"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	// RevocationEndpoint is empty when the provider advertises none.
	RevocationEndpoint string `json:"revocation_endpoint"`
}

// discover reads the OpenID Connect discovery document of the given issuer.
// The issuer must be an https URL. A nil client selects [http.DefaultClient].
//
// The client is an [HTTPDoer] rather than an [http.Client] so that discovery
// travels the same path as every other request its caller makes. Discovery only
// needs Do. A caller that injects a fake, or a Doer that pins a certificate or
// carries mTLS, would otherwise find its exchange going through that Doer while
// discovery reached past it to [http.DefaultClient].
func discover(ctx context.Context, issuer string, client HTTPDoer) (*providerMetadata, error) {
	if !strings.HasPrefix(issuer, "https://") {
		return nil, fmt.Errorf("the issuer %q must start with https://", issuer)
	}
	// httpDoerOrDefault and not cmp.Or: a nil *http.Client held in an interface
	// is not a nil interface, so cmp.Or would pass it through and Do would
	// panic on it.
	client = httpDoerOrDefault(client)

	docURL := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating the discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", docURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("reading the discovery document from %s: %w", docURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the discovery endpoint %s returned status %d: %s", docURL, resp.StatusCode, truncateBody(body))
	}

	doc := &providerMetadata{}
	if err := json.Unmarshal(body, doc); err != nil {
		return nil, fmt.Errorf("parsing the discovery document from %s: %w", docURL, err)
	}

	if strings.TrimSuffix(doc.Issuer, "/") != strings.TrimSuffix(issuer, "/") {
		return nil, fmt.Errorf("the discovery document reports issuer %q, which does not match the configured issuer %q", doc.Issuer, issuer)
	}
	if doc.TokenEndpoint == "" {
		return nil, fmt.Errorf("the discovery document from %s has no token_endpoint", docURL)
	}

	// RFC 8414 requires https endpoints. The refresh token and the ID token
	// travel to these URLs, so an http one would put them on the wire in
	// cleartext.
	for _, endpoint := range []struct{ name, value string }{
		{name: "token_endpoint", value: doc.TokenEndpoint},
		{name: "device_authorization_endpoint", value: doc.DeviceAuthorizationEndpoint},
		{name: "revocation_endpoint", value: doc.RevocationEndpoint},
	} {
		if endpoint.value != "" && !strings.HasPrefix(endpoint.value, "https://") {
			return nil, fmt.Errorf("the discovery document from %s gives the %s %q, which must start with https://", docURL, endpoint.name, endpoint.value)
		}
	}

	return doc, nil
}

// maxResponseBodyBytes caps how much of a provider's response is read at all.
const maxResponseBodyBytes = 1 << 20

// maxErrorBodyBytes limits how much of a provider's response body reaches an
// error message. A body can echo the request parameters, which is the one
// plausible route by which a token could reach a log line.
const maxErrorBodyBytes = 512

func truncateBody(body []byte) string {
	if len(body) <= maxErrorBodyBytes {
		return string(body)
	}
	return string(body[:maxErrorBodyBytes]) + "... (truncated)"
}

// defaultOIDCUserScopes are the scopes that the device login requests.
// offline_access asks for a refresh token, without which the user must repeat
// the device flow every hour.
var defaultOIDCUserScopes = []string{"openid", "profile", "email", "offline_access"}

// Tokens are the credentials that a user login produces. Expiry is read from
// the exp claim of the ID token.
type Tokens struct {
	IDToken      string
	RefreshToken string
	Expiry       time.Time
	// Email is the email claim of the ID token. It is not persisted, so a
	// [TokenStore] returns it empty; only a fresh login or refresh sets it.
	Email string
}

// DevicePrompt is what the user must be shown to complete a device login.
type DevicePrompt struct {
	VerificationURI string
	// VerificationURIComplete embeds the user code. It is empty when the
	// provider supplies none.
	VerificationURIComplete string
	UserCode                string
	ExpiresAt               time.Time
}

// OIDCUserConfig configures an interactive OIDC user login.
type OIDCUserConfig struct {
	Clock    clockwork.Clock
	Issuer   string
	ClientID string
	// HTTPClient is optional. Tests inject one.
	HTTPClient *http.Client
	// LoginHint is the command that the user must run to log in. It appears
	// in the error that an RPC gets when no token is stored.
	LoginHint string
	// SkipTransportSecurity disables transport security checks for testing
	// only.
	SkipTransportSecurity bool
}

func (c OIDCUserConfig) oauth2Config(meta *providerMetadata) *oauth2.Config {
	return &oauth2.Config{
		ClientID: c.ClientID,
		Scopes:   defaultOIDCUserScopes,
		Endpoint: oauth2.Endpoint{
			DeviceAuthURL: meta.DeviceAuthorizationEndpoint,
			TokenURL:      meta.TokenEndpoint,
			AuthStyle:     oauth2.AuthStyleInParams,
		},
	}
}

// withHTTPClient gives the oauth2 package the configured HTTP client.
func (c OIDCUserConfig) withHTTPClient(ctx context.Context) context.Context {
	if c.HTTPClient == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, c.HTTPClient)
}

// idTokenClaims are the email and exp claims of an ID token.
type idTokenClaims struct {
	Email     string
	ExpiresAt time.Time
}

// parseIDTokenClaims reads the email and exp claims of an ID token. It does
// not verify the signature; the Spacetime server does that.
func parseIDTokenClaims(idToken string) (*idTokenClaims, error) {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(idToken, claims); err != nil {
		return nil, fmt.Errorf("parsing the ID token: %w", err)
	}

	expiresAt, err := claims.GetExpirationTime()
	if err != nil {
		return nil, fmt.Errorf("reading the exp claim of the ID token: %w", err)
	}
	if expiresAt == nil {
		return nil, errors.New("the ID token has no exp claim")
	}

	email, _ := claims["email"].(string)
	return &idTokenClaims{Email: email, ExpiresAt: expiresAt.Time}, nil
}

// tokensFromOAuth2 extracts the ID token and its claims from a token
// response.
func tokensFromOAuth2(tok *oauth2.Token) (*Tokens, error) {
	idToken, _ := tok.Extra("id_token").(string)
	if idToken == "" {
		return nil, errors.New("the identity provider returned no ID token; make sure the openid scope is granted and that the application's Auth Token Type is JWT")
	}

	claims, err := parseIDTokenClaims(idToken)
	if err != nil {
		return nil, err
	}
	if claims.Email == "" {
		return nil, errors.New("the ID token has no email claim; enable the user profile information in the ID token in the identity provider's application settings")
	}

	return &Tokens{
		IDToken:      idToken,
		RefreshToken: tok.RefreshToken,
		Expiry:       claims.ExpiresAt,
		Email:        claims.Email,
	}, nil
}

var (
	// ErrNotLoggedIn reports that no token is stored for the profile.
	ErrNotLoggedIn = errors.New("not logged in")
	// ErrSessionExpired reports that the stored session can no longer
	// produce a valid ID token.
	ErrSessionExpired = errors.New("your session expired")
	// ErrNoRevocationEndpoint reports that the provider advertises no RFC 7009
	// revocation endpoint.
	ErrNoRevocationEndpoint = errors.New("the identity provider advertises no revocation endpoint")
	// ErrCodeExpired reports that the user did not approve the device login
	// before the code expired.
	ErrCodeExpired = errors.New("the login code expired")
)

// loginAgainHint names the command that starts a new login. It is empty when
// the caller supplied no hint, so that the auth package never invents a
// command name of its own.
func (c OIDCUserConfig) loginAgainHint() string {
	if c.LoginHint == "" {
		return ""
	}
	return fmt.Sprintf(". Run `%s` again", c.LoginHint)
}

// Revoke asks the provider to invalidate a refresh token (RFC 7009).
func Revoke(ctx context.Context, c OIDCUserConfig, refreshToken string) error {
	meta, err := discover(ctx, c.Issuer, c.HTTPClient)
	if err != nil {
		return err
	}
	if meta.RevocationEndpoint == "" {
		return ErrNoRevocationEndpoint
	}

	form := url.Values{
		"token":           {refreshToken},
		"token_type_hint": {"refresh_token"},
		"client_id":       {c.ClientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, meta.RevocationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("creating the revocation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := cmp.Or(c.HTTPClient, http.DefaultClient)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending the revocation request to %s: %w", meta.RevocationEndpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return fmt.Errorf("reading the revocation response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("the revocation endpoint %s returned status %d: %s", meta.RevocationEndpoint, resp.StatusCode, truncateBody(body))
	}
	return nil
}

// TokenStore persists tokens between nbictl invocations. Load returns
// (nil, nil) when the user is not logged in.
type TokenStore interface {
	Load() (*Tokens, error)
	Save(*Tokens) error
}

// NewOIDCUserCredentials creates a [credentials.PerRPCCredentials]
// implementation that sends the stored OIDC ID token, and refreshes it
// silently when it nears its expiry.
func NewOIDCUserCredentials(_ context.Context, c OIDCUserConfig, store TokenStore) (credentials.PerRPCCredentials, error) {
	errs := []error{}
	if c.Clock == nil {
		errs = append(errs, errors.New("missing required field 'Clock'"))
	}
	if c.Issuer == "" {
		errs = append(errs, errors.New("missing required field 'Issuer'"))
	}
	if c.ClientID == "" {
		errs = append(errs, errors.New("missing required field 'ClientID'"))
	}
	if store == nil {
		errs = append(errs, errors.New("missing required token store"))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &oidcUserCredentials{config: c, store: store}, nil
}

type oidcUserCredentials struct {
	// cached serves the RPC path without touching the file. oidcCredentials
	// caches the same way. It is only ever consulted while the token is
	// valid, so a refresh by another process is still picked up the moment
	// the cached token goes stale.
	cached atomic.Pointer[Tokens]
	sf     singleflight.Group
	config OIDCUserConfig
	store  TokenStore
}

// use caches the tokens and returns the ID token to send.
func (uc *oidcUserCredentials) use(tokens *Tokens) string {
	uc.cached.Store(tokens)
	return tokens.IDToken
}

func (uc *oidcUserCredentials) RequireTransportSecurity() bool {
	return !uc.config.SkipTransportSecurity
}

func (uc *oidcUserCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	reqInfo, ok := credentials.RequestInfoFromContext(ctx)
	if !ok {
		return nil, errors.New("failed to obtain RequestInfoFromContext")
	}
	if uc.RequireTransportSecurity() {
		if err := credentials.CheckSecurityLevel(reqInfo.AuthInfo, credentials.PrivacyAndIntegrity); err != nil {
			return nil, fmt.Errorf("cannot include credentials in unsafe communication: %w", err)
		}
	}

	token, err := uc.getToken(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		authorizationHeader: "Bearer " + token,
	}, nil
}

// isStale reports whether the ID token expires within tokenExpirationWindow.
func (uc *oidcUserCredentials) isStale(tokens *Tokens) bool {
	return isStale(uc.config.Clock, tokens.Expiry)
}

func (uc *oidcUserCredentials) notLoggedIn() error {
	if uc.config.LoginHint == "" {
		return ErrNotLoggedIn
	}
	return fmt.Errorf("%w: run `%s`", ErrNotLoggedIn, uc.config.LoginHint)
}

// sessionExpired reports that the stored session can no longer produce a
// valid ID token, and names the command that starts a new one.
func (uc *oidcUserCredentials) sessionExpired(cause error) error {
	return fmt.Errorf("%w: %w%s", ErrSessionExpired, cause, uc.config.loginAgainHint())
}

// load reads the stored tokens, and reports the not-logged-in error when the
// store holds none.
func (uc *oidcUserCredentials) load() (*Tokens, error) {
	tokens, err := uc.store.Load()
	if err != nil {
		return nil, fmt.Errorf("reading the stored tokens: %w", err)
	}
	if tokens == nil {
		return nil, uc.notLoggedIn()
	}
	return tokens, nil
}

func (uc *oidcUserCredentials) getToken(ctx context.Context) (string, error) {
	if tokens := uc.cached.Load(); tokens != nil && !uc.isStale(tokens) {
		return tokens.IDToken, nil
	}

	tokens, err := uc.load()
	if err != nil {
		return "", err
	}
	if !uc.isStale(tokens) {
		return uc.use(tokens), nil
	}
	if tokens.RefreshToken == "" {
		return "", uc.sessionExpired(errors.New("there is no refresh token"))
	}

	// nbictl sends concurrent RPCs, so collapse the refreshes into one. The
	// refresh runs under the first caller's context, so cancelling that RPC
	// fails the RPCs that are waiting on the same refresh.
	result, err, _ := uc.sf.Do("refresh", func() (any, error) {
		tokens, err := uc.load()
		if err != nil {
			return nil, err
		}
		if !uc.isStale(tokens) {
			return uc.use(tokens), nil
		}

		refreshed, err := uc.refresh(ctx, tokens.RefreshToken)
		if err != nil {
			// Providers that rotate refresh tokens, Zitadel among them,
			// invalidate the old one. A second nbictl process that refreshed
			// first therefore leaves this one holding a dead token. Prefer
			// what that process stored over reporting a healthy session as
			// expired.
			if stored, loadErr := uc.store.Load(); loadErr == nil && stored != nil && !uc.isStale(stored) {
				return uc.use(stored), nil
			}
			return nil, err
		}
		if err := uc.store.Save(refreshed); err != nil {
			return nil, fmt.Errorf("saving the refreshed tokens: %w", err)
		}
		return uc.use(refreshed), nil
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

func (uc *oidcUserCredentials) refresh(ctx context.Context, refreshToken string) (*Tokens, error) {
	meta, err := discover(ctx, uc.config.Issuer, uc.config.HTTPClient)
	if err != nil {
		return nil, uc.sessionExpired(err)
	}

	ctx = uc.config.withHTTPClient(ctx)
	tok, err := uc.config.oauth2Config(meta).TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err != nil {
		return nil, uc.sessionExpired(err)
	}

	tokens, err := tokensFromOAuth2(tok)
	if err != nil {
		return nil, uc.sessionExpired(err)
	}
	return tokens, nil
}

// DeviceLogin runs the OAuth 2.0 device authorization grant (RFC 8628). It
// calls prompt once with the instructions that the user must follow, and then
// it polls the token endpoint until the user approves the login, the code
// expires, or ctx is cancelled.
func DeviceLogin(ctx context.Context, c OIDCUserConfig, prompt func(DevicePrompt) error) (*Tokens, error) {
	meta, err := discover(ctx, c.Issuer, c.HTTPClient)
	if err != nil {
		return nil, err
	}
	if meta.DeviceAuthorizationEndpoint == "" {
		return nil, fmt.Errorf("the issuer %s does not support the OAuth 2.0 device authorization grant", c.Issuer)
	}

	ctx = c.withHTTPClient(ctx)
	cfg := c.oauth2Config(meta)

	deviceAuth, err := cfg.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("requesting a device code from %s: %w", meta.DeviceAuthorizationEndpoint, err)
	}

	if err := prompt(DevicePrompt{
		VerificationURI:         deviceAuth.VerificationURI,
		VerificationURIComplete: deviceAuth.VerificationURIComplete,
		UserCode:                deviceAuth.UserCode,
		ExpiresAt:               deviceAuth.Expiry,
	}); err != nil {
		return nil, err
	}

	tok, err := cfg.DeviceAccessToken(ctx, deviceAuth)
	if err != nil {
		return nil, c.deviceAccessTokenError(err)
	}
	return tokensFromOAuth2(tok)
}

// deviceAccessTokenError names the next action when a device login ends
// without an approval. The oauth2 package reports an expiry two ways: as the
// OAuth expired_token code, or, once it stops polling at the code's own
// expiry, as a deadline on the context. A cancelled context is the user
// pressing Ctrl-C, which is not an expiry.
func (c OIDCUserConfig) deviceAccessTokenError(err error) error {
	expired := errors.Is(err, context.DeadlineExceeded)
	retrieveErr := &oauth2.RetrieveError{}
	if errors.As(err, &retrieveErr) && retrieveErr.ErrorCode == "expired_token" {
		expired = true
	}
	if !expired {
		return fmt.Errorf("waiting for the login to complete: %w", err)
	}
	return fmt.Errorf("%w: %w%s", ErrCodeExpired, err, c.loginAgainHint())
}
