// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agentcli

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	"google.golang.org/protobuf/types/known/emptypb"

	"aalyria.com/spacetime/agent/internal/configpb"
	"aalyria.com/spacetime/auth"
)

const (
	testIssuer    = "https://idp.example.com"
	testProjectID = "234567890123456789"
	testKeyID     = "345678901234567890"
	testUserID    = "456789012345678901"
)

// testPrivateKeyPEM returns one PEM-encoded RSA private key for the whole
// test binary. Generating a key is slow, and every test can share one.
var testPrivateKeyPEM = sync.OnceValues(func() (string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})), nil
})

func testPEM(t *testing.T) string {
	t.Helper()
	pemKey, err := testPrivateKeyPEM()
	if err != nil {
		t.Fatalf("generating the test private key: %v", err)
	}
	return pemKey
}

// keyFileJSON encodes a key file with the given fields. A field whose value
// is the empty string is left out of the file.
func keyFileJSON(t *testing.T, fields map[string]string) string {
	t.Helper()
	present := map[string]string{}
	for name, value := range fields {
		if value != "" {
			present[name] = value
		}
	}
	contents, err := json.Marshal(present)
	if err != nil {
		t.Fatalf("encoding the test key file: %v", err)
	}
	return string(contents)
}

func validKeyFileJSON(t *testing.T) string {
	t.Helper()
	return keyFileJSON(t, map[string]string{"keyId": testKeyID, "key": testPEM(t), "userId": testUserID})
}

