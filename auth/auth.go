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
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc/credentials"
)

const (
	authorizationHeader   = "authorization"
	tokenLifetime         = 1 * time.Hour
	tokenExpirationWindow = 5 * time.Minute
)

// authCredentials is an implementation of [credentials.PerRPCCredentials].
type authCredentials struct {
	mu     sync.RWMutex
	tokens map[string]*expiringToken
	config Config
	pkey   any
}

func (ac *authCredentials) RequireTransportSecurity() bool {
	return !ac.config.SkipTransportSecurity
}

// getToken retrieves a cached token for the given audience or creates a new one
func (ac *authCredentials) getToken(_ context.Context, audience string) (string, error) {
	ac.mu.RLock()
	token, exists := ac.tokens[audience]
	ac.mu.RUnlock()
	if exists && !token.isStale(ac.config.Clock) {
		return token.tok, nil
	}

	// Need to create a new token
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Double-check after acquiring write lock
	token, exists = ac.tokens[audience]
	if exists && !token.isStale(ac.config.Clock) {
		return token.tok, nil
	}

	// Generate new token
	now := ac.config.Clock.Now()
	expiresAt := now.Add(tokenLifetime)
	opts := JWTOptions{
		Email:        ac.config.Email,
		PrivateKeyID: ac.config.PrivateKeyID,
		Audience:     audience,
		ExpiresAt:    expiresAt,
		IssuedAt:     now,
	}

	newToken, err := CreateJWT(opts, ac.pkey)
	if err != nil {
		return "", err
	}

	ac.tokens[audience] = &expiringToken{tok: newToken, expiresAt: expiresAt}
	return newToken, nil
}

func (ac *authCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	reqInfo, ok := credentials.RequestInfoFromContext(ctx)
	if !ok {
		return nil, errors.New("failed to obtain RequestInfoFromContext")
	}
	if !ac.config.SkipTransportSecurity {
		if err := credentials.CheckSecurityLevel(reqInfo.AuthInfo, credentials.PrivacyAndIntegrity); err != nil {
			return nil, fmt.Errorf("cannot include credentials in unsafe communication: %w", err)
		}
	}

	// URI is required and must be exactly one value
	if len(uri) == 0 {
		return nil, errors.New("URI is required for authentication")
	}
	if len(uri) > 1 {
		return nil, errors.New("exactly one URI must be provided for authentication")
	}

	// Compose the audience by parsing the URI for hostname and port, but read Method from reqInfo
	// because the uri provided here is truncated off the grpcMethod part.
	// See: https://github.com/grpc/grpc-go/blob/85240a5b02defe7b653ccba66866b4370c982b6a/internal/transport/http2_client.go#L645
	// and: https://github.com/grpc/grpc-go/issues/8421
	// PS: expect `uri` to be in the form of: "https://hostname[:non-standard-port]/packageName"
	parsedUri, err := url.Parse(uri[0])
	if err != nil {
		return nil, err
	}
	// URL.Host has the form of host[:port]
	// We trim the default port `:443` if present
	audience := "https://" + strings.TrimSuffix(parsedUri.Host, ":443") + reqInfo.Method

	// Get or create token for this specific audience
	token, err := ac.getToken(ctx, audience)
	if err != nil {
		return nil, fmt.Errorf("failed to get authentication token: %w", err)
	}

	return map[string]string{
		authorizationHeader: "Bearer " + token,
	}, nil
}

type Config struct {
	Clock        clockwork.Clock
	PrivateKey   io.Reader
	PrivateKeyID string
	Email        string
	// SkipTransportSecurity disables transport security checks for testing only
	SkipTransportSecurity bool
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

	return &authCredentials{
		tokens: make(map[string]*expiringToken),
		config: c,
		pkey:   pkey,
	}, nil
}

// JWTOptions contains options for JWT creation
type JWTOptions struct {
	Email        string
	PrivateKeyID string
	Audience     string
	ExpiresAt    time.Time
	IssuedAt     time.Time
}

// CreateJWT creates a JWT token with the specified options and private key
func CreateJWT(opts JWTOptions, pkey any) (string, error) {
	claims := jwt.MapClaims{
		"iss": opts.Email,
		"sub": opts.Email,
		"iat": jwt.NewNumericDate(opts.IssuedAt),
		"exp": jwt.NewNumericDate(opts.ExpiresAt),
	}

	if opts.Audience != "" {
		claims["aud"] = opts.Audience
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if opts.PrivateKeyID != "" {
		token.Header["kid"] = opts.PrivateKeyID
	}

	tokenString, err := token.SignedString(pkey)
	if err != nil {
		return "", fmt.Errorf("signing auth jwt: %w", err)
	}

	return tokenString, nil
}

// ParsePrivateKey parses a private key from PEM-encoded data
func ParsePrivateKey(data []byte) (any, error) {
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

func (et *expiringToken) isStale(clock clockwork.Clock) bool {
	return clock.Now().After(et.expiresAt.Add(tokenExpirationWindow))
}
