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
load("@rules_proto_grpc_python//:defs.bzl", "python_proto_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "common_proto",
    srcs = ["opentelemetry/proto/common/v1/common.proto"],
    visibility = ["//visibility:public"],
)

python_proto_library(
    name = "common_python_proto",
    protos = [":common_proto"],
)

go_proto_library(
    name = "common_go_proto",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "go.opentelemetry.io/proto/otlp/common/v1",
    proto = ":common_proto",
)

proto_library(
    name = "resource_proto",
    srcs = ["opentelemetry/proto/resource/v1/resource.proto"],
    visibility = ["//visibility:public"],
    deps = [":common_proto"],
)

python_proto_library(
    name = "resource_python_proto",
    protos = [":resource_proto"],
    deps = [":common_python_proto"],
)

go_proto_library(
    name = "resource_go_proto",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "go.opentelemetry.io/proto/otlp/resource/v1",
    proto = ":resource_proto",
)

proto_library(
    name = "metrics_proto",
    srcs = ["opentelemetry/proto/metrics/v1/metrics.proto"],
    visibility = ["//visibility:public"],
    deps = [
        ":common_proto",
        ":resource_proto",
    ],
)

python_proto_library(
    name = "metrics_python_proto",
    protos = [":metrics_proto"],
    deps = [
        ":common_python_proto",
        ":resource_python_proto",
    ],
)

go_proto_library(
    name = "metrics_go_proto",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "go.opentelemetry.io/proto/otlp/metrics/v1",
    proto = ":metrics_proto",
)
