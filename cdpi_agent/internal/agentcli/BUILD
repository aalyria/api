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

load("@io_bazel_rules_go//go:def.bzl", "go_library")

package(default_visibility = ["//visibility:public"])

go_library(
    name = "agentcli_lib",
    srcs = [
        "agentcli.go",
        "netlink_linux.go",
        "netlink_other.go",
    ],
    importpath = "aalyria.com/spacetime/cdpi_agent/internal/agentcli",
    deps = [
        "//auth",
        "//cdpi_agent",
        "//cdpi_agent/enactment",
        "//cdpi_agent/enactment/extproc",
        "//cdpi_agent/internal/configpb:configpb_go_proto",
        "//cdpi_agent/internal/protofmt",
        "//cdpi_agent/internal/task",
        "//cdpi_agent/telemetry",
        "//cdpi_agent/telemetry/extproc",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_rs_zerolog//:zerolog",
        "@io_opentelemetry_go_contrib_instrumentation_google_golang_org_grpc_otelgrpc//:otelgrpc",
        "@io_opentelemetry_go_otel//attribute",
        "@io_opentelemetry_go_otel//propagation",
        "@io_opentelemetry_go_otel//semconv/v1.17.0:v1_17_0",
        "@io_opentelemetry_go_otel_exporters_otlp_otlptrace_otlptracegrpc//:otlptracegrpc",
        "@io_opentelemetry_go_otel_sdk//resource",
        "@io_opentelemetry_go_otel_sdk//trace",
        "@io_opentelemetry_go_otel_trace//:trace",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//backoff",
        "@org_golang_google_grpc//channelz/service",
        "@org_golang_google_grpc//credentials",
        "@org_golang_google_grpc//credentials/insecure",
        "@org_golang_google_protobuf//types/known/anypb",
        "@org_golang_x_sync//errgroup",
    ] + select({
        "@io_bazel_rules_go//go/platform:android": [
            "//cdpi_agent/enactment/netlink",
            "//cdpi_agent/telemetry/netlink",
            "@com_github_vishvananda_netlink//:netlink",
        ],
        "@io_bazel_rules_go//go/platform:linux": [
            "//cdpi_agent/enactment/netlink",
            "//cdpi_agent/telemetry/netlink",
            "@com_github_vishvananda_netlink//:netlink",
        ],
        "//conditions:default": [],
    }),
)
