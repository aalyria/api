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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aalyria.com/spacetime/tools/nbictl/nbictlpb"
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
	}
	testConfigForUpdate = &nbictlpb.Config{
		Name:    "test update",
		KeyId:   "update_key_id",
		Email:   "update_user_id",
		PrivKey: "update_priv_key",
		Url:     "update_url",
	}
	testConfigs = &nbictlpb.AppConfig{
		Configs: []*nbictlpb.Config{testConfig},
	}
)

func TestReadConfig(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	checkErr(t, setConfig(io.Discard, io.Discard, testConfig, confFile))

	got, err := readConfig(testConfig.GetName(), confFile)
	checkErr(t, err)

	assertProtosEqual(t, testConfig, got)
}

func TestReadConfig_WithOnlyOneProfileInConfig(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	checkErr(t, setConfig(io.Discard, io.Discard, testConfig, confFile))

	got, err := readConfig("", confFile)
	checkErr(t, err)

	assertProtosEqual(t, testConfig, got)
}

func TestReadConfig_WithNoProfileNameWithMultipleProfilesInConfig(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)
	for _, contextToCreate := range []string{"test_1", "test_2", "test_3"} {
		checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{Name: contextToCreate}, confFile))
	}

	_, err = readConfig("", confFile)
	want := "--profile flag required because there are multiple profiles defined in the configuration"
	if got := err.Error(); !strings.Contains(got, want) {
		t.Fatalf("want: %s, got %s", want, got)
	}
}

func TestReadConfig_WithFileWithNoPermission(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)
	file, err := os.Create(confFile)
	checkErr(t, err)
	defer file.Close()

	checkErr(t, os.Chmod(confFile, 0000))

	if _, err = readConfigs(confFile); err == nil {
		t.Fatal("unable to detect that issues with selected config file")
	}
}

func TestReadConfig_WithNonExistingProfileName(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)
	for _, contextToCreate := range []string{"test_1", "test_2", "test_3"} {
		checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{Name: contextToCreate}, confFile))
	}

	_, err = readConfig("non_existing", confFile)
	wantErrMsg := `unable to get the profile with the name: "non_existing" (expected one of [test_1, test_2, test_3])`
	if gotErrMsg := err.Error(); !strings.Contains(gotErrMsg, wantErrMsg) {
		t.Fatalf("want: %s, got %s", wantErrMsg, gotErrMsg)
	}
}

func TestReadConfig_WithProfileNameMatchingOneProfileByURL(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)
	for _, str := range []string{"1", "2", "3"} {
		checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{Name: "test_" + str, Url: str + ".test.example"}, confFile))
	}

	got, err := readConfig("2.test.example", confFile)
	checkErr(t, err)
	want, err := readConfig("test_2", confFile)
	checkErr(t, err)
	assertProtosEqual(t, want, got)
}

func TestReadConfig_WithProfileNameMatchingManyProfilesByURL(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)
	for _, str := range []string{"1", "2", "3"} {
		checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{Name: "test_" + str, Url: "same.test.example"}, confFile))
	}
	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{Name: "test_4", Url: "other.test.example"}, confFile))

	_, err = readConfig("same.test.example", confFile)
	wantErrMsg := `unable to get the profile with the name: "same.test.example" (expected one of [test_1, test_2, test_3, test_4]); ` +
		`additionally, profile match by URL found multiple matches (3): [test_1, test_2, test_3]`
	if gotErrMsg := err.Error(); !strings.Contains(gotErrMsg, wantErrMsg) {
		t.Fatalf("want: %s, got %s", wantErrMsg, gotErrMsg)
	}
}

func TestSetConfig_WithNoUpdate(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	checkErr(t, setConfig(io.Discard, io.Discard, testConfig, confFile))
	wantContexts, err := readConfigs(confFile)
	checkErr(t, err)
	assertProtosEqual(t, testConfigs, wantContexts)

	contextWithNoChange := &nbictlpb.Config{Name: testConfig.GetName()}
	checkErr(t, setConfig(io.Discard, io.Discard, contextWithNoChange, confFile))

	gotContexts, err := readConfigs(confFile)
	checkErr(t, err)
	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetConfig_UpdatePrivateKey(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	// initial setup
	checkErr(t, setConfig(io.Discard, io.Discard, testConfig, confFile))
	checkErr(t, setConfig(io.Discard, io.Discard, testConfigForUpdate, confFile))

	// update the existing context with a new private key
	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:    testConfig.GetName(),
		PrivKey: "private_key.updated",
	}, confFile))
	gotContexts, err := readConfigs(confFile)
	checkErr(t, err)

	// check that the private key is updated
	updatedContext := proto.Clone(testConfig).(*nbictlpb.Config)
	updatedContext.PrivKey = "private_key.updated"
	wantContexts := &nbictlpb.AppConfig{
		Configs: []*nbictlpb.Config{updatedContext, testConfigForUpdate},
	}
	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetConfig_UpdateKeyId(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	// initial setup
	checkErr(t, setConfig(io.Discard, io.Discard, testConfig, confFile))
	checkErr(t, setConfig(io.Discard, io.Discard, testConfigForUpdate, confFile))

	// update the existing context with a new key id
	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:  testConfigForUpdate.GetName(),
		KeyId: "key_id.updated",
	}, confFile))
	gotContexts, err := readConfigs(confFile)
	checkErr(t, err)

	// check that the key id is updated
	updatedContext := proto.Clone(testConfigForUpdate).(*nbictlpb.Config)
	updatedContext.KeyId = "key_id.updated"
	wantContexts := &nbictlpb.AppConfig{
		Configs: []*nbictlpb.Config{testConfig, updatedContext},
	}
	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetConfig_UpdateUserID(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	// initial setup
	checkErr(t, setConfig(io.Discard, io.Discard, testConfig, confFile))
	checkErr(t, setConfig(io.Discard, io.Discard, testConfigForUpdate, confFile))

	// update the existing context with a new user id
	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:  testConfig.GetName(),
		Email: "email.updated",
	}, confFile))
	gotContexts, err := readConfigs(confFile)
	checkErr(t, err)

	// check that the user id is updated
	updatedContext := proto.Clone(testConfig).(*nbictlpb.Config)
	updatedContext.Email = "email.updated"
	wantContexts := &nbictlpb.AppConfig{
		Configs: []*nbictlpb.Config{updatedContext, testConfigForUpdate},
	}
	assertProtosEqual(t, wantContexts, gotContexts)
}

func TestSetConfig_UpdateUrl(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	// initial setup
	checkErr(t, setConfig(io.Discard, io.Discard, testConfig, confFile))
	checkErr(t, setConfig(io.Discard, io.Discard, testConfigForUpdate, confFile))
	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name: testConfig.GetName(),
		Url:  "url.updated",
	}, confFile))
	gotContexts, err := readConfigs(confFile)
	checkErr(t, err)

	// check that the url is updated
	updatedContext := proto.Clone(testConfig).(*nbictlpb.Config)
	updatedContext.Url = "url.updated"
	wantContexts := &nbictlpb.AppConfig{
		Configs: []*nbictlpb.Config{updatedContext, testConfigForUpdate},
	}
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
		t.Fatalf("proto mismatch: (-want +got):\n%s", diff)
	}
}