func writeFile(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func validSpec(keyFile string) *configpb.AuthStrategy_OidcServiceAccount {
	return &configpb.AuthStrategy_OidcServiceAccount{
		Issuer:                testIssuer,
		ProjectId:             testProjectID,
		ServiceAccountKeyFile: keyFile,
	}
}

func insecureConnParams(authStrategy *configpb.AuthStrategy) *configpb.ConnectionParams {
	return &configpb.ConnectionParams{
		TransportSecurity: &configpb.ConnectionParams_TransportSecurity{
			Type: &configpb.ConnectionParams_TransportSecurity_Insecure{Insecure: &emptypb.Empty{}},
		},
		EndpointUri:  "sbi.example.com:443",
		AuthStrategy: authStrategy,
	}
}

// assertNoKeyMaterial fails when the error text holds any 20-byte window of
// the key material. The whole PEM never appears in an error, but a prefix or
// a line of it would be as bad.
func assertNoKeyMaterial(t *testing.T, err error, keyMaterial string) {
	t.Helper()
	if err == nil {
		return
	}
	text := err.Error()
	const window = 20
	if len(keyMaterial) <= window {
		if strings.Contains(text, keyMaterial) {
			t.Errorf("the error holds the key material: %q", text)
		}
		return
	}
	for i := range len(keyMaterial) - window + 1 {
		if strings.Contains(text, keyMaterial[i:i+window]) {
			t.Errorf("the error holds a slice of the key material at offset %d: %q", i, text)
			return
		}
	}
}

func TestGetDialOptsOIDCServiceAccount(t *testing.T) {
	keyFile := writeFile(t, "key.json", validKeyFileJSON(t))
	connParams := insecureConnParams(&configpb.AuthStrategy{
		Type: &configpb.AuthStrategy_OidcServiceAccount_{OidcServiceAccount: validSpec(keyFile)},
	})

	dialOpts, err := getDialOpts(t.Context(), connParams, clockwork.NewFakeClock())
	if err != nil {
		t.Fatalf("getDialOpts error: %v", err)
	}
	if len(dialOpts) == 0 {
		t.Error("getDialOpts returned no dial options")
	}
}

func TestOIDCServiceAccountConfig(t *testing.T) {
	clock := clockwork.NewFakeClock()
	spec := &configpb.AuthStrategy_OidcServiceAccount{
		Issuer:                "https://issuer.example.com",
		ProjectId:             "111111111111111111",
		ServiceAccountKeyFile: "/path/that/is/not/read.json",
	}
	key := &serviceAccountKey{
		KeyID:  "222222222222222222",
		Key:    "-----BEGIN RSA PRIVATE KEY-----\nnot-read-here\n-----END RSA PRIVATE KEY-----\n",
		UserID: "333333333333333333",
	}

	got := oidcServiceAccountConfig(spec, key, clock, true)

	if got.Clock != clock {
		t.Errorf("Clock = %v, want the given clock", got.Clock)
	}
	if got.PrivateKey == nil {
		t.Fatal("PrivateKey is nil")
	}
	pkey, err := io.ReadAll(got.PrivateKey)
	if err != nil {
		t.Fatalf("reading PrivateKey: %v", err)
	}
	if string(pkey) != key.Key {
		t.Errorf("PrivateKey = %q, want %q", pkey, key.Key)
	}
	got.Clock, got.PrivateKey = nil, nil
	want := auth.OIDCServiceAccountConfig{
		Issuer:                "https://issuer.example.com",
		ProjectID:             "111111111111111111",
		KeyID:                 "222222222222222222",
		UserID:                "333333333333333333",
		SkipTransportSecurity: true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("config mismatch (-want +got):\n%s", diff)
	}
}

func TestNewOIDCServiceAccountCredentialsConfigErrors(t *testing.T) {
	keyFile := writeFile(t, "key.json", validKeyFileJSON(t))

	tests := []struct {
		name string
		spec *configpb.AuthStrategy_OidcServiceAccount
		want []string
	}{
		{
			name: "missing issuer",
			spec: &configpb.AuthStrategy_OidcServiceAccount{ProjectId: testProjectID, ServiceAccountKeyFile: keyFile},
			want: []string{"'issuer'"},
		},
		{
			name: "missing project_id",
			spec: &configpb.AuthStrategy_OidcServiceAccount{Issuer: testIssuer, ServiceAccountKeyFile: keyFile},
			want: []string{"'project_id'"},
		},
		{
			name: "missing service_account_key_file",
			spec: &configpb.AuthStrategy_OidcServiceAccount{Issuer: testIssuer, ProjectId: testProjectID},
			want: []string{"'service_account_key_file'"},
		},
		{
			name: "all fields missing",
			spec: &configpb.AuthStrategy_OidcServiceAccount{},
			want: []string{"'issuer'", "'project_id'", "'service_account_key_file'"},
		},
		{
			name: "issuer not https",
			spec: &configpb.AuthStrategy_OidcServiceAccount{Issuer: "http://idp.example.com", ProjectId: testProjectID, ServiceAccountKeyFile: keyFile},
			want: []string{"'issuer'", "https://"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newOIDCServiceAccountCredentials(t.Context(), tc.spec, clockwork.NewFakeClock(), false)
			if err == nil {
				t.Fatal("newOIDCServiceAccountCredentials returned no error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
			}
			assertNoKeyMaterial(t, err, testPEM(t))
		})
	}
}

func TestNewOIDCServiceAccountCredentialsKeyFileErrors(t *testing.T) {
	pemKey := testPEM(t)
	oneMiB := 1 << 20

	tests := []struct {
		name string
		// keyFile returns the path to use as service_account_key_file.
		keyFile func(t *testing.T) string
		// keyMaterial is what no error may contain.
		keyMaterial string
		want        []string
		wantAbsent  []string
	}{
		{
			name: "missing keyId",
			keyFile: func(t *testing.T) string {
				return writeFile(t, "key.json", keyFileJSON(t, map[string]string{"key": pemKey, "userId": testUserID}))
			},
			keyMaterial: pemKey,
			want:        []string{"'keyId'"},
		},
		{
			name: "missing key",
			keyFile: func(t *testing.T) string {
				return writeFile(t, "key.json", keyFileJSON(t, map[string]string{"keyId": testKeyID, "userId": testUserID}))
			},
			keyMaterial: pemKey,
			want:        []string{"'key'"},
		},
		{
			name: "missing userId",
			keyFile: func(t *testing.T) string {
				return writeFile(t, "key.json", keyFileJSON(t, map[string]string{"keyId": testKeyID, "key": pemKey}))
			},
			keyMaterial: pemKey,
			want:        []string{"'userId'", "application key"},
		},
		{
			name: "file does not exist",
			keyFile: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.json")
			},
			keyMaterial: pemKey,
		},
		{
			name: "directory",
			keyFile: func(t *testing.T) string {
				return t.TempDir()
			},
			keyMaterial: pemKey,
			want:        []string{"not a regular file"},
		},
		{
			name: "one byte over 1 MiB",
			keyFile: func(t *testing.T) string {
				return writeFile(t, "key.json", strings.Repeat("x", oneMiB+1))
			},
			keyMaterial: pemKey,
			want:        []string{"larger than"},
		},
		{
			name: "exactly 1 MiB is not too large",
			keyFile: func(t *testing.T) string {
				return writeFile(t, "key.json", strings.Repeat("x", oneMiB))
			},
			keyMaterial: pemKey,
			wantAbsent:  []string{"larger than"},
		},
		{
			name: "bad JSON",
			keyFile: func(t *testing.T) string {
				return writeFile(t, "key.json", `{"keyId": `)
			},
			keyMaterial: pemKey,
		},
		{
			name: "key is not PEM",
			keyFile: func(t *testing.T) string {
				return writeFile(t, "key.json", keyFileJSON(t, map[string]string{"keyId": testKeyID, "key": "this-is-not-a-pem-encoded-private-key-at-all", "userId": testUserID}))
			},
			keyMaterial: "this-is-not-a-pem-encoded-private-key-at-all",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.keyFile(t)
			_, err := newOIDCServiceAccountCredentials(t.Context(), validSpec(path), clockwork.NewFakeClock(), false)
			if err == nil {
				t.Fatal("newOIDCServiceAccountCredentials returned no error")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error %q does not name the path %q", err, path)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not hold %q", err, want)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(err.Error(), absent) {
					t.Errorf("error %q holds %q", err, absent)
				}
			}
			assertNoKeyMaterial(t, err, tc.keyMaterial)
		})
	}
}

func TestNewOIDCServiceAccountCredentialsSymbolicLink(t *testing.T) {
	target := writeFile(t, "key.json", validKeyFileJSON(t))
	link := filepath.Join(t.TempDir(), "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("creating the symbolic link: %v", err)
	}

	creds, err := newOIDCServiceAccountCredentials(t.Context(), validSpec(link), clockwork.NewFakeClock(), false)
	if err != nil {
		t.Fatalf("newOIDCServiceAccountCredentials error: %v", err)
	}
	if creds == nil {
		t.Error("newOIDCServiceAccountCredentials returned nil credentials")
	}
}

func TestReadServiceAccountKey(t *testing.T) {
	path := writeFile(t, "key.json", validKeyFileJSON(t))

	got, err := readServiceAccountKey(path)
	if err != nil {
		t.Fatalf("readServiceAccountKey error: %v", err)
	}
	want := &serviceAccountKey{KeyID: testKeyID, Key: testPEM(t), UserID: testUserID}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("key mismatch (-want +got):\n%s", diff)
	}
}
