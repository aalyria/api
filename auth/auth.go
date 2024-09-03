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
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc/credentials"
)

const (
	authHeader            = "authorization"
	proxyAuthHeader       = "proxy-authorization"
	tokenLifetime         = 1 * time.Hour
	tokenExpirationWindow = 5 * time.Minute
	proxyAudience         = "60292403139-me68tjgajl5dcdbpnlm2ek830lvsnslq.apps.googleusercontent.com"
	GoogleOIDCURL         = "https://www.googleapis.com/oauth2/v4/token"
)

var (
	// Google OIDC only supports RS256: https://accounts.google.com/.well-known/openid-configuration
	proxySigningMethod = jwt.SigningMethodRS256
	// Spacetime expects RS384
	spacetimeSigningMethod = jwt.SigningMethodRS384
)

// authCredentials is an implementation of [credentials.PerRPCCredentials].
type authCredentials struct {
	spacetimeTokenSrc, proxyTokenSrc func(context.Context) (string, error)
}

func (ac authCredentials) RequireTransportSecurity() bool { return true }

func (ac authCredentials) fetch(ctx context.Context) (stToken, proxyToken string, _ error) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	var stErr, proxyErr error
	go func() {
		defer wg.Done()
		stToken, stErr = ac.spacetimeTokenSrc(ctx)
	}()
	go func() {
		defer wg.Done()
		proxyToken, proxyErr = ac.proxyTokenSrc(ctx)
	}()

	wg.Wait()

	return stToken, proxyToken, errors.Join(stErr, proxyErr)
}

// This was copied from the grpc/credentials/oauth.TokenSource implementation.
// The gRPC version doesn't allow changing the header, whereas we need to
// support both "authorization" and "proxy-authorization".
func (ac authCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	stToken, proxyToken, err := ac.fetch(ctx)
	if err != nil {
		return nil, err
	}

	ri, _ := credentials.RequestInfoFromContext(ctx)
	if err := credentials.CheckSecurityLevel(ri.AuthInfo, credentials.PrivacyAndIntegrity); err != nil {
		return nil, fmt.Errorf("unable to transfer TokenSource PerRPCCredentials: %w", err)
	}

	return map[string]string{
		authHeader:      "Bearer " + stToken,
		proxyAuthHeader: "Bearer " + proxyToken,
	}, nil
}

type Config struct {
	Client       *http.Client
	Clock        clockwork.Clock
	PrivateKey   io.Reader
	PrivateKeyID string
	Email        string
	Host         string
}

// NewCredentials creates a [credentials.PerRPCCredentials] implementation that
// can be used to authenticate outgoing gRPC requests with Spacetime services.
func NewCredentials(ctx context.Context, c Config) (credentials.PerRPCCredentials, error) {
	errs := []error{}
	switch {
	case c.Clock == nil:
		errs = append(errs, errors.New("missing required field 'Clock'"))
	case c.Email == "":
		errs = append(errs, errors.New("missing required field 'Email'"))
	case c.PrivateKeyID == "":
		errs = append(errs, errors.New("missing required field 'PrivateKeyID'"))
	case c.PrivateKey == nil:
		errs = append(errs, errors.New("missing required field 'PrivateKey'"))
	case c.Host == "":
		errs = append(errs, errors.New("missing required field 'Host'"))
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
	pkey, err := parsePrivateKey(pkeyBlock.Bytes)
	if err != nil {
		return nil, err
	}

	var (
		stSrc, proxySrc func(context.Context) (string, error)
		stErr, proxyErr error
		wg              sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		stSrc, stErr = newSpacetimeTokenSource(ctx, c, pkey)
	}()
	go func() {
		defer wg.Done()
		proxySrc, proxyErr = newProxyTokenSource(ctx, c, pkey)
	}()
	wg.Wait()

	return authCredentials{spacetimeTokenSrc: stSrc, proxyTokenSrc: proxySrc}, errors.Join(stErr, proxyErr)
}

func newSpacetimeTokenSource(ctx context.Context, c Config, pkey any) (func(context.Context) (string, error), error) {
	return reuseToken(ctx, c, pkey, func(context.Context) (*expiringToken, error) {
		return generateNewJWT(c, pkey, spacetimeSigningMethod, nil)
	})
}

func newProxyTokenSource(ctx context.Context, c Config, pkey any) (func(context.Context) (string, error), error) {
	return reuseToken(ctx, c, pkey, func(ctx context.Context) (*expiringToken, error) {
		toExchange, err := generateNewJWT(c, pkey, proxySigningMethod, map[string]any{
			"aud":             GoogleOIDCURL,
			"target_audience": proxyAudience,
		})
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, "POST", GoogleOIDCURL, strings.NewReader((url.Values{
			"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
			"assertion":  {toExchange.tok},
		}).Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", `application/x-www-form-urlencoded`)

		resp, err := cmp.Or(c.Client, http.DefaultClient).Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		type oidcResponse struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
			IDToken          string `json:"id_token"`
		}
		r := oidcResponse{}
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, err
		}
		if r.Error != "" {
			return nil, fmt.Errorf("exchanging OIDC token: %s: %s", r.Error, r.ErrorDescription)
		}
		return &expiringToken{tok: r.IDToken, expiresAt: toExchange.expiresAt}, nil
	})
}

func reuseToken(ctx context.Context, c Config, pkey any, genToken func(context.Context) (*expiringToken, error)) (func(context.Context) (string, error), error) {
	freshToken, err := genToken(ctx)
	if err != nil {
		return nil, err
	}

	mu := &sync.Mutex{}
	return func(_ context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()

		if freshToken.isStale(c.Clock) {
			ft, err := genToken(ctx)
			if err != nil {
				return "", err
			}
			freshToken = ft
		}
		return freshToken.tok, nil
	}, nil
}

func parsePrivateKey(data []byte) (any, error) {
	var pkey any
	ok := false
	parseErrs := []error{}
	for algName, parse := range map[string]func([]byte) (any, error){
		"pkcs1": func(d []byte) (any, error) {
			k, err := x509.ParsePKCS1PrivateKey(d)
			return any(k), err
		},
		"pkcs8": x509.ParsePKCS8PrivateKey,
	} {
		k, err := parse(data)
		if err != nil {
			parseErrs = append(parseErrs, fmt.Errorf("%s: %w", algName, err))
			continue
		}

		pkey = k
		ok = true
	}

	if !ok {
		return nil, errors.Join(parseErrs...)
	}
	return pkey, nil
}

type expiringToken struct {
	expiresAt time.Time
	tok       string
}

func generateNewJWT(c Config, pkey any, signingMethod jwt.SigningMethod, extraClaims map[string]any) (*expiringToken, error) {
	now := c.Clock.Now()
	expiresAt := now.Add(tokenLifetime)

	claims := jwt.MapClaims{
		// AUDience
		"aud": c.Host,
		// Key ID
		"kid": c.PrivateKeyID,
		// ISSuer
		"iss": c.Email,
		// SUBject
		"sub": c.Email,
		// EXPires at
		"exp": jwt.NewNumericDate(expiresAt),
		// Issued AT
		"iat": jwt.NewNumericDate(now),
	}
	maps.Insert(claims, maps.All(extraClaims))

	token, err := jwt.NewWithClaims(signingMethod, claims).SignedString(pkey)
	if err != nil {
		return nil, fmt.Errorf("signing auth jwt: %w", err)
	}

	return &expiringToken{tok: token, expiresAt: expiresAt}, nil
}

func (et *expiringToken) isStale(clock clockwork.Clock) bool {
	return clock.Now().After(et.expiresAt.Add(tokenExpirationWindow))
}
