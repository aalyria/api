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

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"aalyria.com/spacetime/tools/nbictl/nbictlpb"
)

func jwtAuthStrategy(email, keyID, privKeyFile string) *nbictlpb.Config_AuthStrategy {
	jwt := &nbictlpb.Config_AuthStrategy_Jwt{
		Email:        email,
		PrivateKeyId: keyID,
	}
	if privKeyFile != "" {
		jwt.SigningStrategy = &nbictlpb.Config_SigningStrategy{
			Type: &nbictlpb.Config_SigningStrategy_PrivateKeyFile{
				PrivateKeyFile: privKeyFile,
			},
		}
	}
	return &nbictlpb.Config_AuthStrategy{
		Type: &nbictlpb.Config_AuthStrategy_Jwt_{Jwt: jwt},
	}
}

var (
	testConfig = &nbictlpb.Config{
		Name:         "unit_testing",
		Url:          "test_url",
		AuthStrategy: jwtAuthStrategy("privateKey.userID", "privateKey.id", "privateKey.path"),
	}
	testConfigForUpdate = &nbictlpb.Config{
		Name:         "test update",
		Url:          "update_url",
		AuthStrategy: jwtAuthStrategy("update_user_id", "update_key_id", "update_priv_key"),
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

	checkErr(t, os.Chmod(confFile, 0o000))

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

	updatedContext := proto.CloneOf(testConfig)
	updatedContext.AuthStrategy = jwtAuthStrategy("privateKey.userID", "privateKey.id", "private_key.updated")
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

	updatedContext := proto.CloneOf(testConfigForUpdate)
	updatedContext.AuthStrategy = jwtAuthStrategy("update_user_id", "key_id.updated", "update_priv_key")
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

	updatedContext := proto.CloneOf(testConfig)
	updatedContext.AuthStrategy = jwtAuthStrategy("email.updated", "privateKey.id", "privateKey.path")
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

	updatedContext := proto.CloneOf(testConfig)
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

// resolveFileArgs returns a copy of args in which any argument naming a file
// written into tmpDir (i.e. a key of fileContents) is rewritten to its absolute
// path under tmpDir. This lets file-based command arguments be resolved without
// changing the process working directory, which is global state and unsafe to
// mutate from t.Parallel() tests. The stdin sentinel "-" is left untouched.
func resolveFileArgs(tmpDir string, fileContents map[string]string, args []string) []string {
	resolved := make([]string, len(args))
	for i, arg := range args {
		if _, ok := fileContents[arg]; ok && arg != "-" {
			resolved[i] = filepath.Join(tmpDir, arg)
		} else {
			resolved[i] = arg
		}
	}
	return resolved
}

func TestSetConfig_MigratesDeprecatedFields(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	legacyConfig := &nbictlpb.Config{
		Name:    "legacy",
		KeyId:   "old_key",
		Email:   "old_email",
		PrivKey: "old_key_path",
		Url:     "old_url",
	}
	checkErr(t, setConfig(io.Discard, io.Discard, legacyConfig, confFile))

	got, err := readConfig("legacy", confFile)
	checkErr(t, err)

	want := &nbictlpb.Config{
		Name:         "legacy",
		Url:          "old_url",
		AuthStrategy: jwtAuthStrategy("old_email", "old_key", "old_key_path"),
	}
	assertProtosEqual(t, want, got)
}

func TestSetConfig_MergesJwtSubFields(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:  "merge_test",
		Email: "user@test",
	}, confFile))
	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:  "merge_test",
		KeyId: "key123",
	}, confFile))
	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:    "merge_test",
		PrivKey: "/path/to/key",
	}, confFile))

	got, err := readConfig("merge_test", confFile)
	checkErr(t, err)

	want := &nbictlpb.Config{
		Name:         "merge_test",
		AuthStrategy: jwtAuthStrategy("user@test", "key123", "/path/to/key"),
	}
	assertProtosEqual(t, want, got)
}

func TestSetConfig_SwitchToNone(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:         "switch_test",
		Url:          "example.com",
		AuthStrategy: jwtAuthStrategy("user@test", "key1", "/path/to/key"),
	}, confFile))

	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name: "switch_test",
		AuthStrategy: &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_None{},
		},
	}, confFile))

	got, err := readConfig("switch_test", confFile)
	checkErr(t, err)

	want := &nbictlpb.Config{
		Name: "switch_test",
		Url:  "example.com",
		AuthStrategy: &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_None{},
		},
	}
	assertProtosEqual(t, want, got)
}

