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

	"aalyria.com/spacetime/github/tools/nbictl/nbictlpb"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	testConfig = &nbictlpb.Config{
		Name:    "unit_testing",
		KeyId:   "privateKey.id",
		Email:   "privateKey.userID",
		PrivKey: "privateKey.path",
		Url:     "test_url",
		OidcUrl: "test_oidc",
	}
	testConfigForUpdate = &nbictlpb.Config{
		Name:    "test update",
		KeyId:   "update_key_id",
		Email:   "update_user_id",
		PrivKey: "update_priv_key",
		Url:     "update_url",
		OidcUrl: "update_oidc",
	}
	testConfigs = &nbictlpb.AppConfig{
		Configs: []*nbictlpb.Config{testConfig},
	}
)

func TestGetConfig(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	err = setConfig(testConfig, nbictlConfig)
	checkErr(t, err)

	got, err := GetConfig(testConfig.GetName(), nbictlConfig)
	checkErr(t, err)

	assertProtosEqual(t, testConfig, got)
}

func TestGetConfig_WithOnlyOneContextInConfig(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	nbictlConfig = filepath.Join(nbictlConfig, confFileName)
	checkErr(t, setConfig(testConfig, nbictlConfig))

	got, err := GetConfig("", nbictlConfig)
	checkErr(t, err)

	assertProtosEqual(t, testConfig, got)
}

func TestGetConfig_WithNoContextWithMultipleContextInConfig(t *testing.T) {
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
		context := &nbictlpb.Config{
			Name: contextToCreate,
		}
		checkErr(t, setConfig(context, nbictlConfig))
	}

	_, err = GetConfig("", nbictlConfig)
	gotErrMsg := err.Error()
	if !strings.Contains(gotErrMsg, wantErrMsg) {
		t.Fatalf("want: %s, got %s", wantErrMsg, gotErrMsg)
	}
}

func TestGetConfigs_WithFileWithNoPermission(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	file, err := os.Create(nbictlConfig)
	checkErr(t, err)
	defer file.Close()

	checkErr(t, os.Chmod(nbictlConfig, 0000))

	if _, err = getConfigs(nbictlConfig); err == nil {
		t.Fatal("unable to detect that issues with selected config file")
		checkErr(t, os.Remove(nbictlConfig))
		t.FailNow()
	}
}

func TestGetConfig_WithNonExistingContextName(t *testing.T) {
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
		context := &nbictlpb.Config{
			Name: contextToCreate,
		}
		checkErr(t, setConfig(context, nbictlConfig))
	}

	_, err = GetConfig(nonExistingContext, nbictlConfig)
	gotErrMsg := err.Error()
	if !strings.Contains(gotErrMsg, wantErrMsg) {
		t.Fatalf("want: %s, got %s", wantErrMsg, gotErrMsg)
	}
}

func TestSetConfig_WithNoUpdate(t *testing.T) {
	t.Parallel()
	// create a temporary directory
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	// initial setup
	checkErr(t, setConfig(testConfig, nbictlConfig))
	wantContexts, err := getConfigs(nbictlConfig)
	checkErr(t, err)
	assertProtosEqual(t, testConfigs, wantContexts)

	contextWithNoChange := &nbictlpb.Config{
		Name: testConfig.GetName(),
	}
	checkErr(t, setConfig(contextWithNoChange, nbictlConfig))

	gotContexts, err := getConfigs(nbictlConfig)
	checkErr(t, err)
	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetConfig_UpdatePrivateKey(t *testing.T) {
	t.Parallel()
	// create a temporary directory
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	// initial setup
	checkErr(t, setConfig(testConfig, nbictlConfig))
	checkErr(t, setConfig(testConfigForUpdate, nbictlConfig))

	// update the existing context with a new private key
	checkErr(t, setConfig(&nbictlpb.Config{
		Name:    testConfig.GetName(),
		PrivKey: "private_key.updated",
	}, nbictlConfig))

	updatedConfig := proto.Clone(testConfig).(*nbictlpb.Config)
	updatedConfig.PrivKey = "private_key.updated"
	wantContexts := &nbictlpb.AppConfig{
		Configs: []*nbictlpb.Config{updatedConfig, testConfigForUpdate},
	}

	// check if the private key is updated
	gotContexts, err := getConfigs(nbictlConfig)
	checkErr(t, err)
	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetConfig_UpdateKeyId(t *testing.T) {
	t.Parallel()
	// create a temporary directory
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	// initial setup
	checkErr(t, setConfig(testConfig, nbictlConfig))
	checkErr(t, setConfig(testConfigForUpdate, nbictlConfig))

	// update the existing context with a new key id
	checkErr(t, setConfig(&nbictlpb.Config{
		Name:  testConfigForUpdate.GetName(),
		KeyId: "key_id.updated",
	}, nbictlConfig))

	updatedConfig := proto.Clone(testConfigForUpdate).(*nbictlpb.Config)
	updatedConfig.KeyId = "key_id.updated"
	wantContexts := &nbictlpb.AppConfig{
		Configs: []*nbictlpb.Config{testConfig, updatedConfig},
	}

	// check if the key id is updated
	gotContexts, err := getConfigs(nbictlConfig)
	checkErr(t, err)

	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetConfig_UpdateUserID(t *testing.T) {
	t.Parallel()
	// create a temporary directory
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	// initial setup
	checkErr(t, setConfig(testConfig, nbictlConfig))
	checkErr(t, setConfig(testConfigForUpdate, nbictlConfig))

	// update the existing context with a new user id

	checkErr(t, setConfig(&nbictlpb.Config{
		Name:  testConfig.GetName(),
		Email: "email.updated",
	}, nbictlConfig))

	updatedConfig := proto.Clone(testConfig).(*nbictlpb.Config)
	updatedConfig.Email = "email.updated"
	wantContexts := &nbictlpb.AppConfig{
		Configs: []*nbictlpb.Config{updatedConfig, testConfigForUpdate},
	}

	// check if the user id is updated
	gotContexts, err := getConfigs(nbictlConfig)
	checkErr(t, err)

	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetConfig_UpdateUrl(t *testing.T) {
	t.Parallel()
	// create a temporary directory
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	nbictlConfig = filepath.Join(nbictlConfig, confFileName)

	// initial setup
	checkErr(t, setConfig(testConfig, nbictlConfig))
	checkErr(t, setConfig(testConfigForUpdate, nbictlConfig))

	// update the existing context with a new url

	checkErr(t, setConfig(&nbictlpb.Config{
		Name: testConfig.GetName(),
		Url:  "url.updated",
	}, nbictlConfig))

	updatedConfig := proto.Clone(testConfig).(*nbictlpb.Config)
	updatedConfig.Url = "url.updated"
	wantContexts := &nbictlpb.AppConfig{
		Configs: []*nbictlpb.Config{updatedConfig, testConfigForUpdate},
	}

	// check if the url is updated
	gotConfigs, err := getConfigs(nbictlConfig)
	checkErr(t, err)

	assertProtosEqual(t, wantContexts, gotConfigs)
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
