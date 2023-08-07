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

load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "nbictl",
    srcs = [
        "connection.go",
        "generate_rsa_key.go",
        "nbictl.go",
        "set_context.go",
    ],
    importpath = "aalyria.com/spacetime/github/tools/nbictl",
    visibility = ["//visibility:public"],
    deps = [
        "//api/nbi/v1alpha:nbi_go_grpc",
        "//cdpi_agent/internal/auth",
        "//tools/nbictl/proto:nbictl_go_proto",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//credentials",
        "@org_golang_google_grpc//credentials/insecure",
        "@org_golang_google_protobuf//encoding/prototext",
    ],
)

go_test(
    name = "nbictl_test",
    srcs = [
        "connection_test.go",
        "fake_nbi_server_test.go",
        "generate_rsa_key_test.go",
        "set_context_test.go",
    ],
    embed = [":nbictl"],
    deps = [
        "//api/nbi/v1alpha:nbi_go_grpc",
        "//tools/nbictl/proto:nbictl_go_proto",
        "@com_github_google_go_cmp//cmp",
        "@io_bazel_rules_go//go/tools/bazel:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//metadata",
        "@org_golang_google_protobuf//proto",
        "@org_golang_google_protobuf//testing/protocmp",
        "@org_golang_x_sync//errgroup",
    ],
)
