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
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/google/go-cmp/cmp"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	commonpb "aalyria.com/spacetime/api/common"
	nbipb "aalyria.com/spacetime/api/nbi/v1alpha"
	respb "aalyria.com/spacetime/api/nbi/v1alpha/resources"
)

type testApp struct {
	*cli.App

	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func newTestApp() testApp {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	app := App()
	app.Writer = stdout
	app.ErrWriter = stderr
	return testApp{stdout: stdout, stderr: stderr, App: app}
}

func TestList_rejectsUnknownEntities(t *testing.T) {
	t.Parallel()

	switch want, err := `unknown entity type "UFO"`, newTestApp().Run([]string{
		"nbictl", "list", "--type", "UFO",
	}); {
	case err == nil:
		t.Fatal("expected --type UFO to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %s, but got %s", want, err.Error())
	}
}

func TestGet_requiresType(t *testing.T) {
	t.Parallel()

	switch want, err := `Required flag "type" not set`, newTestApp().Run([]string{
		"nbictl", "get", "--id", "abc",
	}); {
	case err == nil:
		t.Fatal("expected missing --type to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}
}

func TestGet_rejectsUnknownEntities(t *testing.T) {
	t.Parallel()

	switch want, err := `unknown entity type "UFO"`, newTestApp().Run([]string{
		"nbictl", "get", "--type", "UFO", "--id", "boba-cafe",
	}); {
	case err == nil:
		t.Fatal("expected --type UFO to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}
}

func TestGet_requiresID(t *testing.T) {
	t.Parallel()

	switch want, err := `Required flag "id" not set`, newTestApp().Run([]string{
		"nbictl", "get", "--type", "NETWORK_NODE",
	}); {
	case err == nil:
		t.Fatal("expected missing --id to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}
}

func TestDelete_rejectsUnknownEntities(t *testing.T) {
	t.Parallel()

	switch want, err := `unknown entity type "UFO"`, newTestApp().Run([]string{
		"nbictl", "delete", "--type", "UFO", "--id", "boba-cafe",
	}); {
	case err == nil:
		t.Fatal("expected --type UFO to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}
}

func TestDelete_requiresID(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	g, ctx := errgroup.WithContext(ctx)
	defer func() { checkErr(t, g.Wait()) }()
	defer cancel()
	srv := startInsecureServer(ctx, t, g)

	keys := generateKeysForTesting(t, tmpDir, "--org", "example org")
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir,
		"set-config",
		"--transport_security", "insecure",
		"--user_id", "usr1",
		"--key_id", "key1",
		"--priv_key", keys.key,
		"--url", srv.listener.Addr().String(),
	}))

	args := []string{"nbictl", "--config_dir", tmpDir, "--context", "DEFAULT", "delete", "--type", "NETWORK_NODE"}
	switch want, err := `either the "type" and "id" flags must be set, or the "files" flag must be set.`, newTestApp().Run(args); {
	case err == nil:
		t.Fatal("expected missing --id to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}
}

func TestDelete_requiresLastCommitTimestampOrIgnoreConsistencyCheck(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	g, ctx := errgroup.WithContext(ctx)
	defer func() { checkErr(t, g.Wait()) }()
	defer cancel()
	srv := startInsecureServer(ctx, t, g)

	keys := generateKeysForTesting(t, tmpDir, "--org", "example org")
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir,
		"set-config",
		"--transport_security", "insecure",
		"--user_id", "usr1",
		"--key_id", "key1",
		"--priv_key", keys.key,
		"--url", srv.listener.Addr().String(),
	}))

	args := []string{"nbictl", "--config_dir", tmpDir, "--context", "DEFAULT", "delete", "--type", "NETWORK_NODE", "--id", "abc"}
	switch want, err := `when deleting a single entity, either "last_commit_timestamp" or "ignore_consistency_check" flags should be set.`, newTestApp().Run(args); {
	case err == nil:
		t.Fatal("expected missing both --last_commit_timestamp and --ignore_consistency_check to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}

	args = []string{"nbictl", "--config_dir", tmpDir, "--context", "DEFAULT", "delete", "--type", "NETWORK_NODE", "--id", "abc", "--last_commit_timestamp", "123", "--ignore_consistency_check"}
	switch want, err := `when deleting a single entity, either "last_commit_timestamp" or "ignore_consistency_check" flags should be set.`, newTestApp().Run(args); {
	case err == nil:
		t.Fatal("expected providing both --last_commit_timestamp and --ignore_consistency_check to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}
}

func TestCreate_requiresFiles(t *testing.T) {
	t.Parallel()

	switch want, err := `Required flag "files" not set`, newTestApp().Run([]string{
		"nbictl", "create",
	}); {
	case err == nil:
		t.Fatal("expected missing --files to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %v, but got %s", want, err.Error())
	}
}

func TestUpdate_requiresFiles(t *testing.T) {
	t.Parallel()

	switch want, err := `Required flag "files" not set`, newTestApp().Run([]string{
		"nbictl", "update",
	}); {
	case err == nil:
		t.Fatal("expected missing --files to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %v, but got %s", want, err.Error())
	}
}

func TestGrpcurlDescribe_rejectsTooManyArgs(t *testing.T) {
	t.Parallel()

	switch want, err := `expected 0 or 1 arguments, got 2`, newTestApp().Run([]string{
		"nbictl", "grpcurl", "describe", "arg1", "arg2",
	}); {
	case err == nil:
		t.Fatal("expected too many arguments to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %v, but got %s", want, err.Error())
	}
}

func TestGrpcurlList_rejectsTooManyArgs(t *testing.T) {
	t.Parallel()

	switch want, err := `expected 0 or 1 arguments, got 2`, newTestApp().Run([]string{
		"nbictl", "grpcurl", "list", "arg1", "arg2",
	}); {
	case err == nil:
		t.Fatal("expected too many arguments to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %v, but got %s", want, err.Error())
	}
}

func TestGrpcurlCall_rejectsInvalidFormat(t *testing.T) {
	t.Parallel()

	switch want, err := `unknown format "idk"`, newTestApp().Run([]string{
		"nbictl", "grpcurl", "call", "--format", "idk",
	}); {
	case err == nil:
		t.Fatal("expected invalid format to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %v, but got %s", want, err.Error())
	}
}

func startInsecureServer(ctx context.Context, t *testing.T, g *errgroup.Group) *FakeNetOpsServer {
	lis, err := net.Listen("tcp", ":0")
	checkErr(t, err)
	srv, err := startFakeNbiServer(ctx, g, lis)
	checkErr(t, err)

	return srv
}

// Writes entities to the given directory in files named entities-%d.textproto.
func writeEntitiesToFiles(entitiesFiles [][]*nbipb.Entity, filePath string) error {
	for i, entities := range entitiesFiles {
		entitiesProto := &nbipb.TxtpbEntities{Entity: entities}
		textprotoBytes, err := prototext.MarshalOptions{Multiline: true}.Marshal(entitiesProto)
		if err != nil {
			return err
		}

		err = os.WriteFile(filepath.Join(filePath, fmt.Sprintf("entities-%d.textproto", i)), textprotoBytes, 0o666)
		if err != nil {
			return err
		}
	}

	return nil
}

func readEntitiesFromFile(filePath string) ([]*nbipb.Entity, error) {
	files, err := filepath.Glob(filePath)
	if err != nil {
		return nil, fmt.Errorf("unable to expand the file path: %w", err)
	} else if len(files) == 0 {
		return nil, fmt.Errorf("no files found under the given file path: %s", filePath)
	}

	entities := make([]*nbipb.Entity, 0, len(files))
	for _, f := range files {
		msg, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("invalid file path: %w", err)
		}

		entity := &nbipb.Entity{}
		if err := prototext.Unmarshal(msg, entity); err != nil {
			return nil, fmt.Errorf("invalid file contents: %w", err)
		}
		entities = append(entities, entity)
	}

	return entities, nil
}

// Returns PLATFORM_DEFINITION entities whose IDs are 'entity-{f}-{i}',
// where f runs from 0 to (numFiles - 1) and i runs from 0 to (numEntities - 1).
func buildTestEntities(numFiles int, numEntities int) [][]*nbipb.Entity {
	files := make([][]*nbipb.Entity, 0)
	for f := 0; f < numFiles; f++ {
		entities := make([]*nbipb.Entity, 0)
		for i := 0; i < numEntities; i++ {
			entityIdStr := fmt.Sprintf("entity-%d-%d", f, i)
			entities = append(entities, &nbipb.Entity{
				Id: proto.String(entityIdStr),
				Group: &nbipb.EntityGroup{
					Type: nbipb.EntityType_PLATFORM_DEFINITION.Enum(),
				},
				Value: &nbipb.Entity_Platform{
					Platform: &commonpb.PlatformDefinition{
						Name: proto.String(entityIdStr),
					},
				},
			})
			files = append(files, entities)
		}
	}
	return files
}

func TestEndToEnd(t *testing.T) {
	t.Parallel()

	type endToEndTestCase struct {
		name string
		cmd  []string
		// Entities that are provided as inputs to the test case.
		// These are written to a temporary directory, which is
		// passed in the --files argument.
		entitiesFiles [][]*nbipb.Entity
		expectFn      func([]byte) error
		// Whether to verify the output of the test based on the
		// state of the fake server.
		expectServerStateFn func(*FakeNetOpsServer) error
		// Whether to verify the output of the test based on the
		// contents of the files containing the entities.
		expectEntityFilesFn func(string) error
		changeServer        func(*FakeNetOpsServer)
	}

	expectLines := func(lines ...string) func([]byte) error {
		return func(gotData []byte) error {
			got := strings.Split(strings.TrimSpace(string(gotData)), "\n")
			if diff := cmp.Diff(lines, got); diff != "" {
				return fmt.Errorf("output mismatch: (-want +got):\n%s", diff)
			}
			return nil
		}
	}

	// We can't use expectLines for prototext output because the format isn't
	// stable (the authors intentionally sometimes vary the format by using two
	// spaces between field name + colon and value), so instead we just verify
	// that the result can get unmarshalled using the prototext library.
	expectTextProto := func(want proto.Message) func([]byte) error {
		return func(gotData []byte) error {
			got := proto.Clone(want)
			if err := prototext.Unmarshal(gotData, got); err != nil {
				return err
			}
			if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
				return fmt.Errorf("output mismatch: (-want +got):\n%s", diff)
			}
			return nil
		}
	}

	// Verifies that all of the expected entities were processed by the server.
	expectEntityIDs := func(expectedEntities [][]*nbipb.Entity) func(*FakeNetOpsServer) error {
		return func(testServer *FakeNetOpsServer) error {
			missingEntityIDs := []string{}
			for _, entities := range expectedEntities {
				for _, e := range entities {
					id := e.GetId()
					if _, ok := testServer.EntityIDsModified[id]; !ok {
						missingEntityIDs = append(missingEntityIDs, id)
					}
				}
			}

			if len(missingEntityIDs) > 0 {
				return fmt.Errorf("expected entities were not modified:\n%v", missingEntityIDs)
			}
			return nil
		}
	}

	// Verifies that the given message was received as the most recent request by the server.
	expectLatestRequest := func(want proto.Message) func(*FakeNetOpsServer) error {
		return func(testServer *FakeNetOpsServer) error {
			if diff := cmp.Diff(want, testServer.LatestRequest, protocmp.Transform()); diff != "" {
				return fmt.Errorf("output mismatch: (-want +got):\n%s", diff)
			}
			return nil
		}
	}

	defaultTestEntities := buildTestEntities(5, 10)

	listResponse := &nbipb.ListEntitiesResponse{
		Entities: []*nbipb.Entity{
			{
				Group:               &nbipb.EntityGroup{},
				Id:                  proto.String("b0ba-cafe"),
				CommitTimestamp:     proto.Int64(1),
				NextCommitTimestamp: proto.Int64(2),
				LastModifiedBy:      proto.String("your friend Ciaran"),
				Value: &nbipb.Entity_NetworkNode{
					NetworkNode: &respb.NetworkNode{},
				},
			},
		},
	}
	listOutput := &nbipb.TxtpbEntities{
		Entity: listResponse.Entities,
	}

	testCases := []endToEndTestCase{
		{
			name: "grpcurl describe netops",
			cmd:  []string{"grpcurl", "describe", "aalyria.spacetime.api.nbi.v1alpha.NetOps"},
			expectFn: expectLines(
				"aalyria.spacetime.api.nbi.v1alpha.NetOps is a service",
			),
		},
		{
			name: "grpcurl describe",
			cmd:  []string{"grpcurl", "describe"},
			expectFn: expectLines(
				"aalyria.spacetime.api.nbi.v1alpha.NetOps is a service",
				"grpc.reflection.v1.ServerReflection is a service",
				"grpc.reflection.v1alpha.ServerReflection is a service",
			),
		},
		{
			name: "grpcurl list",
			cmd:  []string{"grpcurl", "list"},
			expectFn: expectLines(
				"aalyria.spacetime.api.nbi.v1alpha.NetOps",
				"grpc.reflection.v1.ServerReflection",
				"grpc.reflection.v1alpha.ServerReflection",
			),
		},
		{
			name: "grpcurl list netops",
			cmd:  []string{"grpcurl", "list", "aalyria.spacetime.api.nbi.v1alpha.NetOps"},
			expectFn: expectLines(
				"aalyria.spacetime.api.nbi.v1alpha.NetOps.CreateEntity",
				"aalyria.spacetime.api.nbi.v1alpha.NetOps.DeleteEntity",
				"aalyria.spacetime.api.nbi.v1alpha.NetOps.GetEntity",
				"aalyria.spacetime.api.nbi.v1alpha.NetOps.ListEntities",
				"aalyria.spacetime.api.nbi.v1alpha.NetOps.ListEntitiesOverTime",
				"aalyria.spacetime.api.nbi.v1alpha.NetOps.UpdateEntity",
				"aalyria.spacetime.api.nbi.v1alpha.NetOps.VersionInfo",
			),
		},
		{
			name: "list",
			cmd:  []string{"list", "-t", "NETWORK_NODE"},
			changeServer: func(srv *FakeNetOpsServer) {
				srv.ListEntityResponse = listResponse
			},
			expectFn: expectTextProto(listOutput),
		},
		{
			name: "list with field mask",
			cmd:  []string{"list", "-t", "NETWORK_NODE", "--field_masks", "network_node.name,network_node.type"},
			expectServerStateFn: expectLatestRequest(&nbipb.ListEntitiesRequest{
				Type:   nbipb.EntityType_NETWORK_NODE.Enum(),
				Filter: &nbipb.EntityFilter{FieldMasks: []string{"network_node.name", "network_node.type"}},
			}),
		},
		{
			name:                "create",
			cmd:                 []string{"create"},
			entitiesFiles:       defaultTestEntities,
			expectServerStateFn: expectEntityIDs(defaultTestEntities),
		},
		{
			name:                "update",
			cmd:                 []string{"update"},
			entitiesFiles:       defaultTestEntities,
			expectServerStateFn: expectEntityIDs(defaultTestEntities),
		},
		{
			name: "delete single entity",
			cmd:  []string{"delete", "--type", "PLATFORM_DEFINITION", "--id", "my-id", "--last_commit_timestamp", "123456"},
			expectServerStateFn: expectEntityIDs([][]*nbipb.Entity{{
				{
					Group: &nbipb.EntityGroup{
						Type: nbipb.EntityType_PLATFORM_DEFINITION.Enum(),
					},
					Id:              proto.String("my-id"),
					CommitTimestamp: proto.Int64(123456),
				},
			}}),
		},
		{
			name:                "delete from files",
			cmd:                 []string{"delete"},
			entitiesFiles:       defaultTestEntities,
			expectServerStateFn: expectEntityIDs(defaultTestEntities),
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			g, ctx := errgroup.WithContext(ctx)
			defer func() { checkErr(t, g.Wait()) }()
			defer cancel()

			tmpDir, err := bazel.NewTmpDir("nbictl")
			checkErr(t, err)

			srv := startInsecureServer(ctx, t, g)
			if tc.changeServer != nil {
				tc.changeServer(srv)
			}

			keys := generateKeysForTesting(t, tmpDir, "--org", "example org")
			checkErr(t, newTestApp().Run([]string{
				"nbictl", "--config_dir", tmpDir,
				"set-config",
				"--transport_security", "insecure",
				"--user_id", "usr1",
				"--key_id", "key1",
				"--priv_key", keys.key,
				"--url", srv.listener.Addr().String(),
			}))

			app := newTestApp()
			args := append([]string{"nbictl", "--config_dir", tmpDir, "--context", "DEFAULT"}, tc.cmd...)

			entitiesFilesDir, err := bazel.NewTmpDir("entities")
			checkErr(t, err)
			// Makes the entity files directory into a glob from which the entities are read.
			entitiesFilesGlob := entitiesFilesDir + "/*"
			if tc.entitiesFiles != nil {
				checkErr(t, writeEntitiesToFiles(tc.entitiesFiles, entitiesFilesDir))
				args = append(args, "--files", entitiesFilesGlob)
			}

			checkErr(t, app.Run(args))
			if tc.expectFn != nil {
				checkErr(t, tc.expectFn(app.stdout.Bytes()))
			}
			if tc.expectServerStateFn != nil {
				checkErr(t, tc.expectServerStateFn(srv))
			}
			if tc.expectEntityFilesFn != nil {
				checkErr(t, tc.expectEntityFilesFn(entitiesFilesGlob))
			}
		})
	}
}
