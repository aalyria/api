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

load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")
load("@rules_proto//proto:defs.bzl", "proto_library")
load("@rules_proto_grpc//cpp:defs.bzl", "cpp_grpc_library")
load("@rules_proto_grpc//java:defs.bzl", "java_grpc_library")
load("@rules_proto_grpc//python:defs.bzl", "python_grpc_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "model_proto",
    srcs = ["model.proto"],
    deps = [
        "@com_google_protobuf//:empty_proto",
        "@org_outernetcouncil_nmts//proto:nmts_proto",
    ],
)

cpp_grpc_library(
    name = "model_cpp_grpc",
    generate_mocks = True,
    protos = [":model_proto"],
    deps = ["@org_outernetcouncil_nmts//proto:nmts_cpp_proto"],
)

java_grpc_library(
    name = "model_java_grpc",
    protos = [":model_proto"],
    deps = ["@org_outernetcouncil_nmts//proto:nmts_java_proto"],
)

python_grpc_library(
    name = "model_python_grpc",
    protos = [":model_proto"],
    deps = ["@org_outernetcouncil_nmts//proto:nmts_python_proto"],
)

go_proto_library(
    name = "v1alpha_go_proto",
    compilers = [
        "@io_bazel_rules_go//proto:go_proto",
        "@io_bazel_rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/model/v1alpha",
    proto = ":model_proto",
    deps = ["@org_outernetcouncil_nmts//proto:nmts_go_proto"],
)
