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
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc/credentials"
)

var testKey = generateRSAPrivateKey()

type badReader struct{ err error }

func (b badReader) Read(_ []byte) (int, error) { return 0, b.err }

func TestNewCredentials_validation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		want string
		c    Config
	}{
		{
			name: "missing email",
			want: "missing required field 'Email'",
			c: Config{
				Email:        "",
				PrivateKey:   bytes.NewBuffer(testKey.privatePEM),
				PrivateKeyID: "1",
				Clock:        clockwork.NewRealClock(),
			},
		},
		{
			name: "missing private key ID",
			want: "missing required field 'PrivateKeyID'",
			c: Config{
				Email:        "some@example.com",
				PrivateKey:   bytes.NewBuffer(testKey.privatePEM),
				PrivateKeyID: "",
				Clock:        clockwork.NewRealClock(),
			},
		},
		{
			name: "missing clock",
			want: "missing required field 'Clock'",
			c: Config{
				Email:        "some@example.com",
				PrivateKey:   bytes.NewBuffer(testKey.privatePEM),
				PrivateKeyID: "1",
				Clock:        nil,
			},
		},
		{
			name: "bad reader for private key",
			want: "getting private key bytes: read went wrong!",
			c: Config{
				Email:        "some@example.com",
				PrivateKey:   badReader{errors.New("read went wrong!")},
				PrivateKeyID: "1",
				Clock:        clockwork.NewRealClock(),
			},
		},
		{
			name: "empty private key",
			want: "empty private key",
			c: Config{
				Email:        "some@example.com",
				PrivateKey:   bytes.NewBuffer([]byte{}),
				PrivateKeyID: "1",
				Clock:        clockwork.NewRealClock(),
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			_, err := NewCredentials(ctx, tc.c)
			if got := cmp.Or(err, errors.New("")).Error(); got != tc.want {
				t.Errorf("unexpected validation error: got %q, but expected %q", got, tc.want)
			}
		})
	}
}

func TestRequireTransportSecurity_Default(t *testing.T) {
	t.Parallel()

	conf := Config{
		Email:        "some@example.com",
		PrivateKey:   bytes.NewBuffer(testKey.privatePEM),
		PrivateKeyID: "1",
		Clock:        clockwork.NewFakeClockAt(time.Date(2011, time.February, 16, 0, 0, 0, 0, time.UTC)),
	}

	creds, err := NewCredentials(context.Background(), conf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !creds.RequireTransportSecurity() {
		t.Errorf("credentials should enforce transport security by default")
	}
}

func TestRequireTransportSecurity_Skip(t *testing.T) {
	t.Parallel()

	conf := Config{
		Email:                 "some@example.com",
		PrivateKey:            bytes.NewBuffer(testKey.privatePEM),
		PrivateKeyID:          "1",
		Clock:                 clockwork.NewFakeClockAt(time.Date(2011, time.February, 16, 0, 0, 0, 0, time.UTC)),
		SkipTransportSecurity: true,
	}

	creds, err := NewCredentials(context.Background(), conf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.RequireTransportSecurity() {
		t.Errorf("credentials should not enforce transport security when SkipTransportSecurity is true")
	}
}

type rsaKeyForTesting struct {
	privateKey *rsa.PrivateKey
	privatePEM []byte
	publicPEM  []byte
}

func generateRSAPrivateKey() rsaKeyForTesting {
	bitSize := 2048
	// Generate RSA key.
	key, err := rsa.GenerateKey(rand.Reader, bitSize)
	if err != nil {
		panic(err)
	}

	// Extract public component.
	pub := key.Public()

	// Encode private key to PKCS#1 ASN.1 PEM.
	keyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		},
	)

	// Encode public key to PKCS#1 ASN.1 PEM.
	pubPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: x509.MarshalPKCS1PublicKey(pub.(*rsa.PublicKey)),
		},
	)

	return rsaKeyForTesting{
		privateKey: key,
		privatePEM: keyPEM,
		publicPEM:  pubPEM,
	}
}

