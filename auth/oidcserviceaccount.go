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

package auth // import "aalyria.com/spacetime/auth"

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc/credentials"
)

const (
	// jwtBearerGrantType is the RFC 7523 grant that exchanges a private-key
	// assertion for an access token.
	jwtBearerGrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"

	// serviceAccountAssertionBackdate moves the assertion's iat into the
	// past. The identity provider accepts an iat that is only about two
	// seconds ahead of its own clock, so a machine whose clock runs fast
	// cannot exchange an assertion that is stamped with the present moment.
	// Backdating costs nothing, because the provider accepts an assertion
	// that is up to an hour old.
	serviceAccountAssertionBackdate = 30 * time.Second

	// serviceAccountAssertionLifetime is how long the assertion stays valid.
	// It is spent immediately, so it needs to outlive one round trip and no
	// more.
	serviceAccountAssertionLifetime = 5 * time.Minute

	// serviceAccountExchangeTimeout bounds one exchange. The exchange is
	// detached from its caller's context, so it needs a deadline of its own or
	// an unresponsive provider would hold every waiting caller forever.
	serviceAccountExchangeTimeout = 30 * time.Second

	// maxServiceAccountTokenLifetime caps the lifetime that expires_in may
	// claim. A value large enough to overflow a [time.Duration] wraps
	// negative, which puts the expiry in the past and re-exchanges on every
	// RPC. Capping a longer-lived token only exchanges it more often than the
	// provider requires.
	maxServiceAccountTokenLifetime = 24 * time.Hour
)

// serviceAccountScopes are the scopes that the token exchange requests. The
// audience scope puts the project into the access token's aud claim, and the
// roles scope puts the granted roles into it. Either one alone yields a token
// with no roles, so both are required. The singular "project:id" of the first
// and the plural "projects" of the second are the identity provider's own
// spellings, not a mistake.
func serviceAccountScopes(projectID string) []string {
	return []string{
		"openid",
		"urn:zitadel:iam:org:project:id:" + projectID + ":aud",
		"urn:zitadel:iam:org:projects:roles",
	}
}

// ServiceAccountToken is an access token and the moment it expires. It is
// what a [ServiceAccountTokenStore] persists.
type ServiceAccountToken struct {
	AccessToken string
	ExpiresAt   time.Time
}

// ServiceAccountTokenStore keeps an access token between processes, so that a
// short-lived command reuses the token that an earlier one obtained. A command
// exits long before a 12-hour token expires, so without a store every
// invocation of a script that loops over one command pays for its own
// exchange.
//
// Load reports (nil, nil) when it holds nothing usable. An implementation
// decides for itself what "usable" means: a token minted for a different
// issuer, project or key is not interchangeable with this one, so a store that
// cannot tell them apart must report nothing rather than the wrong token.
type ServiceAccountTokenStore interface {
	Load() (*ServiceAccountToken, error)
	Save(*ServiceAccountToken) error
}

// OIDCServiceAccountConfig configures the RFC 7523 private-key exchange that
// authenticates a machine as an identity provider service account. The caller
// supplies the fields of a service account key file; this package never reads
// that file, because its format belongs to the provider and not to the
// protocol.
type OIDCServiceAccountConfig struct {
	Clock clockwork.Clock
	// Issuer is the identity provider's https URL. Discovery finds the token
	// endpoint, so no path is hardcoded here.
	Issuer string
	// ProjectID names the Spacetime project. It appears in both of the
	// project-specific scopes, and it decides the name of the roles claim
	// that the access token carries.
	ProjectID string
	// KeyID is the key file's keyId. It becomes the assertion's kid header,
	// which is how the provider finds the public key that verifies the
	// signature.
	KeyID string
	// UserID is the key file's userId. It becomes both the iss and the sub
	// claim of the assertion.
	UserID string
	// PrivateKey is the PEM-encoded private key of the key file.
	PrivateKey io.Reader
	// HTTPClient is optional. Tests inject one.
	HTTPClient HTTPDoer
	// TokenStore is optional. When it is nil the access token lives only for
	// as long as the process does.
	TokenStore ServiceAccountTokenStore
	// SkipTransportSecurity disables transport security checks for testing
	// only.
	SkipTransportSecurity bool
}

