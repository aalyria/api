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
	"encoding/pem"
	"errors"
	"strings"
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