func TestSetConfig_PartialJwtUpdate(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:         "partial_test",
		Url:          "example.com",
		AuthStrategy: jwtAuthStrategy("original@test", "key1", "/path/to/key"),
	}, confFile))

	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:  "partial_test",
		Email: "updated@test",
	}, confFile))

	got, err := readConfig("partial_test", confFile)
	checkErr(t, err)

	want := &nbictlpb.Config{
		Name:         "partial_test",
		Url:          "example.com",
		AuthStrategy: jwtAuthStrategy("updated@test", "key1", "/path/to/key"),
	}
	assertProtosEqual(t, want, got)
}

func assertProtosEqual(t *testing.T, want, got interface{}) {
	t.Helper()

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Fatalf("proto mismatch: (-want +got):\n%s", diff)
	}
}

func TestSetConfig_EndpointConfigSingleDomain(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	config := &nbictlpb.Config{
		Name: "single_domain_test",
		Url:  "api.example.com:443",
		EndpointConfig: &nbictlpb.Config_EndpointConfig{
			Strategy: &nbictlpb.Config_EndpointConfig_SingleDomain{
				SingleDomain: &nbictlpb.Config_ServiceEndpoint{
					Url: "localhost:8080",
					TransportSecurity: &nbictlpb.Config_TransportSecurity{
						Type: &nbictlpb.Config_TransportSecurity_Insecure{},
					},
					AuthStrategy: &nbictlpb.Config_AuthStrategy{
						Type: &nbictlpb.Config_AuthStrategy_None{},
					},
				},
			},
		},
	}
	checkErr(t, setConfig(io.Discard, io.Discard, config, confFile))

	got, err := readConfig("single_domain_test", confFile)
	checkErr(t, err)

	assertProtosEqual(t, config, got)
}

func TestSetConfig_EndpointConfigCustom(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	config := &nbictlpb.Config{
		Name: "custom_test",
		Url:  "default.example.com:443",
		AuthStrategy: &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_Jwt_{
				Jwt: &nbictlpb.Config_AuthStrategy_Jwt{
					Email:        "user@example.com",
					PrivateKeyId: "key-123",
				},
			},
		},
		EndpointConfig: &nbictlpb.Config_EndpointConfig{
			Strategy: &nbictlpb.Config_EndpointConfig_Custom{
				Custom: &nbictlpb.Config_CustomEndpoints{
					Model:  &nbictlpb.Config_ServiceEndpoint{Url: "model.internal:443"},
					Status: &nbictlpb.Config_ServiceEndpoint{Url: "status.internal:443"},
					Provisioning: &nbictlpb.Config_ServiceEndpoint{
						Url: "prov.partner:443",
						AuthStrategy: &nbictlpb.Config_AuthStrategy{
							Type: &nbictlpb.Config_AuthStrategy_None{},
						},
					},
					Default: &nbictlpb.Config_ServiceEndpoint{Url: "fallback.example.com:443"},
				},
			},
		},
	}
	checkErr(t, setConfig(io.Discard, io.Discard, config, confFile))

	got, err := readConfig("custom_test", confFile)
	checkErr(t, err)

	assertProtosEqual(t, config, got)
}

func TestSetConfig_EndpointConfigUpdate(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name: "update_test",
		Url:  "api.example.com:443",
	}, confFile))

	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name: "update_test",
		EndpointConfig: &nbictlpb.Config_EndpointConfig{
			Strategy: &nbictlpb.Config_EndpointConfig_SingleDomain{
				SingleDomain: &nbictlpb.Config_ServiceEndpoint{
					Url: "localhost:8080",
				},
			},
		},
	}, confFile))

	got, err := readConfig("update_test", confFile)
	checkErr(t, err)

	want := &nbictlpb.Config{
		Name: "update_test",
		Url:  "api.example.com:443",
		EndpointConfig: &nbictlpb.Config_EndpointConfig{
			Strategy: &nbictlpb.Config_EndpointConfig_SingleDomain{
				SingleDomain: &nbictlpb.Config_ServiceEndpoint{
					Url: "localhost:8080",
				},
			},
		},
	}
	assertProtosEqual(t, want, got)
}

