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

package nbictl

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aalyria.com/spacetime/auth"
)

// testPEMKey is a minimal RSA private key PEM block, used only to verify
// that readServiceAccountKey round-trips the key bytes. It is not a valid
// cryptographic key.
const testPEMKey = `-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBALRiMLAHudeSA/xKl1oqNTHoHBKKbDM+tE63u4BWFP/B5Rlx5UC
c1s9VDMdNtKj3eFCmMRGTorFCGJJOEW2i5kCAwEAAQJAIH5l60zVmNLLkpIKuD6I
UaKLGgXp5TxiuTxqNVbQHrGcFKEFqpkPJWbWbXHQmHFRfYzFOGqkHPZ2gGPAi4vl
gQIhANb2/2v9xHH3Jq6hLs1c+I8/z6j9M9TfP8aTGQIhAN8e0fPDqr5o2g5LGx1A
eaMHe8pK3iJ0z5e9BRtxQdFxAiEA3mM6T1q+LR3W1RJd1Yn9VadPcYoXJM7w4E8l
A1MCIBuEtTfT0p9KZ5Vck0LDJL1kX0jMb2K0yWYU1VDi7bhhAiEAmstlb5xY7SKc
N5s+eIAoIEKMFTbMqV9N0VPZF3XMC4U=
-----END RSA PRIVATE KEY-----
`

func writeKeyFile(t *testing.T, dir, filename, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("writing key file: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod key file: %v", err)
	}
	return path
}

func TestReadServiceAccountKey_WellFormed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `{"keyId":"key-1","key":"` + strings.ReplaceAll(testPEMKey, "\n", `\n`) + `","userId":"user-1"}`
	path := writeKeyFile(t, dir, "key.json", content, 0o600)

	var errBuf bytes.Buffer
	got, err := readServiceAccountKey(&errBuf, path)
	if err != nil {
		t.Fatalf("readServiceAccountKey error: %v", err)
	}
	if got.KeyID != "key-1" {
		t.Errorf("got KeyID %q, want %q", got.KeyID, "key-1")
	}
	if got.UserID != "user-1" {
		t.Errorf("got UserID %q, want %q", got.UserID, "user-1")
	}
	if !strings.Contains(got.Key, "RSA PRIVATE KEY") {
		t.Errorf("Key does not look like a PEM block: %q", got.Key)
	}
	if errBuf.Len() != 0 {
		t.Errorf("unexpected warning on stderr: %q", errBuf.String())
	}
}

func TestReadServiceAccountKey_MissingUserID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `{"keyId":"key-1","key":"-----BEGIN RSA PRIVATE KEY-----\ndata\n-----END RSA PRIVATE KEY-----\n"}`
	path := writeKeyFile(t, dir, "key.json", content, 0o600)

	var errBuf bytes.Buffer
	_, err := readServiceAccountKey(&errBuf, path)
	if err == nil {
		t.Fatal("expected an error for missing userId, got nil")
	}
	if !strings.Contains(err.Error(), "userId") {
		t.Errorf("error %q does not name the missing field 'userId'", err.Error())
	}
}

func TestReadServiceAccountKey_InvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeKeyFile(t, dir, "key.json", "not json", 0o600)

	var errBuf bytes.Buffer
	_, err := readServiceAccountKey(&errBuf, path)
	if err == nil {
		t.Fatal("expected an error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the file path %q", err.Error(), path)
	}
}

