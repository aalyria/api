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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
)

const validToken = `eyJhbGciOiJSUzI1NiIsImtpZCI6IjcyMTk0YjI2MzU0YzIzYzBiYTU5YTZkNzUxZGZmYWEyNTg2NTkwNGUiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOiJodHRwczovL3d3dy5nb29nbGVhcGlzLmNvbS9vYXV0aDIvdjQvdG9rZW4iLCJleHAiOjE2ODE3OTIzMTksImlhdCI6MTY4MTc4ODcxOSwiaXNzIjoiY2RwaS1hZ2VudEBhNWEtc3BhY2V0aW1lLWdrZS1iYWNrLWRldi5pYW0uZ3NlcnZpY2VhY2NvdW50LmNvbSIsInN1YiI6ImNkcGktYWdlbnRAYTVhLXNwYWNldGltZS1na2UtYmFjay1kZXYuaWFtLmdzZXJ2aWNlYWNjb3VudC5jb20iLCJ0YXJnZXRfYXVkaWVuY2UiOiI2MDI5MjQwMzEzOS1tZTY4dGpnYWpsNWRjZGJwbmxtMmVrODMwbHZzbnNscS5hcHBzLmdvb2dsZXVzZXJjb250ZW50LmNvbSJ9.QyOi7vkFCwdmjT4ChT3_yVY4ZObUJkZkYC0q7alF_thiotdJKRiSo1ZHp_XnS0nM4WSWcQYLGHUDdAMPS0R22brFGzCl8ndgNjqI38yp_LDL8QVTqnLBGUj-m3xB5wH17Q_Dt8riBB4IE-mSS8FB-R6sqSwn-seMfMDydScC0FrtOF3-2BCYpIAlf1AQKN083QdtKgNEVDi72npPr2MmsWV3tct6ydXHWNbxG423kfSD6vCZSUTvWXAuVjuOwnbc2LHZS04U-jiLpvHxu06OwHOQ5LoGVPyd69o8Ny_Bapd2m0YCX2xJr8_HH2nw1jH7EplFf-owbBYz9ZtQoQ2YTA`

var testKey = generateRSAPrivateKey()

type badReader struct{ err error }

func (b badReader) Read(_ []byte) (int, error) { return 0, b.err }

func TestNewCredentials_validation(t *testing.T) {
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
			ctx := context.Background()
			_, err := NewCredentials(ctx, tc.c)
			if err.Error() != tc.want {
				t.Errorf("unexpected validation error: got %v, but expected %s", err, tc.want)
			}
		})
	}
}

func TestNewCredentials(t *testing.T) {
	numCalls := &atomic.Int64{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		numCalls.Add(1)
		json.NewEncoder(w).Encode(map[string]interface{}{"id_token": validToken})
	}))
	defer ts.Close()

	ctx := context.Background()
	conf := Config{
		Email:        "some@example.com",
		PrivateKey:   bytes.NewBuffer(testKey.privatePEM),
		PrivateKeyID: "1",
		Clock:        clockwork.NewFakeClockAt(time.Date(2011, time.February, 16, 0, 0, 0, 0, time.UTC)),

		oidcURL: "http://" + ts.Listener.Addr().String() + "/",
	}

	creds, err := NewCredentials(ctx, conf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !creds.RequireTransportSecurity() {
		t.Errorf("credentials should enforce transport security")
	}
	if nc := numCalls.Load(); nc != 1 {
		t.Errorf("expected test server to be called once, but got %d calls", nc)
	}

	// TODO: setup a secure local gRPC server and add a test that
	// checks that the credentials are adding the metadata appropriately. This
	// is more difficult than it sounds because the gRPC RequestInfo struct is
	// required and only gets added by the grpc/internal/credentials package,
	// which can't be used outside the grpc packages.
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