func oidcUserAuthStrategy(issuer, clientID string) *nbictlpb.Config_AuthStrategy {
	return &nbictlpb.Config_AuthStrategy{
		Type: &nbictlpb.Config_AuthStrategy_OidcUser_{
			OidcUser: &nbictlpb.Config_AuthStrategy_OidcUser{
				Issuer:   issuer,
				ClientId: clientID,
			},
		},
	}
}

func TestSetConfig_OidcUser(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "dev", "config", "set",
		"--auth_strategy", "oidc_user",
		"--issuer", "https://zitadel.example.com",
		"--client_id", "client-1",
		"--url", "nbi.example.com",
	}))

	got, err := readConfig("dev", filepath.Join(tmpDir, confFileName))
	checkErr(t, err)

	assertProtosEqual(t, &nbictlpb.Config{
		Name:         "dev",
		Url:          "nbi.example.com",
		AuthStrategy: oidcUserAuthStrategy("https://zitadel.example.com", "client-1"),
	}, got)
}

// TestSetConfig_OidcUserPartialUpdate makes sure that a second config set
// command keeps the fields that it does not name.
func TestSetConfig_OidcUserPartialUpdate(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "dev", "config", "set",
		"--auth_strategy", "oidc_user",
		"--issuer", "https://zitadel.example.com",
		"--client_id", "client-1",
	}))
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "dev", "config", "set",
		"--auth_strategy", "oidc_user",
		"--client_id", "client-2",
	}))

	got, err := readConfig("dev", filepath.Join(tmpDir, confFileName))
	checkErr(t, err)

	assertProtosEqual(t, &nbictlpb.Config{
		Name:         "dev",
		AuthStrategy: oidcUserAuthStrategy("https://zitadel.example.com", "client-2"),
	}, got)
}

// TestSetConfig_SwitchesFromJwtToOidcUser makes sure that a strategy change
// replaces the strategy instead of merging the two.
func TestSetConfig_SwitchesFromJwtToOidcUser(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(tmpDir, confFileName)

	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:         "dev",
		AuthStrategy: jwtAuthStrategy("someone@example.com", "key-1", "/tmp/key.pem"),
	}, confFile))

	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "dev", "config", "set",
		"--auth_strategy", "oidc_user",
		"--issuer", "https://zitadel.example.com",
		"--client_id", "client-1",
	}))

	got, err := readConfig("dev", confFile)
	checkErr(t, err)

	assertProtosEqual(t, &nbictlpb.Config{
		Name:         "dev",
		AuthStrategy: oidcUserAuthStrategy("https://zitadel.example.com", "client-1"),
	}, got)
}

func TestSetConfig_EndpointConfigNoUpdatePreservesExisting(t *testing.T) {
	t.Parallel()

	confDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	confFile := filepath.Join(confDir, confFileName)

	original := &nbictlpb.Config{
		Name: "preserve_test",
		Url:  "api.example.com:443",
		EndpointConfig: &nbictlpb.Config_EndpointConfig{
			Strategy: &nbictlpb.Config_EndpointConfig_SingleDomain{
				SingleDomain: &nbictlpb.Config_ServiceEndpoint{Url: "localhost:8080"},
			},
		},
	}
	checkErr(t, setConfig(io.Discard, io.Discard, original, confFile))

	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name: "preserve_test",
		Url:  "updated.example.com:443",
	}, confFile))

	got, err := readConfig("preserve_test", confFile)
	checkErr(t, err)

	want := &nbictlpb.Config{
		Name: "preserve_test",
		Url:  "updated.example.com:443",
		EndpointConfig: &nbictlpb.Config_EndpointConfig{
			Strategy: &nbictlpb.Config_EndpointConfig_SingleDomain{
				SingleDomain: &nbictlpb.Config_ServiceEndpoint{Url: "localhost:8080"},
			},
		},
	}
	assertProtosEqual(t, want, got)
}

