# Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Build rules for the Go CDPI agent. These shouldn't depend on any other
# internal packages apart from those found in //api.

load("@rules_go//go:def.bzl", "go_library", "go_test")
load("//:version.bzl", "VERSION")

go_library(
    name = "nbictl",
    srcs = [
        "config.go",
        "connection.go",
        "fs.go",
        "generate_auth_token.go",
        "generate_rsa_key.go",
        "grpcurl.go",
        "model.go",
        "nbictl.go",
        "provisioning.go",
    ],
    importpath = "aalyria.com/spacetime/tools/nbictl",
    visibility = ["//visibility:public"],
    x_defs = {
        "Version": VERSION,
    },
    deps = [
        "//api/common:common_go_proto",
        "//api/federation/interconnect/v1alpha:interconnect_go_proto",
        "//api/model/v1:v1_go_proto",
        "//api/nbi/v1alpha:nbi_go_grpc",
        "//api/nbi/v1alpha/resources:nbi_resources_go_grpc",
        "//api/provisioning/v1alpha:provisioning_go_grpc",
        "//api/scheduling/v1alpha:scheduling_go_grpc",
        "//api/telemetry/v1alpha:telemetry_go_grpc",
        "//auth",
        "//tools/nbictl/proto:nbictl_go_proto",
        "//tools/nbictl/proto:provisioning_go_proto",
        "@com_github_deckarep_golang_set_v2//:golang-set",
        "@com_github_fullstorydev_grpcurl//:grpcurl",
        "@com_github_golang_jwt_jwt_v5//:jwt",
        "@com_github_jhump_protoreflect//desc",
        "@com_github_jhump_protoreflect//grpcreflect",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_samber_lo//:lo",
        "@com_github_urfave_cli_v2//:cli",
        "@org_golang_google_genproto//googleapis/type/interval",
        "@org_golang_google_grpc//:grpc",
        "@org_golang_google_grpc//codes",
        "@org_golang_google_grpc//credentials",
        "@org_golang_google_grpc//credentials/insecure",
        "@org_golang_google_grpc//encoding/gzip",
        "@org_golang_google_grpc//status",
        "@org_golang_google_protobuf//encoding/protojson",
        "@org_golang_google_protobuf//encoding/prototext",
        "@org_golang_google_protobuf//proto",
        "@org_golang_google_protobuf//reflect/protoreflect",
        "@org_golang_google_protobuf//types/descriptorpb",
        "@org_golang_google_protobuf//types/known/durationpb",
        "@org_golang_google_protobuf//types/known/timestamppb",
        "@org_golang_x_sync//errgroup",
        "@org_outernetcouncil_nmts//v1/lib/entityrelationship",
        "@org_outernetcouncil_nmts//v1/proto:nmts_go_proto",
    ],
)

go_test(
    name = "nbictl_test",
    srcs = [
        "config_test.go",
        "connection_test.go",
        "fake_modelapi_server_test.go",
        "fake_nbi_server_test.go",
        "generate_auth_token_test.go",
        "generate_rsa_key_test.go",
        "model_test.go",
        "nbictl_test.go",
    ],
    embed = [":nbictl"],
    deps = [
        "//api/common:common_go_proto",
        "//api/model/v1:v1_go_proto",
        "//api/nbi/v1alpha:nbi_go_grpc",
        "//api/nbi/v1alpha/resources:nbi_resources_go_grpc",
        "//auth/authtest",
        "//tools/nbictl/proto:nbictl_go_proto",
        "@com_github_golang_jwt_jwt_v5//:jwt",
        "@com_github_google_go_cmp//cmp",
        "@com_github_urfave_cli_v2//:cli",
        "@org_golang_google_grpc//:grpc",
        "@org_golang_google_grpc//metadata",
        "@org_golang_google_grpc//reflection",
        "@org_golang_google_protobuf//encoding/prototext",
        "@org_golang_google_protobuf//proto",
        "@org_golang_google_protobuf//testing/protocmp",
        "@org_golang_google_protobuf//types/known/emptypb",
        "@org_golang_x_sync//errgroup",
        "@org_outernetcouncil_nmts//v1/proto:nmts_go_proto",
        "@org_outernetcouncil_nmts//v1/proto/ek/physical:physical_go_proto",
        "@rules_go//go/tools/bazel",
    ],
)

# Check that the README.md file matches the output of the nbictl tool's
# "readme" subcommand exactly. Normally the README file would be a `genfile`
# rule, but for ease of browsing documentation we want it to be checked in.
sh_test(
    name = "readme_test",
    size = "small",
    srcs = ["readme_test.sh"],
    data = [
        ":README.md",
        "//tools/nbictl/cmd/nbictl",
    ],
)
