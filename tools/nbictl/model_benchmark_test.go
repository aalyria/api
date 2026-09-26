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
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/prototext"

	nmtspb "outernetcouncil.org/nmts/v1/proto"
)

func writeFragmentFiles(dir string, entities []*nmtspb.Entity, relationships []*nmtspb.Relationship, filesCount int) error {
	for i := range filesCount {
		fragment := &nmtspb.Fragment{}

		eStart := i * len(entities) / filesCount
		eEnd := (i + 1) * len(entities) / filesCount
		fragment.Entity = entities[eStart:eEnd]

		rStart := i * len(relationships) / filesCount
		rEnd := (i + 1) * len(relationships) / filesCount
		fragment.Relationship = relationships[rStart:rEnd]

		data, err := prototext.Marshal(fragment)
		if err != nil {
			return fmt.Errorf("marshalling fragment %d: %w", i, err)
		}

		path := filepath.Join(dir, fmt.Sprintf("fragment-%03d.txtpb", i))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("writing fragment file %s: %w", path, err)
		}
	}
	return nil
}

func generateKeysForBenchmark(b *testing.B, dir string, args ...string) testKeyPath {
	b.Helper()

	if err := newTestApp().Run(append([]string{"nbictl", "--config_dir", dir, "generate-keys", "--dir", dir}, args...)); err != nil {
		b.Fatalf("unable to generate keys: %v", err)
	}

	privKeyPaths, err := filepath.Glob(filepath.Join(dir, "*.key"))
	if err != nil {
		b.Fatal(err)
	} else if len(privKeyPaths) != 1 {
		b.Fatalf("expected to generate 1 private key, got %v", privKeyPaths)
	}
	certPaths, err := filepath.Glob(filepath.Join(dir, "*.crt"))
	if err != nil {
		b.Fatal(err)
	} else if len(certPaths) != 1 {
		b.Fatalf("expected to generate 1 cert, got %v", certPaths)
	}

	return testKeyPath{
		key:  privKeyPaths[0],
		cert: certPaths[0],
	}
}

func setupBenchmarkEnv(b *testing.B, numEntities int, numFiles int, seedRemote bool) (argsPrefix []string, cancel context.CancelFunc) {
	b.Helper()

	tmpDir, err := bazel.NewTmpDir("nbictl-bench")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { os.RemoveAll(tmpDir) })

	entities, relationships := generateSyntheticModel(numEntities)

	modelDir := filepath.Join(tmpDir, "model")
	if err := os.MkdirAll(modelDir, 0o700); err != nil {
		b.Fatal(err)
	}
	if err := writeFragmentFiles(modelDir, entities, relationships, numFiles); err != nil {
		b.Fatal(err)
	}

	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Minute)
	b.Cleanup(cancelFn)

	g, ctx := errgroup.WithContext(ctx)
	b.Cleanup(func() {
		if err := g.Wait(); err != nil {
			b.Logf("errgroup wait: %v", err)
		}
	})

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		b.Fatal(err)
	}

	srv, err := startInMemoryModelServer(ctx, g, lis)
	if err != nil {
		b.Fatal(err)
	}

	if seedRemote {
		srv.Seed(entities, relationships)
	}

	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		b.Fatal(err)
	}

	keys := generateKeysForBenchmark(b, configDir, "--org", "bench org")

	argsPrefix = []string{"nbictl", "--config_dir", configDir, "--context", "DEFAULT"}
	if err := newTestApp().Run(append(argsPrefix, []string{
		"config",
		"set",
		"--transport_security", "insecure",
		"--user_id", "benchuser",
		"--key_id", "benchkey",
		"--priv_key", keys.key,
		"--url", lis.Addr().String(),
	}...)); err != nil {
		b.Fatal(err)
	}

	argsPrefix = append(argsPrefix, "model-v1", "sync", "-r", modelDir)
	return argsPrefix, cancelFn
}

func BenchmarkModelSync_AddAll(b *testing.B) {
	for b.Loop() {

		b.StopTimer()
		// Re-create the server state for each iteration by creating a fresh environment
		argsPrefix, cancel := setupBenchmarkEnv(b, 1000, 10, false)
		defer cancel()
		b.StartTimer()

		app := newTestApp()
		if err := app.Run(argsPrefix); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkModelSync_UpdateAll(b *testing.B) {
	argsPrefix, cancel := setupBenchmarkEnv(b, 1000, 10, true)
	defer cancel()

	for b.Loop() {
		app := newTestApp()
		if err := app.Run(argsPrefix); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkModelSync_MixedWithDelete(b *testing.B) {
	argsPrefix, cancel := setupBenchmarkEnv(b, 1000, 10, true)
	defer cancel()

	args := make([]string, 0, len(argsPrefix)+1)
	// Insert --delete flag before the directory argument.
	args = append(args, argsPrefix[:len(argsPrefix)-1]...)
	args = append(args, "--delete")
	args = append(args, argsPrefix[len(argsPrefix)-1])

	for b.Loop() {
		app := newTestApp()
		if err := app.Run(args); err != nil {
			b.Fatal(err)
		}
	}
}
