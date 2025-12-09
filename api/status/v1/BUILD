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

load("@protobuf//bazel:proto_library.bzl", "proto_library")
load("@rules_go//proto:def.bzl", "go_proto_library")
load("@rules_proto_grpc_python//:defs.bzl", "python_grpc_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "status_proto",
    srcs = [
        "status.proto",
    ],
    deps = [
        "@opentelemetry_metrics//:metrics_proto",
    ],
)

go_proto_library(
    name = "status_go_grpc",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/status/v1",
    proto = ":status_proto",
    deps = [
        "@io_opentelemetry_go_proto_otlp//metrics/v1:metrics",
    ],
)

python_grpc_library(
    name = "status_python_grpc",
    protos = [":status_proto"],
)