// TestSetConfig_TightensAnExistingDirectory covers the upgrade path. An
// nbictl from before the permissions change created the config directory with
// mode 0777, and os.MkdirAll leaves an existing directory's mode alone, so
// without an explicit chmod those users keep a world-writable directory with
// the token store inside it.
func TestSetConfig_TightensAnExistingDirectory(t *testing.T) {
	t.Parallel()

	confDir := filepath.Join(t.TempDir(), "nbictl")
	checkErr(t, os.MkdirAll(confDir, 0o777))
	checkErr(t, os.Chmod(confDir, 0o777))

	confFile := filepath.Join(confDir, confFileName)
	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name: "dev",
		Url:  "nbi.example.com",
	}, confFile))

	info, err := os.Stat(confDir)
	checkErr(t, err)
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("the pre-existing config directory is still mode %o, want %o", got, 0o700)
	}
}

// TestSetConfig_RestrictsThePermissions makes sure that the config directory
// and file are not readable by other local users. The token store lives
// inside this directory, so a permissive mode would let another user read a
// refresh token, or rewrite the issuer of a profile.
func TestSetConfig_RestrictsThePermissions(t *testing.T) {
	t.Parallel()

	confFile := filepath.Join(t.TempDir(), "nbictl", confFileName)
	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name: "dev",
		Url:  "nbi.example.com",
	}, confFile))

	fileInfo, err := os.Stat(confFile)
	checkErr(t, err)
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("the config file mode is %o, want %o", got, 0o600)
	}

	dirInfo, err := os.Stat(filepath.Dir(confFile))
	checkErr(t, err)
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("the config directory mode is %o, want %o", got, 0o700)
	}
}

func oidcServiceAccountAuthStrategy(issuer, projectID, keyFile string) *nbictlpb.Config_AuthStrategy {
	return &nbictlpb.Config_AuthStrategy{
		Type: &nbictlpb.Config_AuthStrategy_OidcServiceAccount_{
			OidcServiceAccount: &nbictlpb.Config_AuthStrategy_OidcServiceAccount{
				Issuer:                issuer,
				ProjectId:             projectID,
				ServiceAccountKeyFile: keyFile,
			},
		},
	}
}

// TestSetConfig_OidcServiceAccount verifies that the three flags are stored
// in the correct proto fields.
func TestSetConfig_OidcServiceAccount(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "robot", "config", "set",
		"--auth_strategy", "oidc_service_account",
		"--issuer", "https://zitadel.example.com",
		"--project_id", "test-project",
		"--service_account_key_file", "/path/to/zitadel-key.json",
	}))

	got, err := readConfig("robot", filepath.Join(tmpDir, confFileName))
	checkErr(t, err)

	assertProtosEqual(t, &nbictlpb.Config{
		Name:         "robot",
		AuthStrategy: oidcServiceAccountAuthStrategy("https://zitadel.example.com", "test-project", "/path/to/zitadel-key.json"),
	}, got)
}

// TestSetConfig_OidcServiceAccountPartialUpdate is deletion test 13: a second
// config set with only --project_id must leave issuer and key file intact. The
// default: arm at mergeAuthStrategy replaces the whole strategy and makes this
// test fail when the OidcServiceAccount_ case is absent.
func TestSetConfig_OidcServiceAccountPartialUpdate(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "robot", "config", "set",
		"--auth_strategy", "oidc_service_account",
		"--issuer", "https://zitadel.example.com",
		"--project_id", "project-1",
		"--service_account_key_file", "/path/to/key.json",
	}))
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "robot", "config", "set",
		"--auth_strategy", "oidc_service_account",
		"--project_id", "project-2",
	}))

	got, err := readConfig("robot", filepath.Join(tmpDir, confFileName))
	checkErr(t, err)

	assertProtosEqual(t, &nbictlpb.Config{
		Name:         "robot",
		AuthStrategy: oidcServiceAccountAuthStrategy("https://zitadel.example.com", "project-2", "/path/to/key.json"),
	}, got)
}

// TestSetConfig_OidcServiceAccountUnknownStrategy verifies that the allowed
// list in the error message names oidc_service_account.
func TestSetConfig_OidcServiceAccountUnknownStrategy(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	err = newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "dev", "config", "set",
		"--auth_strategy", "nonsense",
	})
	if err == nil {
		t.Fatal("expected an error for an unknown auth strategy, got nil")
	}
	if !strings.Contains(err.Error(), "oidc_service_account") {
		t.Errorf("error %q does not mention oidc_service_account in the allowed list", err.Error())
	}
}

