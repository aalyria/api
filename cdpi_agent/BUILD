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

package(default_visibility = ["//cdpi_agent:__subpackages__"])

go_library(
    name = "agent",
    srcs = [
        "agent.go",
        "enactment_service.go",
        "node_controller.go",
        "telemetry_service.go",
        "timing.go",
    ],
    importpath = "aalyria.com/spacetime/cdpi_agent/agent",
    deps = [
        "//api/cdpi/v1alpha:cdpi_go_grpc",
        "//api/common:common_go_proto",
        "//cdpi_agent/enactment",
        "//cdpi_agent/internal/channels",
        "//cdpi_agent/internal/loggable",
        "//cdpi_agent/internal/task",
        "//cdpi_agent/telemetry",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_rs_zerolog//:zerolog",
        "@io_opentelemetry_go_otel//attribute",
        "@io_opentelemetry_go_otel_trace//:trace",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//codes",
        "@org_golang_google_grpc//status",
        "@org_golang_google_protobuf//encoding/protojson",
        "@org_golang_google_protobuf//proto",
        "@org_golang_google_protobuf//types/known/timestamppb",
        "@org_golang_x_sync//errgroup",
    ],
)

go_test(
    name = "agent_enactment_test",
    size = "small",
    srcs = ["agent_enactment_test.go"],
    embed = [":agent"],
    deps = [
        "//api/cdpi/v1alpha:cdpi_go_grpc",
        "//api/common:common_go_proto",
        "@com_github_google_go_cmp//cmp",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_rs_zerolog//:zerolog",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//codes",
        "@org_golang_google_grpc//credentials/insecure",
        "@org_golang_google_grpc//status",
        "@org_golang_google_protobuf//proto",
        "@org_golang_google_protobuf//testing/protocmp",
        "@org_golang_google_protobuf//types/known/timestamppb",
        "@org_golang_x_sync//errgroup",
    ],
)

go_test(
    name = "agent_telemetry_test",
    size = "small",
    srcs = ["agent_telemetry_test.go"],
    embed = [":agent"],
    deps = [
        "//api/cdpi/v1alpha:cdpi_go_grpc",
        "//api/common:common_go_proto",
        "@com_github_google_go_cmp//cmp",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_rs_zerolog//:zerolog",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//codes",
        "@org_golang_google_grpc//credentials/insecure",
        "@org_golang_google_grpc//status",
        "@org_golang_google_protobuf//proto",
        "@org_golang_google_protobuf//testing/protocmp",
        "@org_golang_google_protobuf//types/known/timestamppb",
        "@org_golang_x_sync//errgroup",
    ],
)

go_test(
    name = "timing_test",
    size = "small",
    srcs = ["timing_test.go"],
    embed = [":agent"],
    deps = [
        "@com_github_jonboulle_clockwork//:clockwork",
    ],
)
