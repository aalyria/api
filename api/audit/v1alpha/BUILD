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
load("@rules_proto_grpc_cpp//:defs.bzl", "cpp_grpc_library")
load("@rules_proto_grpc_java//:defs.bzl", "java_grpc_library")
load("@rules_proto_grpc_python//:defs.bzl", "python_grpc_library")

package(default_visibility = ["//visibility:public"])

cpp_grpc_library(
    name = "audit_cpp_grpc",
    protos = [":audit_proto"],
    deps = [
        "@googleapis//google/api:annotations_cc_proto",
        "@googleapis//google/api:field_behavior_cc_proto",
        "@googleapis//google/rpc:status_cc_proto",
    ],
)

go_proto_library(
    name = "audit_go_grpc",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/audit/v1alpha",
    proto = ":audit_proto",
    deps = [
        "@org_golang_google_genproto_googleapis_api//annotations",
        "@org_golang_google_genproto_googleapis_rpc//status",
    ],
)

java_grpc_library(
    name = "audit_java_grpc",
    protos = [":audit_proto"],
    deps = [
        "@googleapis//google/api:api_java_proto",
        "@googleapis//google/rpc:rpc_java_proto",
        "@googleapis//google/type:type_java_proto",
    ],
)

python_grpc_library(
    name = "audit_python_grpc",
    protos = [":audit_proto"],
)

proto_library(
    name = "audit_proto",
    srcs = ["audit_logs.proto"],
    deps = [
        "@googleapis//google/api:field_behavior_proto",
        "@googleapis//google/rpc:status_proto",
        "@protobuf//:struct_proto",
        "@protobuf//:timestamp_proto",
    ],
)
