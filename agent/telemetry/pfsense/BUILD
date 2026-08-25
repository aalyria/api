# Copyright (c) Aalyria Technologies, Inc., and its affiliates.
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

load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "pfsense",
    srcs = ["pfsense.go"],
    importpath = "aalyria.com/spacetime/agent/telemetry/pfsense",
    visibility = ["//visibility:public"],
    deps = [
        "//agent/internal/pfsense",
        "//agent/internal/pfsense/impl",
        "//api/common:common_go_proto",
        "//api/telemetry/v1alpha:telemetry_go_grpc",
        "@com_github_rs_zerolog//:zerolog",
        "@org_golang_google_protobuf//encoding/prototext",
        "@org_golang_google_protobuf//proto",
        "@org_golang_google_protobuf//types/known/timestamppb",
    ],
)

go_test(
    name = "pfsense_test",
    srcs = ["pfsense_test.go"],
    embed = [":pfsense"],
    deps = [
        "//agent/internal/pfsense",
        "//agent/internal/pfsense/pfsensemock",
        "//api/common:common_go_proto",
        "//api/telemetry/v1alpha:telemetry_go_grpc",
        "@org_golang_google_protobuf//encoding/prototext",
    ],
)