// NewOIDCServiceAccountCredentials creates a
// [credentials.PerRPCCredentials] implementation that signs an RFC 7523
// assertion with a service account's private key, exchanges it at the
// provider's token endpoint for an access token, and sends that token on
// every request. There is no interactive step and nothing is written to disk:
// the access token lives in memory for as long as the process does.
func NewOIDCServiceAccountCredentials(_ context.Context, c OIDCServiceAccountConfig) (credentials.PerRPCCredentials, error) {
	// Report every missing field at once. An operator who fills in a config
	// one error at a time pays a round trip for each.
	errs := []error{}
	if c.Clock == nil {
		errs = append(errs, errors.New("missing required field 'Clock'"))
	}
	if c.Issuer == "" {
		errs = append(errs, errors.New("missing required field 'Issuer'"))
	}
	if c.ProjectID == "" {
		errs = append(errs, errors.New("missing required field 'ProjectID'"))
	}
	if c.KeyID == "" {
		errs = append(errs, errors.New("missing required field 'KeyID'"))
	}
	if c.UserID == "" {
		errs = append(errs, errors.New("missing required field 'UserID'"))
	}
	if c.PrivateKey == nil {
		errs = append(errs, errors.New("missing required field 'PrivateKey'"))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	pkeyBytes, err := io.ReadAll(c.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("getting private key bytes: %w", err)
	} else if len(pkeyBytes) == 0 {
		return nil, errors.New("empty private key")
	}

	pkeyBlock, _ := pem.Decode(pkeyBytes)
	if pkeyBlock == nil {
		return nil, errors.New("PrivateKey not PEM-encoded")
	}
	pkey, err := ParsePrivateKey(pkeyBlock.Bytes)
	if err != nil {
		return nil, err
	}

	client := httpDoerOrDefault(c.HTTPClient)
	return &oidcServiceAccountCredentials{
		config:         c,
		pkey:           pkey,
		client:         client,
		exchangeClient: withoutRedirects(client),
	}, nil
}

// withoutRedirects returns a Doer whose [http.Client] does not follow
// redirects. It sets CheckRedirect to [http.ErrUseLastResponse], the standard
// library's sentinel for "return the 3xx response instead of following it".
// That sentinel is the whole mechanism.
//
// RFC 6749 section 5.1 defines a token response as a 200 whose body carries
// the parameters, so a redirect has no part in the exchange, and OAuth
// libraries leave the redirect policy to the HTTP client. Go's default policy
// follows a redirect: it replays the POST body on a 307 or a 308, and it
// permits an https to http downgrade. That body holds the signed assertion,
// which is a bearer credential. Refusing the redirect keeps the assertion off
// every host except the discovered token endpoint, and it preserves the https
// check that [discover] makes on that endpoint. The caller turns the returned
// 3xx into an error that names the Location.
//
// The client is copied rather than mutated, so a caller's own client keeps its
// policy. A Doer that is not an [http.Client] comes back unchanged, because
// only an http.Client has a redirect policy.
func withoutRedirects(doer HTTPDoer) HTTPDoer {
	client, ok := doer.(*http.Client)
	if !ok {
		return doer
	}
	copied := *client
	copied.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copied
}

type oidcServiceAccountCredentials struct {
	token  atomic.Pointer[expiringToken]
	sf     singleflight.Group
	config OIDCServiceAccountConfig
	pkey   any
	// client serves discovery, which is a public document over TLS.
	client HTTPDoer
	// exchangeClient serves the token request, which carries the assertion.
	exchangeClient HTTPDoer
}

func (sc *oidcServiceAccountCredentials) RequireTransportSecurity() bool {
	return !sc.config.SkipTransportSecurity
}

func (sc *oidcServiceAccountCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	reqInfo, ok := credentials.RequestInfoFromContext(ctx)
	if !ok {
		return nil, errors.New("failed to obtain RequestInfoFromContext")
	}
	if sc.RequireTransportSecurity() {
		if err := credentials.CheckSecurityLevel(reqInfo.AuthInfo, credentials.PrivacyAndIntegrity); err != nil {
			return nil, fmt.Errorf("cannot include credentials in unsafe communication: %w", err)
		}
	}

	token, err := sc.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get the service account access token: %w", err)
	}

	return map[string]string{
		authorizationHeader: "Bearer " + token,
	}, nil
}

