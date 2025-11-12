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

load("@protobuf//bazel:proto_library.bzl", "proto_library")
load("@rules_go//proto:def.bzl", "go_proto_library")
load("@rules_proto_grpc_python//:defs.bzl", "python_grpc_library")

proto_library(
    name = "federation_proto",
    srcs = ["federation.proto"],
    visibility = ["//visibility:public"],
    deps = [
        "//api/common:common_proto",
        "//api/types:types_proto",
        "@googleapis//google/type:interval_proto",
        "@protobuf//:duration_proto",
    ],
)

go_proto_library(
    name = "federation_go_proto",
    compilers = [
        "@rules_go//proto:go_grpc_v2",
        "@rules_go//proto:go_proto",
    ],
    importpath = "aalyria.com/spacetime/api/fed/v1alpha",
    proto = ":federation_proto",
    visibility = ["//visibility:public"],
    deps = [
        "//api/common:common_go_proto",
        "//api/types:types_go_proto",
        "@org_golang_google_genproto//googleapis/type/interval",
    ],
)

go_proto_library(
    name = "federation_go_grpc",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/fed/v1alpha",
    proto = ":federation_proto",
    visibility = ["//visibility:public"],
    deps = [
        "//api/common:common_go_proto",
        "//api/types:types_go_proto",
        "@org_golang_google_genproto//googleapis/type/interval",
    ],
)

python_grpc_library(
    name = "federation_python_grpc",
    protos = [":federation_proto"],
    visibility = ["//visibility:public"],
    deps = [
        "//api/common:common_python_proto",
        "//api/federation/interconnect/v1alpha:interconnect_python_grpc",
        "//api/types:types_python_proto",
    ],
)
