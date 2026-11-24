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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	gcmp "github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
)

type provisioningSyncTestEnv struct {
	dataDir    string
	argsPrefix []string
	srv        *fakeProvisioningServer
}

func setupProvisioningSyncTestEnv(t *testing.T, srv *fakeProvisioningServer) *provisioningSyncTestEnv {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	g, ctx := errgroup.WithContext(ctx)
	t.Cleanup(func() {
		cancel()
		_ = g.Wait()
	})

	tmpDir, err := bazel.NewTmpDir("nbictl-provisioning-sync")
	checkErr(t, err)
	dataDir := filepath.Join(tmpDir, "data")
	checkErr(t, os.MkdirAll(dataDir, 0o700))

	lis, err := net.Listen("tcp", "localhost:0")
	checkErr(t, err)
	startFakeProvisioningServer(ctx, g, lis, srv)

	keys := generateKeysForTesting(t, tmpDir, "--org", "test org")
	argsPrefix := []string{"nbictl", "--config_dir", tmpDir, "--context", "DEFAULT"}
	checkErr(t, newTestApp().Run(append(slices.Clone(argsPrefix),
		"config", "set",
		"--transport_security", "insecure",
		"--user_id", "testuser",
		"--key_id", "testkey",
		"--priv_key", keys.key,
		"--url", lis.Addr().String(),
		// Send every API, including provisioning, to the one fake server.
		"--endpoint_config", "single_domain",
	)))

	return &provisioningSyncTestEnv{dataDir: dataDir, argsPrefix: argsPrefix, srv: srv}
}

func (env *provisioningSyncTestEnv) writeFile(t *testing.T, name, content string) {
	t.Helper()
	checkErr(t, os.WriteFile(filepath.Join(env.dataDir, name), []byte(content), 0o644))
}

func (env *provisioningSyncTestEnv) run(t *testing.T, args ...string) error {
	t.Helper()
	return newTestApp().Run(append(slices.Clone(env.argsPrefix), args...))
}

func (env *provisioningSyncTestEnv) sync(t *testing.T, extraFlags ...string) error {
	t.Helper()
	args := append([]string{"provisioning-v1alpha", "sync", "-r", "--progress", "off"}, extraFlags...)
	return env.run(t, append(args, env.dataDir)...)
}

// regionsAndLimits returns n geographic regions and one emissions limit in
// each, as a ProvisioningResources text proto.
func regionsAndLimits(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "geographic_regions { name: \"geographicRegions/r%d\" }\n", i)
		fmt.Fprintf(&b, "emissions_limits { name: \"emissionsLimits/l%d\" upper_frequency_hz: 1 geographic_regions: \"geographicRegions/r%d\" }\n", i, i)
	}
	return b.String()
}

func TestProvisioningSync_CreatesGeographicRegionsBeforeEmissionsLimits(t *testing.T) {
	t.Parallel()
	srv := newFakeProvisioningServer()
	srv.writeDelay = 20 * time.Millisecond
	env := setupProvisioningSyncTestEnv(t, srv)
	env.writeFile(t, "regions.txtpb", regionsAndLimits(20))

	checkErr(t, env.sync(t))

	if len(srv.regions) != 20 || len(srv.limits) != 20 {
		t.Errorf("got %d regions and %d emissions limits, want 20 of each", len(srv.regions), len(srv.limits))
	}
}

func TestProvisioningDeleteAll_DeletesInDependencyOrder(t *testing.T) {
	t.Parallel()
	srv := newFakeProvisioningServer()
	env := setupProvisioningSyncTestEnv(t, srv)
	env.writeFile(t, "all.txtpb", regionsAndLimits(1))
	checkErr(t, env.sync(t))

	checkErr(t, env.run(t, "provisioning-v1alpha", "delete-all", "--execute", "--progress", "off"))

	want := []string{"emissionsLimits/l0", "geographicRegions/r0"}
	if diff := gcmp.Diff(want, srv.deleteLog); diff != "" {
		t.Errorf("delete order mismatch (-want +got):\n%s", diff)
	}
}
