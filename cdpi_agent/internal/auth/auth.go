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

package auth // import "aalyria.com/spacetime/cdpi_agent/internal/auth"

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonboulle/clockwork"
	"golang.org/x/oauth2"
	oauthjwt "golang.org/x/oauth2/jwt"
	"google.golang.org/grpc/credentials"
)

const (
	authHeader            = "authorization"
	proxyAuthHeader       = "proxy-authorization"
	tokenLifetime         = 1 * time.Hour
	tokenExpirationWindow = 5 * time.Minute
	proxyAudience         = "60292403139-me68tjgajl5dcdbpnlm2ek830lvsnslq.apps.googleusercontent.com"
	googleOIDCUrl         = "https://www.googleapis.com/oauth2/v4/token"
)

// authCredentials is an implementation of grpc/credentials.PerRPCCredentials.
type authCredentials struct {
	stTokenSrc    func() (string, error)
	proxyTokenSrc func() (string, error)
}

func (ac authCredentials) RequireTransportSecurity() bool { return true }

// This was copied from the grpc/credentials/oauth.TokenSource implementation.
// The gRPC version doesn't allow changing the header, whereas we need to
// support both "authorization" and "proxy-authorization".
func (ac authCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	stTok, stErr := ac.stTokenSrc()
	proxyTok, proxyErr := ac.proxyTokenSrc()
	if stErr != nil || proxyErr != nil {
		return nil, errors.Join(stErr, proxyErr)
	}

	ri, _ := credentials.RequestInfoFromContext(ctx)
	if err := credentials.CheckSecurityLevel(ri.AuthInfo, credentials.PrivacyAndIntegrity); err != nil {
		return nil, fmt.Errorf("unable to transfer TokenSource PerRPCCredentials: %w", err)
	}

	return map[string]string{
		authHeader:      "Bearer " + stTok,
		proxyAuthHeader: "Bearer " + proxyTok,
	}, nil
}

type Config struct {
	Clock        clockwork.Clock
	PrivateKey   io.Reader
	PrivateKeyID string
	Email        string

	oidcURL string
}

// NewCredentials creates a credentials.PerRPCCredentials implementation that
// can be used to authenticate outgoing gRPC requests with the Spacetime
// service.
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
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	if c.oidcURL == "" {
		c.oidcURL = googleOIDCUrl
	}

	pkeyBytes, err := io.ReadAll(c.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("getting private key bytes: %w", err)
	} else if len(pkeyBytes) == 0 {
		return nil, errors.New("empty private key")
	}

	stSrc, stErr := newSpacetimeTokenSource(c, pkeyBytes)
	proxySrc, proxyErr := newProxyTokenSource(ctx, c, pkeyBytes)
	if stErr != nil || proxyErr != nil {
		return nil, errors.Join(stErr, proxyErr)
	}

	return authCredentials{
		stTokenSrc:    stSrc,
		proxyTokenSrc: proxySrc,
	}, nil
}

func newProxyTokenSource(ctx context.Context, c Config, pkeyBytes []byte) (func() (string, error), error) {
	jwtConf := oauthjwt.Config{
		Email:         c.Email,
		Subject:       c.Email,
		PrivateKey:    pkeyBytes,
		PrivateKeyID:  c.PrivateKeyID,
		Expires:       tokenLifetime,
		TokenURL:      c.oidcURL,
		PrivateClaims: map[string]interface{}{"target_audience": proxyAudience},
		UseIDToken:    true,
	}

	ts := (&jwtConf).TokenSource(ctx)
	initToken, err := ts.Token()
	if err != nil {
		return nil, err
	}
	// wrap the token source in a caching layer
	ts = oauth2.ReuseTokenSource(initToken, ts)

	return func() (string, error) {
		t, err := ts.Token()
		if err != nil {
			return "", err
		}
		return t.AccessToken, nil
	}, nil
}

func newSpacetimeTokenSource(c Config, pkeyBytes []byte) (func() (string, error), error) {
	pkeyBlock, _ := pem.Decode(pkeyBytes)
	if pkeyBlock == nil {
		return nil, errors.New("PrivateKey not PEM-encoded")
	}

	pkey, err := parsePrivateKey(pkeyBlock.Bytes)
	if err != nil {
		return nil, err
	}

	freshToken, err := generateNewJWT(c, pkey)
	if err != nil {
		return nil, err
	}

	mu := &sync.Mutex{}
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()

		if freshToken.isStale(c.Clock) {
			ft, err := generateNewJWT(c, pkey)
			if err != nil {
				return "", err
			}
			freshToken = ft
		}

		return freshToken.tok, nil
	}, nil
}

func parsePrivateKey(data []byte) (interface{}, error) {
	var pkey interface{}
	ok := false
	parseErrs := []error{}
	for algName, parse := range map[string]func([]byte) (interface{}, error){
		"pkcs1": func(d []byte) (interface{}, error) {
			k, err := x509.ParsePKCS1PrivateKey(d)
			return interface{}(k), err
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

func generateNewJWT(c Config, pkey interface{}) (*expiringToken, error) {
	now := c.Clock.Now()
	expiresAt := now.Add(tokenLifetime)

	token, err := jwt.NewWithClaims(jwt.SigningMethodPS512, jwt.MapClaims{
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
	}).SignedString(pkey)
	if err != nil {
		return nil, fmt.Errorf("signing auth jwt: %w", err)
	}

	return &expiringToken{
		tok:       token,
		expiresAt: expiresAt,
	}, nil
}

func (et *expiringToken) isStale(clock clockwork.Clock) bool {
	return clock.Now().After(et.expiresAt.Add(tokenExpirationWindow))
}
