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
		"nbictl", "delete", "--type", "UFO", "--timestamp", "3", "--id", "boba-cafe",
	}); {
	case err == nil:
		t.Fatal("expected --type UFO to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}
}

func TestDelete_requiresID(t *testing.T) {
	t.Parallel()

	switch want, err := `Required flag "id" not set`, newTestApp().Run([]string{
		"nbictl", "delete", "--type", "NETWORK_NODE", "--timestamp", "3",
	}); {
	case err == nil:
		t.Fatal("expected missing --id to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %q, but got %q", want, err.Error())
	}
}

func TestDelete_requiresTimestamp(t *testing.T) {
	t.Parallel()

	switch want, err := `Required flag "timestamp" not set`, newTestApp().Run([]string{
		"nbictl", "delete", "--type", "NETWORK_NODE", "--id", "boba-cafe",
	}); {
	case err == nil:
		t.Fatal("expected missing --timestamp to cause an error, got nil")
	case !strings.Contains(err.Error(), want):
		t.Fatalf("expected error to contain %v, but got %s", want, err.Error())
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

func TestEndToEnd(t *testing.T) {
	t.Parallel()

	type endToEndTestCase struct {
		name         string
		cmd          []string
		expectFn     func([]byte) error
		changeServer func(*FakeNetOpsServer)
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
				"grpc.reflection.v1alpha.ServerReflection is a service",
			),
		},
		{
			name: "grpcurl list",
			cmd:  []string{"grpcurl", "list"},
			expectFn: expectLines(
				"aalyria.spacetime.api.nbi.v1alpha.NetOps",
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
				"aalyria.spacetime.api.nbi.v1alpha.NetOps.LoadScenario",
				"aalyria.spacetime.api.nbi.v1alpha.NetOps.UpdateEntity",
			),
		},
		{
			name: "list",
			cmd:  []string{"list", "-t", "NETWORK_NODE"},
			changeServer: func(srv *FakeNetOpsServer) {
				srv.ListEntityResponse = listResponse
			},
			expectFn: expectTextProto(listResponse),
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
			checkErr(t, app.Run(append([]string{"nbictl", "--config_dir", tmpDir, "--context", "DEFAULT"}, tc.cmd...)))
			checkErr(t, tc.expectFn(app.stdout.Bytes()))
		})
	}
}
