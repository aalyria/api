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

	modelpb "aalyria.com/spacetime/api/model/v1alpha"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
	gcmp "github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	nmtspb "outernetcouncil.org/nmts/proto"
	nmtsphypb "outernetcouncil.org/nmts/proto/ek/physical"
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

	os.Chdir(tmpDir)

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
		"set-config",
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
		desc:            "'model upsert-entity' without arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "upsert-entity"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model upsert-entity' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "upsert-entity", "uuid-1234.txtpb", "uuid-5678.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model upsert-entity' with one argument errors if file is missing",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "upsert-entity", "uuid-1234.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc: "'model upsert-entity' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"-": "id: \"uuid-1234\" ek_platform{}",
		},
		responseError:   nil,
		responseMessage: &modelpb.UpsertEntityResponse{},
		cmdLineArgs:     []string{"model", "upsert-entity", "-"},
		wantAppError:    false,
		wantRequest: &modelpb.UpsertEntityRequest{
			Entity: &nmtspb.Entity{
				Id: "uuid-1234",
				Kind: &nmtspb.Entity_EkPlatform{
					EkPlatform: &nmtsphypb.Platform{},
				},
			},
		},
	},
	{
		desc: "'model upsert-entity' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"uuid-1234.txtpb": "id: \"uuid-1234\" ek_platform{}",
		},
		responseError:   nil,
		responseMessage: &modelpb.UpsertEntityResponse{},
		cmdLineArgs:     []string{"model", "upsert-entity", "uuid-1234.txtpb"},
		wantAppError:    false,
		wantRequest: &modelpb.UpsertEntityRequest{
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
		cmdLineArgs:     []string{"model", "update-entity"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model update-entity' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "update-entity", "uuid-1234.txtpb", "uuid-5678.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model update-entity' with one argument errors if file is missing",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "update-entity", "uuid-1234.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc: "'model update-entity' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"-": "entity{ id: \"uuid-1234\" ek_platform{ name: \"platform_1234\" } } mask: { paths: \"ek_platform.name\" }",
		},
		responseError:   nil,
		responseMessage: &modelpb.UpdateEntityResponse{},
		cmdLineArgs:     []string{"model", "update-entity", "-"},
		wantAppError:    false,
		wantRequest: &modelpb.UpdateEntityRequest{
			Patch: &nmtspb.PartialEntity{
				Entity: &nmtspb.Entity{
					Id: "uuid-1234",
					Kind: &nmtspb.Entity_EkPlatform{
						EkPlatform: &nmtsphypb.Platform{
							Name: "platform_1234",
						},
					},
				},
				Mask: &fieldmaskpb.FieldMask{
					Paths: []string{
						"ek_platform.name",
					},
				},
			},
		},
	},
	{
		desc: "'model update-entity' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"uuid-1234.txtpb": "entity{ id: \"uuid-1234\" ek_platform{ name: \"platform_1234\" } } mask: { paths: \"ek_platform.name\" }",
		},
		responseError:   nil,
		responseMessage: &modelpb.UpdateEntityResponse{},
		cmdLineArgs:     []string{"model", "update-entity", "uuid-1234.txtpb"},
		wantAppError:    false,
		wantRequest: &modelpb.UpdateEntityRequest{
			Patch: &nmtspb.PartialEntity{
				Entity: &nmtspb.Entity{
					Id: "uuid-1234",
					Kind: &nmtspb.Entity_EkPlatform{
						EkPlatform: &nmtsphypb.Platform{
							Name: "platform_1234",
						},
					},
				},
				Mask: &fieldmaskpb.FieldMask{
					Paths: []string{
						"ek_platform.name",
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
		cmdLineArgs:     []string{"model", "delete-entity"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model delete-entity' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "delete-entity", "uuid-1234", "uuid-5678"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model delete-entity' with one argument calls API as expected",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: &emptypb.Empty{},
		cmdLineArgs:     []string{"model", "delete-entity", "uuid-1234"},
		wantAppError:    false,
		wantRequest: &modelpb.DeleteEntityRequest{
			EntityId: "uuid-1234",
		},
	},
	{
		desc:            "'model insert-relationship' without arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "insert-relationship"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model insert-relationship' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "insert-relationship", "uuid-1234.txtpb", "uuid-5678.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model insert-relationship' with one argument errors if file is missing",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "insert-relationship", "uuid-1234.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc: "'model insert-relationship' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"-": "a: \"uuid-1234\" kind: RK_CONTAINS z: \"uuid-5678\"",
		},
		responseError:   nil,
		responseMessage: &modelpb.InsertRelationshipResponse{},
		cmdLineArgs:     []string{"model", "insert-relationship", "-"},
		wantAppError:    false,
		wantRequest: &modelpb.InsertRelationshipRequest{
			Relationship: &nmtspb.Relationship{
				A:    "uuid-1234",
				Kind: nmtspb.RK_RK_CONTAINS,
				Z:    "uuid-5678",
			},
		},
	},
	{
		desc: "'model insert-relationship' with one argument calls API as expected (stdin)",
		fileContents: map[string]string{
			"uuid-1234.txtpb": "a: \"uuid-1234\" kind: RK_CONTAINS z: \"uuid-5678\"",
		},
		responseError:   nil,
		responseMessage: &modelpb.InsertRelationshipResponse{},
		cmdLineArgs:     []string{"model", "insert-relationship", "uuid-1234.txtpb"},
		wantAppError:    false,
		wantRequest: &modelpb.InsertRelationshipRequest{
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
		cmdLineArgs:     []string{"model", "delete-relationship"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model delete-relationship' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "delete-relationship", "uuid-1234.txtpb", "uuid-5678.txtpb"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model delete-relationship' with one argument errors if file is missing",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "delete-relationship", "uuid-1234.txtpb"},
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
		cmdLineArgs:     []string{"model", "delete-relationship", "-"},
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
		cmdLineArgs:     []string{"model", "delete-relationship", "uuid-1234.txtpb"},
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
		cmdLineArgs:     []string{"model", "get-entity"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model get-entity' with too many arguments does not call API",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: nil,
		cmdLineArgs:     []string{"model", "get-entity", "uuid-1234", "uuid-5678"},
		wantAppError:    true,
		wantRequest:     nil,
	},
	{
		desc:            "'model get-entity' with one argument calls API as expected",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: &modelpb.GetEntityResponse{},
		cmdLineArgs:     []string{"model", "get-entity", "uuid-1234"},
		wantAppError:    false,
		wantRequest: &modelpb.GetEntityRequest{
			EntityId: "uuid-1234",
		},
	},
	{
		desc:            "'model list-elements' calls API as expected",
		fileContents:    nil,
		responseError:   nil,
		responseMessage: &modelpb.ListElementsResponse{},
		cmdLineArgs:     []string{"model", "list-elements"},
		wantAppError:    false,
		wantRequest:     &modelpb.ListElementsRequest{},
	},
}

func TestCases(t *testing.T) {
	for _, tc := range testCases {
		tc.Run(t)
	}
}