// getToken returns the cached access token, and exchanges a new one when
// neither the process nor the store holds one that is still fresh.
func (sc *oidcServiceAccountCredentials) getToken(ctx context.Context) (string, error) {
	if token := sc.token.Load(); token != nil && !token.isStale(sc.config.Clock) {
		return token.tok, nil
	}

	// Collapse concurrent exchanges into one. The exchange is detached from
	// the context of whichever caller started it and given a deadline of its
	// own, because every caller waits on the same call: at cold start every
	// RPC misses the cache at once, and one of them giving up would otherwise
	// fail all the others with a misleading "context canceled".
	//
	// Each caller still waits under its own context, so giving up returns.
	// The exchange it abandons carries on and fills the cache for whoever is
	// still waiting.
	ch := sc.sf.DoChan("token", func() (any, error) {
		if token := sc.token.Load(); token != nil && !token.isStale(sc.config.Clock) {
			return token.tok, nil
		}
		if token := sc.loadStoredToken(); token != nil {
			sc.token.Store(token)
			return token.tok, nil
		}
		exchangeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), serviceAccountExchangeTimeout)
		defer cancel()
		accessToken, expiresAt, err := sc.exchangeToken(exchangeCtx)
		if err != nil {
			return nil, err
		}
		sc.token.Store(&expiringToken{tok: accessToken, expiresAt: expiresAt})
		sc.storeToken(accessToken, expiresAt)
		return accessToken, nil
	})

	select {
	case <-ctx.Done():
		return "", context.Cause(ctx)
	case result := <-ch:
		if result.Err != nil {
			return "", result.Err
		}
		return result.Val.(string), nil
	}
}

// loadStoredToken returns the stored access token when the store holds one
// that is still fresh, and nil otherwise.
//
// A store fault is a miss, not an error. The cache only saves a round trip, so
// a cache that cannot be read must cost that round trip and nothing more.
// Failing here would let one unreadable file stop every command instead.
func (sc *oidcServiceAccountCredentials) loadStoredToken() *expiringToken {
	if sc.config.TokenStore == nil {
		return nil
	}
	stored, err := sc.config.TokenStore.Load()
	if err != nil || stored == nil || stored.AccessToken == "" {
		return nil
	}
	token := &expiringToken{tok: stored.AccessToken, expiresAt: stored.ExpiresAt}
	if token.isStale(sc.config.Clock) {
		return nil
	}
	return token
}

// storeToken offers a freshly exchanged token to the store.
//
// The error is discarded for the same reason [loadStoredToken] discards its
// own: the token in hand is valid and the RPC must proceed. A store reports a
// write it could not make; this package has nowhere to report it to.
func (sc *oidcServiceAccountCredentials) storeToken(accessToken string, expiresAt time.Time) {
	if sc.config.TokenStore == nil {
		return
	}
	_ = sc.config.TokenStore.Save(&ServiceAccountToken{AccessToken: accessToken, ExpiresAt: expiresAt})
}

