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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "aalyria.com/spacetime/github/tools/nbictl/resource"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	testContext = &pb.Context{
		Name:    "unit_testing",
		KeyId:   "privateKey.id",
		Email:   "privateKey.userID",
		PrivKey: "privateKey.path",
		Url:     "test_url",
		OidcUrl: "test_oidc",
	}
	testContextForUpdate = &pb.Context{
		Name:    "test update",
		KeyId:   "update_key_id",
		Email:   "update_user_id",
		PrivKey: "update_priv_key",
		Url:     "update_url",
		OidcUrl: "update_oidc",
	}
	testContexts = &pb.NbiCtlConfig{
		Contexts: []*pb.Context{testContext},
	}
)

func TestGetContext(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	err = setContext(testContext, nbictlConfig)
	checkErr(t, err)

	got, err := GetContext(testContext.GetName(), nbictlConfig)
	checkErr(t, err)

	assertProtosEqual(t, testContext, got)
}

func TestGetContext_WithOnlyOneContextInConfig(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	nbictlConfig = filepath.Join(nbictlConfig, confFileName)
	checkErr(t, setContext(testContext, nbictlConfig))

	got, err := GetContext("", nbictlConfig)
	checkErr(t, err)

	assertProtosEqual(t, testContext, got)
}

func TestGetContext_WithNoContextWithMultipleContextInConfig(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	contextsToCreate := []string{
		"test_1",
		"test_2",
		"test_3",
	}
	wantErrMsg := "--context flag required because there are multiple contexts defined in the configuration."

	for _, contextToCreate := range contextsToCreate {
		context := &pb.Context{
			Name: contextToCreate,
		}
		checkErr(t, setContext(context, nbictlConfig))
	}

	_, err = GetContext("", nbictlConfig)
	gotErrMsg := err.Error()
	if !strings.Contains(gotErrMsg, wantErrMsg) {
		t.Fatalf("want: %s, got %s", wantErrMsg, gotErrMsg)
	}
}

func TestGetContexts_WithFileWithNoPermission(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	file, err := os.Create(nbictlConfig)
	checkErr(t, err)
	defer file.Close()

	checkErr(t, os.Chmod(nbictlConfig, 0000))

	if _, err = getContexts(nbictlConfig); err == nil {
		t.Fatal("unable to detect that issues with selected config file")
		checkErr(t, os.Remove(nbictlConfig))
		t.FailNow()
	}
}

func TestGetContext_WithNonExistingContextName(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	contextsToCreate := []string{
		"test_1",
		"test_2",
		"test_3",
	}
	nonExistingContext := "non_existing"
	wantErrMsg := fmt.Sprintf("unable to get the context with the name: %s. the list of available context names are the following: %v", nonExistingContext, contextsToCreate)

	for _, contextToCreate := range contextsToCreate {
		context := &pb.Context{
			Name: contextToCreate,
		}
		checkErr(t, setContext(context, nbictlConfig))
	}

	_, err = GetContext(nonExistingContext, nbictlConfig)
	gotErrMsg := err.Error()
	if !strings.Contains(gotErrMsg, wantErrMsg) {
		t.Fatalf("want: %s, got %s", wantErrMsg, gotErrMsg)
	}
}

func TestSetContext_WithNoUpdate(t *testing.T) {
	t.Parallel()
	// create a temporary directory
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	// initial setup
	checkErr(t, setContext(testContext, nbictlConfig))
	wantContexts, err := getContexts(nbictlConfig)
	checkErr(t, err)
	assertProtosEqual(t, testContexts, wantContexts)

	contextWithNoChange := &pb.Context{
		Name: testContext.GetName(),
	}
	checkErr(t, setContext(contextWithNoChange, nbictlConfig))

	gotContexts, err := getContexts(nbictlConfig)
	checkErr(t, err)
	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetContext_UpdatePrivateKey(t *testing.T) {
	t.Parallel()
	// create a temporary directory
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	// initial setup
	checkErr(t, setContext(testContext, nbictlConfig))
	checkErr(t, setContext(testContextForUpdate, nbictlConfig))

	// update the existing context with a new private key
	checkErr(t, setContext(&pb.Context{
		Name:    testContext.GetName(),
		PrivKey: "private_key.updated",
	}, nbictlConfig))

	updatedContext := proto.Clone(testContext).(*pb.Context)
	updatedContext.PrivKey = "private_key.updated"
	wantContexts := &pb.NbiCtlConfig{
		Contexts: []*pb.Context{updatedContext, testContextForUpdate},
	}

	// check if the private key is updated
	gotContexts, err := getContexts(nbictlConfig)
	checkErr(t, err)
	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetContext_UpdateKeyId(t *testing.T) {
	t.Parallel()
	// create a temporary directory
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	// initial setup
	checkErr(t, setContext(testContext, nbictlConfig))
	checkErr(t, setContext(testContextForUpdate, nbictlConfig))

	// update the existing context with a new key id
	checkErr(t, setContext(&pb.Context{
		Name:  testContextForUpdate.GetName(),
		KeyId: "key_id.updated",
	}, nbictlConfig))

	updatedContext := proto.Clone(testContextForUpdate).(*pb.Context)
	updatedContext.KeyId = "key_id.updated"
	wantContexts := &pb.NbiCtlConfig{
		Contexts: []*pb.Context{testContext, updatedContext},
	}

	// check if the key id is updated
	gotContexts, err := getContexts(nbictlConfig)
	checkErr(t, err)

	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetContext_UpdateUserID(t *testing.T) {
	t.Parallel()
	// create a temporary directory
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	// initial setup
	checkErr(t, setContext(testContext, nbictlConfig))
	checkErr(t, setContext(testContextForUpdate, nbictlConfig))

	// update the existing context with a new user id

	checkErr(t, setContext(&pb.Context{
		Name:  testContext.GetName(),
		Email: "email.updated",
	}, nbictlConfig))

	updatedContext := proto.Clone(testContext).(*pb.Context)
	updatedContext.Email = "email.updated"
	wantContexts := &pb.NbiCtlConfig{
		Contexts: []*pb.Context{updatedContext, testContextForUpdate},
	}

	// check if the user id is updated
	gotContexts, err := getContexts(nbictlConfig)
	checkErr(t, err)

	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetContext_UpdateUrl(t *testing.T) {
	t.Parallel()
	// create a temporary directory
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	// initial setup
	checkErr(t, setContext(testContext, nbictlConfig))
	checkErr(t, setContext(testContextForUpdate, nbictlConfig))

	// update the existing context with a new url

	checkErr(t, setContext(&pb.Context{
		Name: testContext.GetName(),
		Url:  "url.updated",
	}, nbictlConfig))

	updatedContext := proto.Clone(testContext).(*pb.Context)
	updatedContext.Url = "url.updated"
	wantContexts := &pb.NbiCtlConfig{
		Contexts: []*pb.Context{updatedContext, testContextForUpdate},
	}

	// check if the url is updated
	gotContexts, err := getContexts(nbictlConfig)
	checkErr(t, err)

	assertProtosEqual(t, wantContexts, gotContexts)
}

func checkErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertProtosEqual(t *testing.T, want, got interface{}) {
	t.Helper()

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("proto mismatch: (-want +got):\n%s", diff)
		t.FailNow()
	}
}
