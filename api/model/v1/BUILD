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

proto_library(
    name = "model_proto",
    srcs = ["model.proto"],
    deps = [
        "@googleapis//google/api:field_behavior_proto",
        "@googleapis//google/longrunning:operations_proto",
        "@org_outernetcouncil_nmts//v1/proto:nmts_proto",
        "@protobuf//:empty_proto",
        "@protobuf//:field_mask_proto",
        "@protobuf//:timestamp_proto",
    ],
)

cpp_grpc_library(
    name = "model_cpp_grpc",
    generate_mocks = True,
    protos = [":model_proto"],
    deps = [
        "@googleapis//google/api:field_behavior_cc_proto",
        "@googleapis//google/longrunning:longrunning_cc_proto",
        "@org_outernetcouncil_nmts//v1/proto:nmts_cpp_proto",
    ],
)

java_grpc_library(
    name = "model_java_grpc",
    protos = [":model_proto"],
    deps = [
        "@googleapis//google/api:api_java_proto",
        "@googleapis//google/longrunning:longrunning_java_proto",
        "@org_outernetcouncil_nmts//v1/proto:nmts_java_proto",
    ],
)

python_grpc_library(
    name = "model_python_grpc",
    protos = [":model_proto"],
    deps = [
        "@googleapis//google/api:field_behavior_py_proto",
        "@googleapis//google/longrunning:operations_py_proto",
        "@org_outernetcouncil_nmts//v1/proto:nmts_python_proto",
    ],
)

go_proto_library(
    name = "v1_go_proto",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/model/v1",
    proto = ":model_proto",
    deps = [
        "@com_google_cloud_go_longrunning//autogen/longrunningpb",
        "@org_golang_google_genproto_googleapis_api//annotations",
        "@org_outernetcouncil_nmts//v1/proto:nmts_go_proto",
    ],
)
