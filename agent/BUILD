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

# Build rules for the Go SBI agent. These shouldn't depend on any other
# internal packages apart from those found in //api.

load("@rules_go//go:def.bzl", "go_library", "go_test")

package(default_visibility = ["//visibility:public"])

go_library(
    name = "agent",
    srcs = [
        "agent.go",
        "enactment_service.go",
        "node_controller.go",
        "telemetry_service.go",
        "timing.go",
    ],
    importpath = "aalyria.com/spacetime/agent",
    deps = [
        "//agent/enactment",
        "//agent/internal/channels",
        "//agent/internal/loggable",
        "//agent/internal/task",
        "//agent/telemetry",
        "//api/common:common_go_proto",
        "//api/scheduling/v1alpha:scheduling_go_grpc",
        "//api/telemetry/v1alpha:telemetry_go_grpc",
        "@com_github_google_uuid//:uuid",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_rs_zerolog//:zerolog",
        "@io_opentelemetry_go_otel//attribute",
        "@org_golang_google_grpc//:grpc",
        "@org_golang_google_grpc//codes",
        "@org_golang_google_grpc//status",
        "@org_golang_google_protobuf//proto",
        "@org_golang_x_sync//errgroup",
    ],
)

go_test(
    name = "agent_test",
    size = "small",
    srcs = [
        "agent_test.go",
        "common_test.go",
        "enactment_test.go",
        "telemetry_test.go",
        "timing_test.go",
    ],
    embed = [":agent"],
    deps = [
        "//agent/internal/channels",
        "//agent/internal/task",
        "//api/common:common_go_proto",
        "//api/scheduling/v1alpha:scheduling_go_grpc",
        "//api/telemetry/v1alpha:telemetry_go_grpc",
        "@com_github_google_go_cmp//cmp",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_rs_zerolog//:zerolog",
        "@org_golang_google_grpc//:grpc",
        "@org_golang_google_grpc//credentials/insecure",
        "@org_golang_google_grpc//status",
        "@org_golang_google_protobuf//encoding/prototext",
        "@org_golang_google_protobuf//proto",
        "@org_golang_google_protobuf//testing/protocmp",
        "@org_golang_google_protobuf//types/known/emptypb",
        "@org_golang_google_protobuf//types/known/timestamppb",
        "@org_golang_x_sync//errgroup",
    ],
)
