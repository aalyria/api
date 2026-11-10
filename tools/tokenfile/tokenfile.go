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

// Package tokenfile keeps the tokens of an interactive OIDC login in a file
// between invocations of a command-line tool.
package tokenfile // import "aalyria.com/spacetime/tools/tokenfile"

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"aalyria.com/spacetime/auth"
)

// Store holds [auth.Tokens] in a JSON file with mode 0600. It implements
// [auth.TokenStore].
type Store struct {
	path string
}

// New returns the store that keeps its tokens in the file at path. The caller
// chooses the path, and with it which identities share a file: tokens are
// sent to every endpoint whose store resolves to the same path.
func New(path string) *Store {
	return &Store{path: path}
}

// Path is the file in which the store keeps its tokens.
func (s *Store) Path() string {
	return s.path
}

// storedTokens is the on-disk form of [auth.Tokens]. It restates the fields
// on purpose: auth/ is exported to the public API repository, so putting JSON
// tags on [auth.Tokens] would make this file format part of that public
// interface and freeze it. Email is deliberately absent, because a stored
// email would go stale against the ID token beside it.
type storedTokens struct {
	IDToken      string    `json:"id_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry"`
}

// Load returns the stored tokens, or (nil, nil) when there is no file.
func (s *Store) Load() (*auth.Tokens, error) {
	contents, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("reading the token file %s: %w", s.path, err)
	}

	stored := storedTokens{}
	if err := json.Unmarshal(contents, &stored); err != nil {
		return nil, fmt.Errorf("parsing the token file %s: %w", s.path, err)
	}

	return &auth.Tokens{
		IDToken:      stored.IDToken,
		RefreshToken: stored.RefreshToken,
		Expiry:       stored.Expiry,
	}, nil
}

// Save replaces the stored tokens. It creates the file's directory with mode
// 0700 if it does not exist.
func (s *Store) Save(tokens *auth.Tokens) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating the token directory %s: %w", dir, err)
	}

	contents, err := json.Marshal(storedTokens{
		IDToken:      tokens.IDToken,
		RefreshToken: tokens.RefreshToken,
		Expiry:       tokens.Expiry,
	})
	if err != nil {
		return fmt.Errorf("encoding the tokens: %w", err)
	}

	// os.CreateTemp makes the file with mode 0600. Write to it and then
	// rename it, so that a concurrent reader never sees a partial file.
	tmp, err := os.CreateTemp(dir, ".tokens-*")
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

// Delete removes the token file. It succeeds when there is no file.
func (s *Store) Delete() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting the token file %s: %w", s.path, err)
	}
	return nil
}