// TestReadServiceAccountKey_PermissiveMode is deletion test 11: the mode check
// in readServiceAccountKey warns but does not refuse. Removing the check makes
// this test fail because no warning is emitted.
func TestReadServiceAccountKey_PermissiveMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `{"keyId":"key-1","key":"` + strings.ReplaceAll(testPEMKey, "\n", `\n`) + `","userId":"user-1"}`

	// 0o644: group and other can read — should warn.
	permissivePath := writeKeyFile(t, dir, "key644.json", content, 0o644)
	var warnBuf bytes.Buffer
	got, err := readServiceAccountKey(&warnBuf, permissivePath)
	if err != nil {
		t.Fatalf("readServiceAccountKey must succeed even for a permissive mode: %v", err)
	}
	if got == nil {
		t.Fatal("got nil result despite no error")
	}
	warn := warnBuf.String()
	if !strings.Contains(warn, permissivePath) {
		t.Errorf("warning %q does not name the path %q", warn, permissivePath)
	}

	// 0o600: only owner can read — must not warn.
	restrictedPath := writeKeyFile(t, dir, "key600.json", content, 0o600)
	var quietBuf bytes.Buffer
	_, err = readServiceAccountKey(&quietBuf, restrictedPath)
	if err != nil {
		t.Fatalf("readServiceAccountKey error: %v", err)
	}
	if quietBuf.Len() != 0 {
		t.Errorf("unexpected warning for a 0600 key file: %q", quietBuf.String())
	}
}

// TestReadServiceAccountKey_NotARegularFile pins the shape of the path. A
// directory carries a mode of its own, so the permission check warns that a
// directory grants the same access as a password and only then fails. A named
// pipe is worse: the read blocks forever with no diagnostic at all.
func TestReadServiceAccountKey_NotARegularFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "keydir")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("creating the directory: %v", err)
	}

	var errBuf bytes.Buffer
	if _, err := readServiceAccountKey(&errBuf, path); err == nil {
		t.Fatal("readServiceAccountKey accepted a directory")
	}
	if warn := errBuf.String(); warn != "" {
		t.Errorf("readServiceAccountKey warned about the mode of something that is not a file: %q", warn)
	}
}

// TestReadServiceAccountKey_TooLarge pins the size. A key file is about two
// kilobytes, so a path that names something far larger is a mistake, and
// reading it whole into memory before parsing it serves nobody.
func TestReadServiceAccountKey_TooLarge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeKeyFile(t, dir, "big.json", strings.Repeat("x", maxServiceAccountKeyBytes+1), 0o600)

	var errBuf bytes.Buffer
	_, err := readServiceAccountKey(&errBuf, path)
	if err == nil {
		t.Fatal("readServiceAccountKey accepted a file larger than the cap")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the file path %q", err.Error(), path)
	}
}

// testSAIdentity is the identity that a cached token belongs to.
func testSAIdentity() serviceAccountIdentity {
	return serviceAccountIdentity{
		issuer:    "https://zitadel.example.com",
		projectID: "test-project",
		userID:    "test-user",
		keyID:     "test-key",
	}
}

func TestServiceAccountTokenStore_RoundTrip(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	errs := &bytes.Buffer{}
	store := newServiceAccountTokenStore(confDir, "robot", testSAIdentity(), errs)

	want := &auth.ServiceAccountToken{
		AccessToken: "a.b.c",
		ExpiresAt:   time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC),
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned an unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned no token after Save")
	}
	if got.AccessToken != want.AccessToken {
		t.Errorf("AccessToken: got %q, want %q", got.AccessToken, want.AccessToken)
	}
	if !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("ExpiresAt: got %v, want %v", got.ExpiresAt, want.ExpiresAt)
	}
	if errs.Len() != 0 {
		t.Errorf("a clean round trip warned: %s", errs)
	}
}

func TestServiceAccountTokenStore_FileModeIs0600(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	store := newServiceAccountTokenStore(confDir, "robot", testSAIdentity(), io.Discard)
	if err := store.Save(&auth.ServiceAccountToken{AccessToken: "a.b.c"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatalf("the cache file is missing: %v", err)
	}
	if want := filepath.Join(confDir, "tokens"); filepath.Dir(store.path) != want {
		t.Errorf("the cache directory is %q, want %q", filepath.Dir(store.path), want)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode: got %o, want %o", got, 0o600)
	}
}

func TestServiceAccountTokenStore_MissingFileLoadsNothing(t *testing.T) {
	t.Parallel()

	store := newServiceAccountTokenStore(t.TempDir(), "robot", testSAIdentity(), io.Discard)
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned an unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("Load returned %+v although no file exists", got)
	}
}

