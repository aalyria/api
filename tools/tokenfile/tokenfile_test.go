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

package tokenfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"aalyria.com/spacetime/auth"
)

// newTestStore returns a store whose file is in a fresh directory that does
// not exist yet, so that Save must create it.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	return New(filepath.Join(t.TempDir(), "tokens", "dev.json"))
}

func TestStore_SaveAndLoad(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	want := &auth.Tokens{
		IDToken:      "id-token-1",
		RefreshToken: "refresh-token-1",
		Expiry:       time.Date(2026, time.August, 11, 13, 0, 0, 0, time.UTC),
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned an unexpected error: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Load mismatch (-want +got):\n%s", diff)
	}
}

// TestStore_DoesNotPersistTheEmail keeps the email out of the file: a stored
// email would go stale against the ID token beside it.
func TestStore_DoesNotPersistTheEmail(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := store.Save(&auth.Tokens{IDToken: "id-token-1", Email: "user@example.com"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned an unexpected error: %v", err)
	}
	if got.Email != "" {
		t.Errorf("Email: got %q, want it empty", got.Email)
	}
}

func TestStore_FileModeIs0600(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := store.Save(&auth.Tokens{IDToken: "id-token-1"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("the token file is missing: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode: got %o, want %o", got, 0o600)
	}
	dirInfo, err := os.Stat(filepath.Dir(store.Path()))
	if err != nil {
		t.Fatalf("the token directory is missing: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("directory mode: got %o, want %o", got, 0o700)
	}
}

func TestStore_LeavesNoTemporaryFile(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := store.Save(&auth.Tokens{IDToken: "id-token-1"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(store.Path()))
	if err != nil {
		t.Fatalf("reading the token directory: %v", err)
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if diff := cmp.Diff([]string{filepath.Base(store.Path())}, names); diff != "" {
		t.Errorf("token directory entries mismatch (-want +got):\n%s", diff)
	}
}

func TestStore_LoadWithNoFile(t *testing.T) {
	t.Parallel()

	got, err := newTestStore(t).Load()
	if err != nil {
		t.Fatalf("Load returned an unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("Load: got %v, want nil when the user is not logged in", got)
	}
}

func TestStore_LoadWithACorruptFile(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o700); err != nil {
		t.Fatalf("creating the token directory: %v", err)
	}
	if err := os.WriteFile(store.Path(), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("writing the token file: %v", err)
	}

	if got, err := store.Load(); err == nil {
		t.Errorf("Load = %v, nil; want an error for a file that isn't JSON", got)
	}
}

func TestStore_Delete(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := store.Save(&auth.Tokens{IDToken: "id-token-1"}); err != nil {
		t.Fatalf("Save returned an unexpected error: %v", err)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete returned an unexpected error: %v", err)
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Error("the token file still exists after Delete")
	}

	// A second delete must succeed.
	if err := store.Delete(); err != nil {
		t.Errorf("the second Delete returned an unexpected error: %v", err)
	}
}
