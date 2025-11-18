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

load("@protobuf//bazel:proto_library.bzl", "proto_library")
load("@rules_go//proto:def.bzl", "go_proto_library")
load("@rules_proto_grpc_cpp//:defs.bzl", "cpp_grpc_library")
load("@rules_proto_grpc_java//:defs.bzl", "java_grpc_library")
load("@rules_proto_grpc_python//:defs.bzl", "python_grpc_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "nbi_proto",
    srcs = [
        "nbi.proto",
        "signal_propagation.proto",
        "txtpb_entities.proto",
    ],
    deps = [
        "//api/common:common_proto",
        "//api/nbi/v1alpha/resources:resources_proto",
        "@googleapis//google/type:interval_proto",
        "@protobuf//:duration_proto",
        "@protobuf//:timestamp_proto",
    ],
)

cpp_grpc_library(
    name = "nbi_cpp_grpc",
    generate_mocks = True,
    protos = [":nbi_proto"],
    deps = [
        "//api/common:common_cpp_proto",
        "//api/nbi/v1alpha/resources:nbi_resources_cpp_grpc",
        "@googleapis//google/rpc:code_cc_proto",
        "@googleapis//google/type:interval_cc_proto",
    ],
)

java_grpc_library(
    name = "nbi_java_grpc",
    protos = [":nbi_proto"],
    deps = [
        "//api/common:common_java_proto",
        "//api/nbi/v1alpha/resources:nbi_resources_java_grpc",
        "@googleapis//google/api:api_java_proto",
        "@googleapis//google/rpc:rpc_java_proto",
        "@googleapis//google/type:type_java_proto",
    ],
)

python_grpc_library(
    name = "nbi_python_grpc",
    protos = [":nbi_proto"],
    deps = [
        "//api/common:common_python_proto",
        "//api/nbi/v1alpha/resources:nbi_resources_python_grpc",
    ],
)

go_proto_library(
    name = "nbi_go_grpc",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/nbi/v1alpha",
    proto = ":nbi_proto",
    deps = [
        "//api/common:common_go_proto",
        "//api/nbi/v1alpha/resources:nbi_resources_go_grpc",
        "@org_golang_google_genproto//googleapis/type/interval",
    ],
)
