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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aalyria.com/spacetime/auth"
)

// serviceAccountKey is the on-disk form of a Zitadel JSON key file. It
// restates the provider's field names here rather than in auth/, matching the
// storedTokens precedent at login.go: auth/ owns the protocol, nbictl owns
// file formats.
type serviceAccountKey struct {
	KeyID  string `json:"keyId"`
	Key    string `json:"key"`
	UserID string `json:"userId"`
}

// maxServiceAccountKeyBytes caps the key file. A Zitadel key file is about two
// kilobytes, so anything near this size names the wrong file.
const maxServiceAccountKeyBytes = 1 << 20

// readServiceAccountKey reads and validates a Zitadel JSON key file. When
// the file's permissions allow group or world read or write access, it writes
// a warning to errWriter but still returns the key. The key material is never
// written to errWriter or included in an error string.
func readServiceAccountKey(errWriter io.Writer, path string) (*serviceAccountKey, error) {
	// The path is inspected before it is opened, rather than with fstat on the
	// open handle, because opening a named pipe blocks until a writer appears
	// and would stop the command with no diagnostic at all.
	//
	// That order leaves a window between the check and the open. It costs
	// nothing here: the operator chooses the path and it is read with the
	// operator's own privileges, so anyone who can swap it already holds
	// everything the key would give them.
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading service account key file %s: %w", path, err)
	}
	// The shape of the path is settled before its mode is reported, because a
	// directory would otherwise be described as granting the same access as a
	// password.
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("reading service account key file %s: not a regular file", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(errWriter, "warning: the service account key file %s has mode %o, which grants the same access as a password to other users or groups; consider restricting it to 0600\n", path, info.Mode().Perm())
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading service account key file %s: %w", path, err)
	}
	defer f.Close()

	// One byte past the cap distinguishes a file at the cap from one over it.
	data, err := io.ReadAll(io.LimitReader(f, maxServiceAccountKeyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading service account key file %s: %w", path, err)
	}
	if len(data) > maxServiceAccountKeyBytes {
		return nil, fmt.Errorf("reading service account key file %s: larger than %d bytes", path, maxServiceAccountKeyBytes)
	}

	var key serviceAccountKey
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, fmt.Errorf("parsing service account key file %s: %w", path, err)
	}

	if key.KeyID == "" {
		return nil, fmt.Errorf("service account key file %s: missing required field 'keyId'", path)
	}
	if key.Key == "" {
		return nil, fmt.Errorf("service account key file %s: missing required field 'key'", path)
	}
	if key.UserID == "" {
		return nil, fmt.Errorf("service account key file %s: missing required field 'userId'", path)
	}

	return &key, nil
}

// serviceAccountTokenFileSuffix names the cache file of the
// oidc_service_account strategy, apart from the oidc_user token file of the
// same profile.
const serviceAccountTokenFileSuffix = ".serviceaccount.json"

// serviceAccountIdentity is everything that decides whether a cached access
// token is interchangeable with a freshly exchanged one. The project id is
// the reason this exists: it sets the token's aud claim and it appears in the
// *name* of the roles claim, so a token minted for one project is unusable
// against another even though both are valid tokens for the same account.
type serviceAccountIdentity struct {
	issuer    string
	projectID string
	userID    string
	keyID     string
}

// fingerprint identifies the four fields together. The NUL separator keeps
// two different identities from sharing one digest, which joining on an empty
// string would allow.
func (id serviceAccountIdentity) fingerprint() string {
	sum := sha256.Sum256([]byte(strings.Join(
		[]string{id.issuer, id.projectID, id.userID, id.keyID}, "\x00")))
	return hex.EncodeToString(sum[:])
}

// storedServiceAccountToken is the on-disk form of
// [auth.ServiceAccountToken]. As with storedTokens in login.go, the JSON tags
// live here and not in auth/, because auth/ is exported to the public API
// repository and this file format must not become part of that interface.
type storedServiceAccountToken struct {
	AccessToken string    `json:"access_token"`
	Expiry      time.Time `json:"expiry"`
	Fingerprint string    `json:"fingerprint"`
}

// serviceAccountTokenStore caches one profile's access token in a JSON file
// with mode 0600. It implements [auth.ServiceAccountTokenStore].
//
// Every fault it meets is reported to errWriter and then treated as an empty
// cache. The cache only saves a round trip, so a file it cannot use must cost
// that round trip and nothing more.
type serviceAccountTokenStore struct {
	path        string
	fingerprint string
	errWriter   io.Writer
}

func newServiceAccountTokenStore(confDir, profileName string, id serviceAccountIdentity, errWriter io.Writer) *serviceAccountTokenStore {
	return &serviceAccountTokenStore{
		path:        filepath.Join(confDir, tokenDirName, profileFileName(profileName, serviceAccountTokenFileSuffix)),
		fingerprint: id.fingerprint(),
		errWriter:   errWriter,
	}
}

func (s *serviceAccountTokenStore) Load() (*auth.ServiceAccountToken, error) {
	contents, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case err != nil:
		fmt.Fprintf(s.errWriter, "warning: the cached token file %s could not be read (%v), so a new token will be obtained\n", s.path, err)
		return nil, nil
	}

	stored := storedServiceAccountToken{}
	if err := json.Unmarshal(contents, &stored); err != nil {
		fmt.Fprintf(s.errWriter, "warning: the cached token file %s could not be parsed (%v), so a new token will be obtained\n", s.path, err)
		return nil, nil
	}

	// A token minted for another issuer, project, user or key is not this
	// profile's token. Serving it would send a token whose roles claim the
	// Permissions service does not read, which arrives as a PermissionDenied
	// that names nothing.
	if stored.Fingerprint != s.fingerprint {
		return nil, nil
	}

	return &auth.ServiceAccountToken{
		AccessToken: stored.AccessToken,
		ExpiresAt:   stored.Expiry,
	}, nil
}

func (s *serviceAccountTokenStore) Save(token *auth.ServiceAccountToken) error {
	if err := s.save(token); err != nil {
		fmt.Fprintf(s.errWriter, "warning: the access token could not be cached (%v), so the next command will obtain its own\n", err)
		return err
	}
	return nil
}

func (s *serviceAccountTokenStore) save(token *auth.ServiceAccountToken) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating the token directory %s: %w", dir, err)
	}

	contents, err := json.Marshal(storedServiceAccountToken{
		AccessToken: token.AccessToken,
		Expiry:      token.ExpiresAt,
		Fingerprint: s.fingerprint,
	})
	if err != nil {
		return fmt.Errorf("encoding the cached token: %w", err)
	}

	// os.CreateTemp makes the file with mode 0600. Write to it and then rename
	// it, so that a concurrent reader never sees a partial file.
	tmp, err := os.CreateTemp(dir, ".serviceaccount-*")
	if err != nil {
		return fmt.Errorf("creating a temporary token file in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(contents); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmp.Name(), s.path, err)
	}
	return nil
}

// Delete removes the cache file and reports whether one was there.
func (s *serviceAccountTokenStore) Delete() (bool, error) {
	switch err := os.Remove(s.path); {
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("deleting the cached token file %s: %w", s.path, err)
	}
	return true, nil
}
