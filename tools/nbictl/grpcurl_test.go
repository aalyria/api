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
	"encoding/json"
	"fmt"
	"net"
	"reflect"
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
)

type testApp struct {
	*cli.App

	stdin  *bytes.Buffer
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func newTestApp() testApp {
	stdin, stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}
	app := App()
	app.Reader = stdin
	app.Writer = stdout
	app.ErrWriter = stderr
	return testApp{stdin: stdin, stdout: stdout, stderr: stderr, App: app}
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

func TestGrpcurl_list(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	g, ctx := errgroup.WithContext(ctx)
	defer func() { checkErr(t, g.Wait()) }()
	defer cancel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	srv := startInsecureServer(ctx, t, g)

	keys := generateKeysForTesting(t, tmpDir, "--org", "example org")
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir,
		"config",
		"set",
		"--transport_security", "insecure",
		"--user_id", "usr1",
		"--key_id", "key1",
		"--priv_key", keys.key,
		"--url", srv.listener.Addr().String(),
	}))

	app := newTestApp()
	args := []string{"nbictl", "--config_dir", tmpDir, "grpcurl", "list"}
	checkErr(t, app.Run(args))
	checkErr(t, expectLines(
		app.stdout.Bytes(),
		"aalyria.spacetime.api.model.v1.Model",
		"grpc.reflection.v1.ServerReflection",
		"grpc.reflection.v1alpha.ServerReflection",
	))

	app = newTestApp()
	args = []string{"nbictl", "--config_dir", tmpDir, "grpcurl", "list", "aalyria.spacetime.api.model.v1.Model"}
	checkErr(t, app.Run(args))
	checkErr(t, expectLines(
		app.stdout.Bytes(),
		"aalyria.spacetime.api.model.v1.Model.CreateEntity",
		"aalyria.spacetime.api.model.v1.Model.CreateRelationship",
		"aalyria.spacetime.api.model.v1.Model.DeleteEntity",
		"aalyria.spacetime.api.model.v1.Model.DeleteRelationship",
		"aalyria.spacetime.api.model.v1.Model.GetEntity",
		"aalyria.spacetime.api.model.v1.Model.ListEntities",
		"aalyria.spacetime.api.model.v1.Model.ListRelationships",
		"aalyria.spacetime.api.model.v1.Model.UpdateEntity",
	))
}

func startInsecureServer(ctx context.Context, t *testing.T, g *errgroup.Group) *FakeModelServer {
	lis, err := net.Listen("tcp", ":0")
	checkErr(t, err)
	srv, err := startFakeModelServer(ctx, g, lis)
	checkErr(t, err)

	return srv
}

func expectLines(gotData []byte, lines ...string) error {
	got := strings.Split(strings.TrimSpace(string(gotData)), "\n")
	if diff := cmp.Diff(lines, got); diff != "" {
		return fmt.Errorf("output mismatch: (-want +got):\n%s", diff)
	}
	return nil
}

// We can't use expectLines for prototext output because the format isn't
// stable (the authors intentionally sometimes vary the format by using two
// spaces between field name + colon and value), so instead we just verify
// that the result can get unmarshalled using the prototext library.
func expectTextProto(want proto.Message) func([]byte) error {
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

func expectJSON(want any) func([]byte) error {
	return func(gotData []byte) error {
		// Create a new instance of the same type as want
		got := reflect.New(reflect.TypeOf(want).Elem()).Interface()

		if err := json.Unmarshal(gotData, got); err != nil {
			return err
		}

		if diff := cmp.Diff(want, got); diff != "" {
			return fmt.Errorf("output mismatch: (-want +got):\n%s", diff)
		}
		return nil
	}
}
