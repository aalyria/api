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
    name = "resources_proto",
    srcs = [
        "antenna_pattern.proto",
        "coverage.proto",
        "devices_in_region.proto",
        "intent.proto",
        "motion_evaluation.proto",
        "network_element.proto",
        "network_link.proto",
        "service_request.proto",
        "wireless_evaluation.proto",
        "wireless_interference.proto",
    ],
    deps = [
        "//api/common:common_proto",
        "//api/types:types_proto",
        "@googleapis//google/rpc:code_proto",
        "@googleapis//google/type:interval_proto",
        "@googleapis//google/type:money_proto",
        "@protobuf//:any_proto",
        "@protobuf//:duration_proto",
        "@protobuf//:timestamp_proto",
    ],
)

cpp_grpc_library(
    name = "nbi_resources_cpp_grpc",
    generate_mocks = True,
    protos = [":resources_proto"],
    deps = [
        "//api/common:common_cpp_proto",
        "//api/types:types_cpp_proto",
        "@googleapis//google/api:annotations_cc_proto",
        "@googleapis//google/rpc:code_cc_proto",
        "@googleapis//google/type:interval_cc_proto",
        "@googleapis//google/type:money_cc_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/geophys:geophys_cpp_proto",
    ],
)

go_proto_library(
    name = "nbi_resources_go_grpc",
    importpath = "aalyria.com/spacetime/api/nbi/v1alpha/resources",
    proto = ":resources_proto",
    deps = [
        "//api/common:common_go_proto",
        "//api/types:types_go_proto",
        "@org_golang_google_genproto//googleapis/type/interval",
        "@org_golang_google_genproto//googleapis/type/money",
        "@org_golang_google_genproto_googleapis_rpc//code",
    ],
)

java_grpc_library(
    name = "nbi_resources_java_grpc",
    protos = [":resources_proto"],
    deps = [
        "//api/common:common_java_proto",
        "//api/types:types_java_proto",
        "@googleapis//google/api:api_java_proto",
        "@googleapis//google/rpc:rpc_java_proto",
        "@googleapis//google/type:type_java_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/geophys:geophys_java_proto",
    ],
)

python_grpc_library(
    name = "nbi_resources_python_grpc",
    protos = [":resources_proto"],
    deps = [
        "//api/common:common_python_proto",
        "@googleapis//google/rpc:code_py_proto",
        "@googleapis//google/type:money_py_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/geophys:geophys_python_proto",
    ],
)
