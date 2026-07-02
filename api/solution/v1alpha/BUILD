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
load("@rules_rust_prost//:defs.bzl", "rust_prost_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "solution_proto",
    srcs = [
        "solution.proto",
        "solution_mutation.proto",
    ],
    deps = [
        "@googleapis//google/api:field_behavior_proto",
        "@googleapis//google/api:resource_proto",
        "@googleapis//google/type:interval_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/geophys:coordinates_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/physical:polarization_proto",
        "@protobuf//:duration_proto",
        "@protobuf//:empty_proto",
        "@protobuf//:field_mask_proto",
        "@protobuf//:timestamp_proto",
    ],
)

go_proto_library(
    name = "solution_go_proto",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/solution/v1alpha",
    proto = ":solution_proto",
    deps = [
        "@org_golang_google_genproto//googleapis/type/interval",
        "@org_golang_google_genproto_googleapis_api//annotations",
        "@org_outernetcouncil_nmts//v1/proto/types/geophys:geophys_go_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/physical:physical_go_proto",
    ],
)

java_grpc_library(
    name = "solution_java_grpc",
    protos = [":solution_proto"],
    deps = [
        "@googleapis//google/api:api_java_proto",
        "@googleapis//google/type:type_java_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/geophys:geophys_java_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/physical:physical_java_proto",
    ],
)

cpp_grpc_library(
    name = "solution_cpp_grpc",
    protos = [":solution_proto"],
    deps = [
        "@googleapis//google/api:field_behavior_cc_proto",
        "@googleapis//google/api:resource_cc_proto",
        "@googleapis//google/type:interval_cc_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/geophys:geophys_cpp_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/physical:physical_cpp_proto",
    ],
)

python_grpc_library(
    name = "solution_python_grpc",
    protos = [":solution_proto"],
    deps = [
        "@googleapis//google/api:field_behavior_py_proto",
        "@googleapis//google/api:resource_py_proto",
        "@googleapis//google/type:interval_py_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/geophys:geophys_python_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/physical:physical_python_proto",
    ],
)

rust_prost_library(
    name = "solution_rust_grpc",
    proto = ":solution_proto",
)