func TestGetRequestMetadata_JWTValidation(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2023, time.January, 1, 12, 0, 0, 0, time.UTC))

	conf := Config{
		Email:                 "test@example.com",
		PrivateKey:            bytes.NewBuffer(testKey.privatePEM),
		PrivateKeyID:          "test-key-id",
		Clock:                 clock,
		SkipTransportSecurity: true, // For testing only
	}

	creds, err := NewCredentials(context.Background(), conf)
	if err != nil {
		t.Fatalf("failed to create credentials: %v", err)
	}

	ctx := credentials.NewContextWithRequestInfo(context.Background(), credentials.RequestInfo{Method: "/service.TestService/TestMethod"})
	testURI := "https://api.example.com:8080/service.TestService"

	metadata, err := creds.GetRequestMetadata(ctx, testURI)
	if err != nil {
		t.Fatalf("GetRequestMetadata failed: %v", err)
	}

	// Validate metadata structure
	if len(metadata) != 1 {
		t.Errorf("expected 1 metadata entry, got %d", len(metadata))
	}

	authHeader, exists := metadata["authorization"]
	if !exists {
		t.Fatal("authorization header not found in metadata")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		t.Errorf("authorization header should start with 'Bearer ', got: %s", authHeader)
	}

	// Extract and validate JWT token
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Parse the JWT token without verification first to examine structure
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("failed to parse JWT token: %v", err)
	}

	// Validate header
	if token.Header["alg"] != "RS256" {
		t.Errorf("expected RS256 algorithm, got %s", token.Header["alg"])
	}
	if token.Header["kid"] != "test-key-id" {
		t.Errorf("expected kid 'test-key-id', got %s", token.Header["kid"])
	}

	// Validate claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("failed to cast claims to MapClaims")
	}

	// Check issuer and subject
	if claims["iss"] != "test@example.com" {
		t.Errorf("expected iss 'test@example.com', got %s", claims["iss"])
	}
	if claims["sub"] != "test@example.com" {
		t.Errorf("expected sub 'test@example.com', got %s", claims["sub"])
	}

	// Check audience
	expectedAudience := "https://api.example.com:8080/service.TestService/TestMethod"
	if claims["aud"] != expectedAudience {
		t.Errorf("expected aud '%s', got %s", expectedAudience, claims["aud"])
	}

	// Check timing claims
	iatClaim, ok := claims["iat"].(float64)
	if !ok {
		t.Error("iat claim should be a number")
	} else {
		expectedIat := clock.Now().Unix()
		if int64(iatClaim) != expectedIat {
			t.Errorf("expected iat %d, got %d", expectedIat, int64(iatClaim))
		}
	}

	expClaim, ok := claims["exp"].(float64)
	if !ok {
		t.Error("exp claim should be a number")
	} else {
		expectedExp := clock.Now().Add(time.Hour).Unix()
		if int64(expClaim) != expectedExp {
			t.Errorf("expected exp %d, got %d", expectedExp, int64(expClaim))
		}
	}

	// Verify the token signature using the private key's public key
	publicKey := &testKey.privateKey.PublicKey
	_, err = jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return publicKey, nil
	}, jwt.WithTimeFunc(clock.Now))
	if err != nil {
		t.Errorf("JWT signature validation failed: %v", err)
	}
}

func TestCreateJWT_WithJTI(t *testing.T) {
	t.Parallel()

	now := time.Date(2023, time.January, 1, 12, 0, 0, 0, time.UTC)
	opts := JWTOptions{
		Email:        "test@example.com",
		PrivateKeyID: "key-1",
		IssuedAt:     now,
		ExpiresAt:    now.Add(time.Hour),
		JTI:          "test-jti-value",
	}

	tokenString, err := CreateJWT(opts, testKey.privateKey)
	if err != nil {
		t.Fatalf("CreateJWT failed: %v", err)
	}

	token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("failed to parse JWT: %v", err)
	}

	claims := token.Claims.(jwt.MapClaims)
	if claims["jti"] != "test-jti-value" {
		t.Errorf("expected jti 'test-jti-value', got %v", claims["jti"])
	}
}

func TestCreateJWT_IssuerSubjectOverride(t *testing.T) {
	t.Parallel()

	now := time.Date(2023, time.January, 1, 12, 0, 0, 0, time.UTC)
	opts := JWTOptions{
		Email:        "email@example.com",
		Issuer:       "custom-issuer",
		Subject:      "custom-subject",
		PrivateKeyID: "key-1",
		IssuedAt:     now,
		ExpiresAt:    now.Add(time.Hour),
	}

	tokenString, err := CreateJWT(opts, testKey.privateKey)
	if err != nil {
		t.Fatalf("CreateJWT failed: %v", err)
	}

	token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("failed to parse JWT: %v", err)
	}

	claims := token.Claims.(jwt.MapClaims)
	if claims["iss"] != "custom-issuer" {
		t.Errorf("expected iss 'custom-issuer', got %v", claims["iss"])
	}
	if claims["sub"] != "custom-subject" {
		t.Errorf("expected sub 'custom-subject', got %v", claims["sub"])
	}
}

