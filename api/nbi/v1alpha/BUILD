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

load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")
load("@rules_proto//proto:defs.bzl", "proto_library")
load("@rules_proto_grpc//cpp:defs.bzl", "cpp_grpc_library")
load("@rules_proto_grpc//java:defs.bzl", "java_grpc_library")
load("@rules_proto_grpc//python:defs.bzl", "python_grpc_library")

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
        "@com_google_googleapis//google/type:interval_proto",
        "@com_google_protobuf//:duration_proto",
        "@com_google_protobuf//:timestamp_proto",
    ],
)

cpp_grpc_library(
    name = "nbi_cpp_grpc",
    generate_mocks = True,
    protos = [":nbi_proto"],
    deps = [
        "//api/common:common_cpp_proto",
        "//api/nbi/v1alpha/resources:nbi_resources_cpp_grpc",
        "@com_google_googleapis//google/rpc:code_cc_proto",
        "@com_google_googleapis//google/type:interval_cc_proto",
    ],
)

java_grpc_library(
    name = "nbi_java_grpc",
    protos = [":nbi_proto"],
    deps = [
        "//api/common:common_java_proto",
        "//api/nbi/v1alpha/resources:nbi_resources_java_grpc",
        "@com_google_googleapis//google/api:api_java_proto",
        "@com_google_googleapis//google/rpc:rpc_java_proto",
        "@com_google_googleapis//google/type:type_java_proto",
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
    name = "v1alpha_go_proto",
    compilers = [
        "@io_bazel_rules_go//proto:go_proto",
        "@io_bazel_rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/nbi/v1alpha",
    proto = ":nbi_proto",
    deps = [
        "//api/common:common_go_proto",
        "//api/nbi/v1alpha/resources:nbi_resources_go_grpc",
        "@org_golang_google_genproto//googleapis/type/interval",
    ],
)