// TestSetConfig_OidcServiceAccountRestrictsPermissions is deletion test 12:
// after a config set with the new strategy, the directory must be 0700 and
// the file must be 0600.
func TestSetConfig_OidcServiceAccountRestrictsPermissions(t *testing.T) {
	t.Parallel()

	confFile := filepath.Join(t.TempDir(), "nbictl", confFileName)
	checkErr(t, setConfig(io.Discard, io.Discard, &nbictlpb.Config{
		Name:         "robot",
		AuthStrategy: oidcServiceAccountAuthStrategy("https://zitadel.example.com", "proj-1", "/tmp/key.json"),
	}, confFile))

	fileInfo, err := os.Stat(confFile)
	checkErr(t, err)
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("config file mode is %o, want %o", got, 0o600)
	}

	dirInfo, err := os.Stat(filepath.Dir(confFile))
	checkErr(t, err)
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("config directory mode is %o, want %o", got, 0o700)
	}
}

// TestSetConfig_OidcServiceAccountRefusesAPartialSwitch pins the one case the
// per-field merge gets wrong. A profile that already names another strategy
// has no service account fields to merge with, so a switch that omits
// --service_account_key_file would keep whatever key file the profile held. nbictl
// would then sign an assertion with that key and disclose it to a different
// identity provider.
func TestSetConfig_OidcServiceAccountRefusesAPartialSwitch(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "robot", "config", "set",
		"--auth_strategy", "oidc_service_account",
		"--issuer", "https://prod.example.com",
		"--project_id", "prod-project",
		"--service_account_key_file", "/path/to/prod-key.json",
	}))
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "robot", "config", "set",
		"--auth_strategy", "oidc_user",
		"--issuer", "https://other.example.com",
		"--client_id", "the-client",
	}))

	err = newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "robot", "config", "set",
		"--auth_strategy", "oidc_service_account",
		"--issuer", "https://other.example.com",
		"--project_id", "other-project",
	})
	if err == nil {
		t.Fatal("config set switched to oidc_service_account without a service account key")
	}
	if !strings.Contains(err.Error(), "service_account_key_file") {
		t.Errorf("error %q does not name the missing flag", err.Error())
	}
}

// TestSetConfig_OidcServiceAccountRefusesAnEmptyStrategy pins a new profile.
// Without the three flags the strategy holds nothing, and the profile fails
// later at dial time rather than here, where the operator can act on it.
func TestSetConfig_OidcServiceAccountRefusesAnEmptyStrategy(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	err = newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "robot", "config", "set",
		"--auth_strategy", "oidc_service_account",
	})
	if err == nil {
		t.Fatal("config set stored an oidc_service_account strategy with no fields")
	}
	for _, flag := range []string{"issuer", "project_id", "service_account_key_file"} {
		if !strings.Contains(err.Error(), flag) {
			t.Errorf("error %q does not name the missing flag %q", err.Error(), flag)
		}
	}
}

// TestSetConfig_UrlUpdateIgnoresTheStoredAuthStrategy pins the scope of the
// completeness check. A command that names no auth strategy must not be judged
// on the one the profile already holds, or an unrelated --url update fails and
// blames a flag the operator never gave.
func TestSetConfig_UrlUpdateIgnoresTheStoredAuthStrategy(t *testing.T) {
	t.Parallel()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	// Written directly, because setConfig itself now refuses to store a
	// profile in this state. A hand-edited config can still hold one.
	confFile := filepath.Join(tmpDir, confFileName)
	stored, err := prototext.Marshal(&nbictlpb.AppConfig{Configs: []*nbictlpb.Config{{
		Name: "robot",
		Url:  "old.example.com",
		AuthStrategy: &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_OidcServiceAccount_{
				OidcServiceAccount: &nbictlpb.Config_AuthStrategy_OidcServiceAccount{
					Issuer:    "https://zitadel.example.com",
					ProjectId: "project-1",
					// No key file: a profile an earlier build could store.
				},
			},
		},
	}}})
	checkErr(t, err)
	checkErr(t, os.WriteFile(confFile, stored, 0o600))

	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "robot", "config", "set",
		"--url", "new.example.com",
	}))

	got, err := readConfig("robot", confFile)
	checkErr(t, err)
	if got.GetUrl() != "new.example.com" {
		t.Errorf("url = %q, want new.example.com", got.GetUrl())
	}
}