func TestNewOIDCCredentials_validation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		want string
		c    OIDCConfig
	}{
		{
			name: "missing clock",
			want: "missing required field 'Clock'",
			c: OIDCConfig{
				ClientID:   "client-1",
				TokenURL:   "https://example.com/token",
				PrivateKey: bytes.NewBuffer(testKey.privatePEM),
			},
		},
		{
			name: "missing client ID",
			want: "missing required field 'ClientID'",
			c: OIDCConfig{
				Clock:      clockwork.NewRealClock(),
				TokenURL:   "https://example.com/token",
				PrivateKey: bytes.NewBuffer(testKey.privatePEM),
			},
		},
		{
			name: "missing token URL",
			want: "missing required field 'TokenURL'",
			c: OIDCConfig{
				Clock:      clockwork.NewRealClock(),
				ClientID:   "client-1",
				PrivateKey: bytes.NewBuffer(testKey.privatePEM),
			},
		},
		{
			name: "missing private key",
			want: "missing required field 'PrivateKey'",
			c: OIDCConfig{
				Clock:    clockwork.NewRealClock(),
				ClientID: "client-1",
				TokenURL: "https://example.com/token",
			},
		},
		{
			name: "empty private key",
			want: "empty private key",
			c: OIDCConfig{
				Clock:      clockwork.NewRealClock(),
				ClientID:   "client-1",
				TokenURL:   "https://example.com/token",
				PrivateKey: bytes.NewBuffer([]byte{}),
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewOIDCCredentials(context.Background(), tc.c)
			if got := cmp.Or(err, errors.New("")).Error(); got != tc.want {
				t.Errorf("unexpected validation error: got %q, but expected %q", got, tc.want)
			}
		})
	}
}

func newFakeTokenServer(t *testing.T, key *rsaKeyForTesting, wantClientID string, clock clockwork.Clock, requestCount *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestCount != nil {
			requestCount.Add(1)
		}

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		if got := r.FormValue("grant_type"); got != "client_credentials" {
			http.Error(w, fmt.Sprintf("bad grant_type: %s", got), http.StatusBadRequest)
			return
		}
		if got := r.FormValue("client_id"); got != wantClientID {
			http.Error(w, fmt.Sprintf("bad client_id: %s", got), http.StatusBadRequest)
			return
		}
		if got := r.FormValue("client_assertion_type"); got != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
			http.Error(w, fmt.Sprintf("bad assertion type: %s", got), http.StatusBadRequest)
			return
		}

		assertion := r.FormValue("client_assertion")
		if assertion == "" {
			http.Error(w, "missing client_assertion", http.StatusBadRequest)
			return
		}

		token, err := jwt.Parse(assertion, func(token *jwt.Token) (any, error) {
			return &key.privateKey.PublicKey, nil
		}, jwt.WithTimeFunc(clock.Now))
		if err != nil || !token.Valid {
			http.Error(w, fmt.Sprintf("invalid assertion: %v", err), http.StatusUnauthorized)
			return
		}

		claims := token.Claims.(jwt.MapClaims)
		if claims["iss"] != wantClientID {
			http.Error(w, fmt.Sprintf("bad iss: %v", claims["iss"]), http.StatusBadRequest)
			return
		}
		if claims["jti"] == nil || claims["jti"] == "" {
			http.Error(w, "missing jti", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fake-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
}

func TestOIDCCredentials_TokenExchange(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2023, time.January, 1, 12, 0, 0, 0, time.UTC))
	server := newFakeTokenServer(t, &testKey, "test-client", clock, nil)
	defer server.Close()

	creds, err := NewOIDCCredentials(context.Background(), OIDCConfig{
		Clock:                 clock,
		PrivateKey:            bytes.NewBuffer(testKey.privatePEM),
		PrivateKeyID:          "test-key-id",
		ClientID:              "test-client",
		TokenURL:              server.URL,
		SkipTransportSecurity: true,
	})
	if err != nil {
		t.Fatalf("failed to create OIDC credentials: %v", err)
	}

	ctx := credentials.NewContextWithRequestInfo(context.Background(), credentials.RequestInfo{Method: "/service.Test/Method"})
	metadata, err := creds.GetRequestMetadata(ctx, "https://api.example.com/service.Test")
	if err != nil {
		t.Fatalf("GetRequestMetadata failed: %v", err)
	}

	authHeader, exists := metadata["authorization"]
	if !exists {
		t.Fatal("authorization header not found")
	}
	if authHeader != "Bearer fake-access-token" {
		t.Errorf("expected 'Bearer fake-access-token', got %q", authHeader)
	}
}

