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
	gcmp "github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	statuspb "aalyria.com/spacetime/api/status/v1"
)

type testCaseStatusApi struct {
	desc            string
	fileContents    map[string]string
	responseError   error
	responseMessage proto.Message
	cmdLineArgs     []string
	wantAppError    bool
	wantRequest     proto.Message
	expectFn        func([]byte) error
}

func (tc *testCaseStatusApi) Run(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	g, ctx := errgroup.WithContext(ctx)
	defer func() { checkErr(t, g.Wait()) }()
	defer cancel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("changing to tmp dir %s: %v", tmpDir, err)
	}

	app := newTestApp()
	for key, value := range tc.fileContents {
		if key == "-" {
			app.stdin.WriteString(value)
		} else {
			absFile, err := filepath.Abs(filepath.Join(tmpDir, key))
			checkErr(t, err)
			_, err = filepath.Rel(tmpDir, absFile)
			checkErr(t, err)
			checkErr(t, os.WriteFile(absFile, []byte(value), 0644))
		}
	}

	lis, err := net.Listen("tcp", ":0")
	checkErr(t, err)
	srv, err := startFakeStatusServer(ctx, g, lis)
	checkErr(t, err)

	argsPrefix := []string{"nbictl", "--config_dir", tmpDir, "--context", "DEFAULT"}

	keys := generateKeysForTesting(t, tmpDir, "--org", "example org")
	checkErr(t, newTestApp().Run(append(argsPrefix, []string{
		"config",
		"set",
		"--transport_security", "insecure",
		"--user_id", "usr1",
		"--key_id", "key1",
		"--priv_key", keys.key,
		"--url", srv.listener.Addr().String(),
	}...)))

	srv.ResponseMessage = tc.responseMessage
	args := append(argsPrefix, tc.cmdLineArgs...)
	err = app.Run(args)
	if tc.wantAppError {
		if err != nil {
			return
		} else {
			t.Errorf("[%v] wanted App error but got success", tc.desc)
		}
	}
	checkErr(t, err)
	if diff := gcmp.Diff(tc.wantRequest, srv.RequestMessage,
		protocmp.Transform(),
	); diff != "" {
		t.Errorf("[%v] mismatch (-want +got):\n%s", tc.desc, diff)
	}
	if tc.expectFn != nil {
		checkErr(t, tc.expectFn(app.stdout.Bytes()))
	}
}

var testCasesStatusApi = []testCaseStatusApi{
	{
		desc:          "'status-v1 get-version",
		fileContents:  nil,
		responseError: nil,
		responseMessage: &statuspb.GetVersionResponse{
			BuildVersion: proto.String("2.1.0"),
		},
		cmdLineArgs:  []string{"status-v1", "get-version"},
		wantAppError: false,
		wantRequest:  &statuspb.GetVersionRequest{},
		expectFn: expectTextProto(&statuspb.GetVersionResponse{
			BuildVersion: proto.String("2.1.0"),
		}),
	},
	{
		desc:          "'status-v1 get-version --format json",
		fileContents:  nil,
		responseError: nil,
		responseMessage: &statuspb.GetVersionResponse{
			BuildVersion: proto.String("2.9.0"),
		},
		cmdLineArgs:  []string{"status-v1", "get-version", "--format", "json"},
		wantAppError: false,
		wantRequest:  &statuspb.GetVersionRequest{},
		expectFn: expectJSON(&map[string]string{
			"buildVersion": "2.9.0",
		}),
	},
}

func TestCasesStatusApi(t *testing.T) {
	t.Parallel()
	for _, tc := range testCasesStatusApi {
		tc := tc
		t.Run(tc.desc, tc.Run)
	}
}
