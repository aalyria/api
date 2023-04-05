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

load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_binary(
    name = "extproc_agent",
    embed = [":extproc_agent_lib"],
    visibility = ["//visibility:public"],
)

go_library(
    name = "extproc_agent_lib",
    srcs = [
        "extproc_agent.go",
        "flag.go",
    ],
    importpath = "aalyria.com/spacetime/cdpi_agent/cmd/extproc_agent",
    visibility = ["//visibility:private"],
    deps = [
        "//api/cdpi/v1alpha:cdpi_go_grpc",
        "//cdpi_agent",
        "//cdpi_agent/enactment/extproc",
        "//cdpi_agent/internal/task",
        "//cdpi_agent/telemetry/extproc",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_rs_zerolog//:zerolog",
        "@io_opentelemetry_go_contrib_instrumentation_google_golang_org_grpc_otelgrpc//:otelgrpc",
        "@io_opentelemetry_go_contrib_instrumentation_net_http_otelhttp//:otelhttp",
        "@io_opentelemetry_go_otel//attribute",
        "@io_opentelemetry_go_otel//propagation",
        "@io_opentelemetry_go_otel//semconv/v1.17.0:v1_17_0",
        "@io_opentelemetry_go_otel_exporters_zipkin//:zipkin",
        "@io_opentelemetry_go_otel_sdk//resource",
        "@io_opentelemetry_go_otel_sdk//trace",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//backoff",
        "@org_golang_google_grpc//channelz/service",
        "@org_golang_google_grpc//codes",
        "@org_golang_google_grpc//credentials",
        "@org_golang_google_grpc//credentials/insecure",
        "@org_golang_google_grpc//status",
        "@org_golang_google_protobuf//proto",
    ],
)
