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
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"golang.org/x/sync/errgroup"

	nmtspb "outernetcouncil.org/nmts/v1/proto"
)

type syncTestEnv struct {
	tmpDir     string
	dataDir    string
	argsPrefix []string
	srv        *InMemoryModelServer
	cancel     context.CancelFunc
	g          *errgroup.Group
}

func setupSyncTestEnv(t *testing.T) *syncTestEnv {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	g, ctx := errgroup.WithContext(ctx)

	tmpDir, err := bazel.NewTmpDir("nbictl-sync")
	if err != nil {
		cancel()
		t.Fatal(err)
	}

	dataDir := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		cancel()
		t.Fatal(err)
	}

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		cancel()
		t.Fatal(err)
	}

	srv, err := startInMemoryModelServer(ctx, g, lis)
	if err != nil {
		cancel()
		t.Fatal(err)
	}

	keys := generateKeysForTesting(t, tmpDir, "--org", "test org")

	argsPrefix := []string{"nbictl", "--config_dir", tmpDir, "--context", "DEFAULT"}
	if err := newTestApp().Run(append(argsPrefix, []string{
		"config",
		"set",
		"--transport_security", "insecure",
		"--user_id", "testuser",
		"--key_id", "testkey",
		"--priv_key", keys.key,
		"--url", lis.Addr().String(),
	}...)); err != nil {
		cancel()
		t.Fatal(err)
	}

	env := &syncTestEnv{
		tmpDir:     tmpDir,
		dataDir:    dataDir,
		argsPrefix: argsPrefix,
		srv:        srv,
		cancel:     cancel,
		g:          g,
	}

	t.Cleanup(func() {
		cancel()
		_ = g.Wait()
		os.RemoveAll(tmpDir)
	})

	return env
}

func (env *syncTestEnv) writeEntityFile(t *testing.T, filename, content string) {
	t.Helper()
	path := filepath.Join(env.dataDir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing entity file %s: %v", path, err)
	}
}

func (env *syncTestEnv) syncArgs(extraFlags ...string) []string {
	args := make([]string, len(env.argsPrefix))
	copy(args, env.argsPrefix)
	args = append(args, "model-v1", "sync", "-r")
	args = append(args, extraFlags...)
	args = append(args, env.dataDir)
	return args
}

func TestModelSync_AddsNewEntities(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)

	env.writeEntityFile(t, "platforms.txtpb", `
entity { id: "platform-1" ek_platform {} }
entity { id: "platform-2" ek_platform {} }
`)

	if err := newTestApp().Run(env.syncArgs()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if got := env.srv.EntityCount(); got != 2 {
		t.Errorf("EntityCount = %d, want 2", got)
	}
}

func TestModelSync_UpdatesExistingEntities(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)

	env.srv.Seed([]*nmtspb.Entity{
		{Id: "platform-1"},
	}, nil)

	env.writeEntityFile(t, "platforms.txtpb", `
entity { id: "platform-1" ek_platform {} }
`)

	if err := newTestApp().Run(env.syncArgs()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if got := env.srv.EntityCount(); got != 1 {
		t.Errorf("EntityCount = %d, want 1", got)
	}
}

func TestModelSync_AddsRelationships(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)

	env.writeEntityFile(t, "model.txtpb", `
entity { id: "platform-1" ek_platform {} }
entity { id: "platform-2" ek_platform {} }
relationship { a: "platform-1" z: "platform-2" kind: RK_CONTAINS }
`)

	if err := newTestApp().Run(env.syncArgs()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if got := env.srv.EntityCount(); got != 2 {
		t.Errorf("EntityCount = %d, want 2", got)
	}
	if got := env.srv.RelationshipCount(); got != 1 {
		t.Errorf("RelationshipCount = %d, want 1", got)
	}
}

func TestModelSync_DeleteMode_RemovesRemoteOnly(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)

	env.srv.Seed(
		[]*nmtspb.Entity{
			{Id: "platform-1"},
			{Id: "platform-2"},
		},
		[]*nmtspb.Relationship{
			{A: "platform-1", Z: "platform-2", Kind: nmtspb.RK_RK_CONTAINS},
		},
	)

	env.writeEntityFile(t, "platforms.txtpb", `
entity { id: "platform-1" ek_platform {} }
`)

	if err := newTestApp().Run(env.syncArgs("--delete")); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if got := env.srv.EntityCount(); got != 1 {
		t.Errorf("EntityCount = %d, want 1", got)
	}
	if got := env.srv.RelationshipCount(); got != 0 {
		t.Errorf("RelationshipCount = %d, want 0", got)
	}
}

func TestModelSync_DryRun_DoesNotMutate(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)

	env.writeEntityFile(t, "platforms.txtpb", `
entity { id: "platform-1" ek_platform {} }
`)

	if err := newTestApp().Run(env.syncArgs("--dry-run")); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if got := env.srv.EntityCount(); got != 0 {
		t.Errorf("EntityCount = %d, want 0", got)
	}
}

func TestModelSync_NoEntities_ReturnsError(t *testing.T) {
	env := setupSyncTestEnv(t)

	if err := newTestApp().Run(env.syncArgs()); err == nil {
		t.Fatal("expected error when syncing empty directory, got nil")
	}
}

func TestModelSync_MultipleFiles(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)

	env.writeEntityFile(t, "entities.txtpb", `
entity { id: "platform-1" ek_platform {} }
entity { id: "platform-2" ek_platform {} }
`)

	env.writeEntityFile(t, "relationships.txtpb", `
relationship { a: "platform-1" z: "platform-2" kind: RK_CONTAINS }
`)

	if err := newTestApp().Run(env.syncArgs()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if got := env.srv.EntityCount(); got != 2 {
		t.Errorf("EntityCount = %d, want 2", got)
	}
	if got := env.srv.RelationshipCount(); got != 1 {
		t.Errorf("RelationshipCount = %d, want 1", got)
	}
}