// exchangeToken signs an assertion and trades it for an access token.
func (sc *oidcServiceAccountCredentials) exchangeToken(ctx context.Context) (string, time.Time, error) {
	meta, err := discover(ctx, sc.config.Issuer, sc.client)
	if err != nil {
		return "", time.Time{}, err
	}

	now := sc.config.Clock.Now()
	// The audience is the issuer, not the token endpoint. The identity
	// provider refuses an assertion that names the endpoint.
	assertion, err := CreateJWT(JWTOptions{
		Issuer:       sc.config.UserID,
		Subject:      sc.config.UserID,
		Audience:     strings.TrimSuffix(sc.config.Issuer, "/"),
		PrivateKeyID: sc.config.KeyID,
		IssuedAt:     now.Add(-serviceAccountAssertionBackdate),
		ExpiresAt:    now.Add(serviceAccountAssertionLifetime),
		JTI:          uuid.NewString(),
	}, sc.pkey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("creating the service account assertion: %w", err)
	}

	form := url.Values{
		"grant_type": {jwtBearerGrantType},
		"assertion":  {assertion},
		"scope":      {strings.Join(serviceAccountScopes(sc.config.ProjectID), " ")},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, meta.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("creating the token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := sc.exchangeClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sending the token request to %s: %w", meta.TokenEndpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("reading the token response from %s: %w", meta.TokenEndpoint, err)
	}

	if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
		// A redirect carries no body worth reporting, so without this the
		// operator sees a bare status number and no cause.
		return "", time.Time{}, fmt.Errorf("the token endpoint %s answered with a redirect to %q, which was not followed;"+
			" the assertion is a credential, and a redirect can send it to a cleartext or third-party destination",
			meta.TokenEndpoint, resp.Header.Get("Location"))
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// A token endpoint can echo the request parameters, and the assertion
		// is a credential, so the body is truncated before it reaches an
		// error.
		return "", time.Time{}, fmt.Errorf("the token endpoint %s returned status %d: %s%s",
			meta.TokenEndpoint, resp.StatusCode, truncateBody(body), invalidGrantCauses(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("parsing the token response from %s: %w", meta.TokenEndpoint, err)
	}
	if tokenResp.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("the token response from %s has no access_token", meta.TokenEndpoint)
	}
	if err := checkAccessTokenIsAJWT(tokenResp.AccessToken); err != nil {
		return "", time.Time{}, err
	}

	return tokenResp.AccessToken, now.Add(accessTokenLifetime(tokenResp.ExpiresIn)), nil
}

// accessTokenLifetime turns an expires_in value in seconds into a lifetime.
// The multiplication is done on a capped value, because seconds that overflow
// a [time.Duration] wrap negative. A missing or non-positive expires_in falls
// back to tokenLifetime, which every strategy in this package shares.
func accessTokenLifetime(expiresIn int64) time.Duration {
	switch {
	case expiresIn <= 0:
		return tokenLifetime
	case expiresIn >= int64(maxServiceAccountTokenLifetime/time.Second):
		return maxServiceAccountTokenLifetime
	default:
		return time.Duration(expiresIn) * time.Second
	}
}

// invalidGrantCauses names every cause of an invalid_grant response. The
// identity provider reports all of them with the same code and the same
// description, so an operator cannot tell them apart from the response alone.
// The returned string is empty when the response is not an invalid_grant.
func invalidGrantCauses(body []byte) string {
	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err != nil || errResp.Error != "invalid_grant" {
		return ""
	}
	return ". The identity provider reports the same error for every cause:" +
		" this machine's clock is more than about 2 seconds ahead of the provider's," +
		" the private key does not belong to the service account," +
		" the UserID names a different account," +
		" or the assertion is more than 1 hour old"
}

// checkAccessTokenIsAJWT refuses an opaque access token. The Spacetime
// services verify a signed token, so an opaque one would be refused several
// services away from the setting that produced it.
func checkAccessTokenIsAJWT(accessToken string) error {
	segments := strings.Split(accessToken, ".")
	if len(segments) == 3 && segments[0] != "" && segments[1] != "" && segments[2] != "" {
		return nil
	}
	return errors.New("the identity provider returned an opaque access token, which Spacetime cannot verify;" +
		" set the service account's Access Token Type to JWT")
}