// TestServiceAccountTokenStore_RefusesAnotherIdentity is the deletion test for
// the fingerprint. A token is only valid for the issuer, project, user and key
// that minted it: the project id decides both the aud claim and the name of
// the roles claim. Serving a token across a `config set --project_id` change
// would produce a PermissionDenied with no clue why.
func TestServiceAccountTokenStore_RefusesAnotherIdentity(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	saved := newServiceAccountTokenStore(confDir, "robot", testSAIdentity(), io.Discard)
	if err := saved.Save(&auth.ServiceAccountToken{
		AccessToken: "a.b.c",
		ExpiresAt:   time.Now().Add(12 * time.Hour),
	}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	for _, tc := range []struct {
		name   string
		mutate func(*serviceAccountIdentity)
	}{
		{name: "another project", mutate: func(i *serviceAccountIdentity) { i.projectID = "other-project" }},
		{name: "another issuer", mutate: func(i *serviceAccountIdentity) { i.issuer = "https://other.example.com" }},
		{name: "another user", mutate: func(i *serviceAccountIdentity) { i.userID = "other-user" }},
		{name: "another key", mutate: func(i *serviceAccountIdentity) { i.keyID = "other-key" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			id := testSAIdentity()
			tc.mutate(&id)
			got, err := newServiceAccountTokenStore(confDir, "robot", id, io.Discard).Load()
			if err != nil {
				t.Fatalf("Load returned an unexpected error: %v", err)
			}
			if got != nil {
				t.Errorf("Load returned a token minted for another identity: %+v", got)
			}
		})
	}
}

// TestServiceAccountTokenStore_CorruptFileLoadsNothing pins that a bad cache
// file costs a round trip rather than wedging the command.
func TestServiceAccountTokenStore_CorruptFileLoadsNothing(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	errs := &bytes.Buffer{}
	store := newServiceAccountTokenStore(confDir, "robot", testSAIdentity(), errs)

	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		t.Fatalf("creating the cache directory: %v", err)
	}
	if err := os.WriteFile(store.path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("writing the corrupt cache file: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned an error for a corrupt file instead of a miss: %v", err)
	}
	if got != nil {
		t.Errorf("Load returned %+v from a corrupt file", got)
	}
	if !strings.Contains(errs.String(), store.path) {
		t.Errorf("the corrupt cache file was not reported; stderr held %q", errs)
	}
}

// TestServiceAccountTokenStore_DoesNotShareTheUserTokenFile keeps the two
// caches of one profile apart. They hold different token kinds for different
// identities, so one must never be parsed as the other.
func TestServiceAccountTokenStore_DoesNotShareTheUserTokenFile(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	sa := newServiceAccountTokenStore(confDir, "robot", testSAIdentity(), io.Discard)
	user := newFileTokenStore(confDir, "robot")
	if sa.path == user.path {
		t.Errorf("both stores use the path %q", sa.path)
	}
}

func TestServiceAccountTokenStore_Delete(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	store := newServiceAccountTokenStore(confDir, "robot", testSAIdentity(), io.Discard)

	removed, err := store.Delete()
	if err != nil {
		t.Fatalf("Delete returned an unexpected error for a missing file: %v", err)
	}
	if removed {
		t.Error("Delete reported a removal although no file exists")
	}

	if err := store.Save(&auth.ServiceAccountToken{AccessToken: "a.b.c"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}
	removed, err = store.Delete()
	if err != nil {
		t.Fatalf("Delete returned an unexpected error: %v", err)
	}
	if !removed {
		t.Error("Delete reported no removal although a file existed")
	}
	if _, err := os.Stat(store.path); !os.IsNotExist(err) {
		t.Errorf("the cache file survived Delete: %v", err)
	}
}
