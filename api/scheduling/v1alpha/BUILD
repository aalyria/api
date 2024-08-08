# Copyright 2024 Aalyria Technologies, Inc., and its affiliates.
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

load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")
load("@rules_proto//proto:defs.bzl", "proto_library")
load("@rules_proto_grpc//cpp:defs.bzl", "cpp_grpc_library")
load("@rules_proto_grpc//java:defs.bzl", "java_grpc_library")
load("@rules_proto_grpc//python:defs.bzl", "python_grpc_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "scheduling_proto",
    srcs = [
        "scheduling.proto",
    ],
    deps = [
        "@com_google_googleapis//google/rpc:status_proto",
        "@com_google_protobuf//:empty_proto",
        "@com_google_protobuf//:timestamp_proto",
    ],
)

cpp_grpc_library(
    name = "scheduling_cpp_grpc",
    protos = [":scheduling_proto"],
    deps = [
        "@com_google_googleapis//google/rpc:status_cc_proto",
    ],
)

go_proto_library(
    name = "scheduling_go_grpc",
    compilers = [
        "@io_bazel_rules_go//proto:go_proto",
        "@io_bazel_rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/scheduling/v1alpha",
    proto = ":scheduling_proto",
    deps = [
        "@org_golang_google_genproto_googleapis_rpc//status",
    ],
)

java_grpc_library(
    name = "scheduling_java_grpc",
    protos = [":scheduling_proto"],
    deps = [
        "@com_google_googleapis//google/rpc:rpc_java_proto",
    ],
)

python_grpc_library(
    name = "scheduling_python_grpc",
    protos = [":scheduling_proto"],
)