func TestOIDCCredentials_TokenCaching(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2023, time.January, 1, 12, 0, 0, 0, time.UTC))
	var requestCount atomic.Int32
	server := newFakeTokenServer(t, &testKey, "test-client", clock, &requestCount)
	defer server.Close()

	creds, err := NewOIDCCredentials(context.Background(), OIDCConfig{
		Clock:                 clock,
		PrivateKey:            bytes.NewBuffer(testKey.privatePEM),
		PrivateKeyID:          "test-key-id",
		ClientID:              "test-client",
		TokenURL:              server.URL,
		SkipTransportSecurity: true,
	})
	if err != nil {
		t.Fatalf("failed to create OIDC credentials: %v", err)
	}

	ctx := credentials.NewContextWithRequestInfo(context.Background(), credentials.RequestInfo{Method: "/service.Test/Method"})

	for i := range 5 {
		if _, err := creds.GetRequestMetadata(ctx, "https://api.example.com/service.Test"); err != nil {
			t.Fatalf("GetRequestMetadata call %d failed: %v", i, err)
		}
	}

	if got := requestCount.Load(); got != 1 {
		t.Errorf("expected 1 token request (cached), got %d", got)
	}
}

func TestOIDCCredentials_TokenRefresh(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2023, time.January, 1, 12, 0, 0, 0, time.UTC))
	var requestCount atomic.Int32
	server := newFakeTokenServer(t, &testKey, "test-client", clock, &requestCount)
	defer server.Close()

	creds, err := NewOIDCCredentials(context.Background(), OIDCConfig{
		Clock:                 clock,
		PrivateKey:            bytes.NewBuffer(testKey.privatePEM),
		PrivateKeyID:          "test-key-id",
		ClientID:              "test-client",
		TokenURL:              server.URL,
		SkipTransportSecurity: true,
	})
	if err != nil {
		t.Fatalf("failed to create OIDC credentials: %v", err)
	}

	ctx := credentials.NewContextWithRequestInfo(context.Background(), credentials.RequestInfo{Method: "/service.Test/Method"})

	// First call: fetches token
	_, err = creds.GetRequestMetadata(ctx, "https://api.example.com/service.Test")
	if err != nil {
		t.Fatalf("first GetRequestMetadata failed: %v", err)
	}

	// Advance clock past staleness window (expires_in=3600s, window=5min, so 56min is stale)
	clock.Advance(56 * time.Minute)

	// Second call: should re-fetch
	_, err = creds.GetRequestMetadata(ctx, "https://api.example.com/service.Test")
	if err != nil {
		t.Fatalf("second GetRequestMetadata failed: %v", err)
	}

	if got := requestCount.Load(); got != 2 {
		t.Errorf("expected 2 token requests (initial + refresh), got %d", got)
	}
}

func TestOIDCCredentials_ErrorHandling(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClockAt(time.Date(2023, time.January, 1, 12, 0, 0, 0, time.UTC))

	t.Run("non-200 response", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}))
		defer server.Close()

		creds, err := NewOIDCCredentials(context.Background(), OIDCConfig{
			Clock:                 clock,
			PrivateKey:            bytes.NewBuffer(testKey.privatePEM),
			ClientID:              "test-client",
			TokenURL:              server.URL,
			SkipTransportSecurity: true,
		})
		if err != nil {
			t.Fatalf("failed to create OIDC credentials: %v", err)
		}

		ctx := credentials.NewContextWithRequestInfo(context.Background(), credentials.RequestInfo{Method: "/test"})
		_, err = creds.GetRequestMetadata(ctx, "https://api.example.com/test")
		if err == nil {
			t.Fatal("expected error for non-200 response")
		}
		if !strings.Contains(err.Error(), "status 401") {
			t.Errorf("expected error about status 401, got: %v", err)
		}
	})

	t.Run("missing access_token", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"token_type": "Bearer"})
		}))
		defer server.Close()

		creds, err := NewOIDCCredentials(context.Background(), OIDCConfig{
			Clock:                 clock,
			PrivateKey:            bytes.NewBuffer(testKey.privatePEM),
			ClientID:              "test-client",
			TokenURL:              server.URL,
			SkipTransportSecurity: true,
		})
		if err != nil {
			t.Fatalf("failed to create OIDC credentials: %v", err)
		}

		ctx := credentials.NewContextWithRequestInfo(context.Background(), credentials.RequestInfo{Method: "/test"})
		_, err = creds.GetRequestMetadata(ctx, "https://api.example.com/test")
		if err == nil {
			t.Fatal("expected error for missing access_token")
		}
		if !strings.Contains(err.Error(), "missing access_token") {
			t.Errorf("expected error about missing access_token, got: %v", err)
		}
	})
}
