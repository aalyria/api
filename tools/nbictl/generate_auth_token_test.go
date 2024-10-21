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

package nbictl

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateAuthToken_requiresUserId(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	// Set key_id so that the config file is created.
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "config", "set", "--key_id", "key1",
	}))

	switch want, err := `no user_id set for chosen context`, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "generate-auth-token",
	}); {
	case err == nil:
		t.Fatal("expected missing user_id setting to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}
}

func TestGenerateAuthToken_requiresKeyId(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	// Set user_id, but not key_id
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "config", "set", "--user_id", "user1",
	}))

	switch want, err := `no key_id set for chosen context`, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "generate-auth-token",
	}); {
	case err == nil:
		t.Fatal("expected missing user_id setting to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}
}

func TestGenerateAuthToken_requiresPrivKey(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	// Set user_id, and key_id but no priv_key
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "config", "set", "--user_id", "user1",
	}))
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "config", "set", "--key_id", "key1",
	}))

	switch want, err := `no priv_key set for chosen context`, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "generate-auth-token",
	}); {
	case err == nil:
		t.Fatal("expected missing user_id setting to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}
}

func TestGenerateAuthToken_happyPath(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	keys := generateKeysForTesting(t, tmpDir, "--org", "user.organization")
	certBytes, err := os.ReadFile(keys.cert)
	checkErr(t, err)
	pemCrtBlock, _ := pem.Decode(certBytes)
	cert, err := x509.ParseCertificate(pemCrtBlock.Bytes)
	checkErr(t, err)

	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "config", "set", "--user_id", "user1",
	}))
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "config", "set", "--key_id", "key1",
	}))
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "config", "set", "--priv_key", keys.key,
	}))

	app := newTestApp()
	checkErr(t, app.Run([]string{
		"nbictl", "--config_dir", tmpDir, "generate-auth-token", "--audience", "providedAudience",
	}))

	token, err := jwt.Parse(string(app.stdout.Bytes()), func(token *jwt.Token) (interface{}, error) {
		return cert.PublicKey, nil
	},
		jwt.WithSubject("user1"),
		jwt.WithIssuer("user1"),
		jwt.WithIssuedAt(),
		jwt.WithExpirationRequired())
	checkErr(t, err)
	if token.Header["kid"] != "key1" {
		t.Errorf("header[kid] invalid. Expected: key1, actual: %s", token.Header["kid"])
	}
	exp, err := token.Claims.GetExpirationTime()
	checkErr(t, err)
	iat, err := token.Claims.GetIssuedAt()
	checkErr(t, err)
	if exp.Time.Sub(iat.Time) != time.Hour {
		t.Errorf("exp is not 1h from iat. exp: %s, iat: %s", exp.Time, iat.Time)
	}
	aud, err := token.Claims.GetAudience()
	if aud[0] != "providedAudience" {
		t.Errorf("aud is not correct. Expected: providedAudience, Actual: %s", aud)
	}
}
