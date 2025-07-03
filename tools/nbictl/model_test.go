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
	"google.golang.org/protobuf/types/known/emptypb"
	nmtspb "outernetcouncil.org/nmts/v1/proto"
	nmtsphypb "outernetcouncil.org/nmts/v1/proto/ek/physical"

	modelpb "aalyria.com/spacetime/api/model/v1"
)

type testCase struct {
	desc            string
	fileContents    map[string]string
	responseError   error
	responseMessage proto.Message
	cmdLineArgs     []string
	wantAppError    bool
	wantRequest     proto.Message
}

func (tc *testCase) Run(t *testing.T) {
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
	srv, err := startFakeModelServer(ctx, g, lis)
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
}

var testCases = []testCase{
	{
		desc:            "'model create-entity' without arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "create-entity"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model create-entity' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "create-entity", "uuid-1234.txtpb", "uuid-5678.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model create-entity' with one argument errors if file is missing",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "create-entity", "uuid-1234.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc: "'model create-entity' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"-": "id: \"uuid-1234\" ek_platform{}",
		},
		responseError: nil,
		responseMessage: &nmtspb.Entity{
			Id: "uuid-1234",
			Kind: &nmtspb.Entity_EkPlatform{
				EkPlatform: &nmtsphypb.Platform{},
			},
		},
		cmdLineArgs:  []string{"model-v1", "create-entity", "-"},
		wantAppError: false,
		wantRequest: &modelpb.CreateEntityRequest{
			Entity: &nmtspb.Entity{
				Id: "uuid-1234",
				Kind: &nmtspb.Entity_EkPlatform{
					EkPlatform: &nmtsphypb.Platform{},
				},
			},
		},
	},
	{
		desc: "'model create-entity' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"uuid-1234.txtpb": "id: \"uuid-1234\" ek_platform{}",
		},
		responseError: nil,
		responseMessage: &nmtspb.Entity{
			Id: "uuid-1234",
			Kind: &nmtspb.Entity_EkPlatform{
				EkPlatform: &nmtsphypb.Platform{},
			},
		},
		cmdLineArgs:  []string{"model-v1", "create-entity", "uuid-1234.txtpb"},
		wantAppError: false,
		wantRequest: &modelpb.CreateEntityRequest{
			Entity: &nmtspb.Entity{
				Id: "uuid-1234",
				Kind: &nmtspb.Entity_EkPlatform{
					EkPlatform: &nmtsphypb.Platform{},
				},
			},
		},
	},
	{
		desc:            "'model update-entity' without arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "update-entity"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model update-entity' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "update-entity", "uuid-1234.txtpb", "uuid-5678.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model update-entity' with one argument errors if file is missing",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "update-entity", "uuid-1234.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc: "'model update-entity' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"-": "id: \"uuid-1234\" ek_platform{ name: \"platform_1234\" }",
		},
		responseError: nil,
		responseMessage: &nmtspb.Entity{
			Id: "uuid-1234",
			Kind: &nmtspb.Entity_EkPlatform{
				EkPlatform: &nmtsphypb.Platform{
					Name: "platform_1234",
				},
			},
		},
		cmdLineArgs:  []string{"model-v1", "update-entity", "-"},
		wantAppError: false,
		wantRequest: &modelpb.UpdateEntityRequest{
			Entity: &nmtspb.Entity{
				Id: "uuid-1234",
				Kind: &nmtspb.Entity_EkPlatform{
					EkPlatform: &nmtsphypb.Platform{
						Name: "platform_1234",
					},
				},
			},
		},
	},
	{
		desc: "'model update-entity' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"uuid-1234.txtpb": "id: \"uuid-1234\" ek_platform { name: \"platform_1234\" }",
		},
		responseError: nil,
		responseMessage: &nmtspb.Entity{
			Id: "uuid-1234",
			Kind: &nmtspb.Entity_EkPlatform{
				EkPlatform: &nmtsphypb.Platform{
					Name: "platform_1234",
				},
			},
		},
		cmdLineArgs:  []string{"model-v1", "update-entity", "uuid-1234.txtpb"},
		wantAppError: false,
		wantRequest: &modelpb.UpdateEntityRequest{
			Entity: &nmtspb.Entity{
				Id: "uuid-1234",
				Kind: &nmtspb.Entity_EkPlatform{
					EkPlatform: &nmtsphypb.Platform{
						Name: "platform_1234",
					},
				},
			},
		},
	},
	{
		desc:            "'model delete-entity' without arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "delete-entity"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model delete-entity' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "delete-entity", "uuid-1234", "uuid-5678"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model delete-entity' with one argument calls API as expected",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: &modelpb.DeleteEntityResponse{},
		cmdLineArgs:     []string{"model-v1", "delete-entity", "uuid-1234"},
		wantAppError:    false,
		wantRequest: &modelpb.DeleteEntityRequest{
			EntityId: "uuid-1234",
		},
	},
	{
		desc:            "'model create-relationship' without arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "create-relationship"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model create-relationship' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "create-relationship", "uuid-1234.txtpb", "uuid-5678.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model create-relationship' with one argument errors if file is missing",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "create-relationship", "uuid-1234.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc: "'model create-relationship' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"-": "a: \"uuid-1234\" kind: RK_CONTAINS z: \"uuid-5678\"",
		},
		responseError: nil,
		responseMessage: &nmtspb.Relationship{
			A:    "uuid-1234",
			Kind: nmtspb.RK_RK_CONTAINS,
			Z:    "uuid-5678",
		},
		cmdLineArgs:  []string{"model-v1", "create-relationship", "-"},
		wantAppError: false,
		wantRequest: &modelpb.CreateRelationshipRequest{
			Relationship: &nmtspb.Relationship{
				A:    "uuid-1234",
				Kind: nmtspb.RK_RK_CONTAINS,
				Z:    "uuid-5678",
			},
		},
	},
	{
		desc: "'model create-relationship' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"uuid-1234.txtpb": "a: \"uuid-1234\" kind: RK_CONTAINS z: \"uuid-5678\"",
		},
		responseError: nil,
		responseMessage: &nmtspb.Relationship{
			A:    "uuid-1234",
			Kind: nmtspb.RK_RK_CONTAINS,
			Z:    "uuid-5678",
		},
		cmdLineArgs:  []string{"model-v1", "create-relationship", "uuid-1234.txtpb"},
		wantAppError: false,
		wantRequest: &modelpb.CreateRelationshipRequest{
			Relationship: &nmtspb.Relationship{
				A:    "uuid-1234",
				Kind: nmtspb.RK_RK_CONTAINS,
				Z:    "uuid-5678",
			},
		},
	},
	{
		desc:            "'model delete-relationship' without arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "delete-relationship"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model delete-relationship' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "delete-relationship", "uuid-1234.txtpb", "uuid-5678.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model delete-relationship' with one argument errors if file is missing",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "delete-relationship", "uuid-1234.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc: "'model delete-relationship' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"-": "a: \"uuid-1234\" kind: RK_CONTAINS z: \"uuid-5678\"",
		},
		responseError:   nil,
		responseMessage: &emptypb.Empty{},
		cmdLineArgs:     []string{"model-v1", "delete-relationship", "-"},
		wantAppError:    false,
		wantRequest: &modelpb.DeleteRelationshipRequest{
			Relationship: &nmtspb.Relationship{
				A:    "uuid-1234",
				Kind: nmtspb.RK_RK_CONTAINS,
				Z:    "uuid-5678",
			},
		},
	},
	{
		desc: "'model delete-relationship' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"uuid-1234.txtpb": "a: \"uuid-1234\" kind: RK_CONTAINS z: \"uuid-5678\"",
		},
		responseError:   nil,
		responseMessage: &emptypb.Empty{},
		cmdLineArgs:     []string{"model-v1", "delete-relationship", "uuid-1234.txtpb"},
		wantAppError:    false,
		wantRequest: &modelpb.DeleteRelationshipRequest{
			Relationship: &nmtspb.Relationship{
				A:    "uuid-1234",
				Kind: nmtspb.RK_RK_CONTAINS,
				Z:    "uuid-5678",
			},
		},
	},
	{
		desc:            "'model get-entity' without arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "get-entity"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model get-entity' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model-v1", "get-entity", "uuid-1234", "uuid-5678"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model get-entity' with one argument calls API as expected",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: &nmtspb.Entity{},
		cmdLineArgs:     []string{"model-v1", "get-entity", "uuid-1234"},
		wantAppError:    false,
		wantRequest: &modelpb.GetEntityRequest{
			EntityId: "uuid-1234",
		},
	},
	{
		desc:            "'model list-entities' calls API as expected",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: &modelpb.ListEntitiesResponse{},
		cmdLineArgs:     []string{"model-v1", "list-entities"},
		wantAppError:    false,
		wantRequest:     &modelpb.ListEntitiesRequest{},
	},
	{
		desc:            "'model list-relationships' calls API as expected",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: &modelpb.ListRelationshipsResponse{},
		cmdLineArgs:     []string{"model-v1", "list-relationships"},
		wantAppError:    false,
		wantRequest:     &modelpb.ListRelationshipsRequest{},
	},
}

func TestCases(t *testing.T) {
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.desc, tc.Run)
	}
}
