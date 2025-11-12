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
load("@rules_proto_grpc_cpp//:defs.bzl", "cpp_grpc_library", "cpp_proto_library")
load("@rules_proto_grpc_python//:defs.bzl", "python_grpc_library")

proto_library(
    name = "solver_proto",
    srcs = ["solver.proto"],
    visibility = ["//visibility:public"],
    deps = [
        "//api/nbi/v1alpha/resources:resources_proto",
        "@googleapis//google/type:interval_proto",
    ],
)

go_proto_library(
    name = "solver_go_proto",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/solver/v1alpha",
    proto = ":solver_proto",
    visibility = ["//visibility:public"],
    deps = [
        "//api/nbi/v1alpha/resources:nbi_resources_go_grpc",
        "@org_golang_google_genproto//googleapis/type/interval",
    ],
)

proto_library(
    name = "beam_hopping_proto",
    srcs = ["beam_hopping.proto"],
    visibility = ["//visibility:public"],
    deps = [
        "//api/scheduling/v1alpha:scheduling_proto",
        "@protobuf//:timestamp_proto",
    ],
)

cpp_grpc_library(
    name = "beam_hopping_cpp_grpc",
    protos = [":beam_hopping_proto"],
    visibility = ["//visibility:public"],
    deps = [
        "//api/common:common_cpp_proto",
        "//api/scheduling/v1alpha:scheduling_cpp_grpc",
        "//api/telemetry/v1alpha:telemetry_cpp_grpc",
        "@googleapis//google/rpc:status_cc_proto",
    ],
)

cpp_proto_library(
    name = "beam_hopping_cpp_proto",
    protos = ["beam_hopping_proto"],
    visibility = ["//visibility:public"],
    deps = [
        "//api/common:common_cpp_proto",
        "//api/solver/v1alpha:beam_hopping_cpp_grpc",
    ],
)

go_proto_library(
    name = "beam_hopping_go_grpc",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/beam_hopping/v1alpha",
    proto = ":beam_hopping_proto",
    visibility = ["//visibility:public"],
    deps = ["//api/scheduling/v1alpha:scheduling_go_grpc"],
)

python_grpc_library(
    name = "beam_hopping_python_grpc",
    protos = [":beam_hopping_proto"],
    visibility = ["//visibility:public"],
    deps = [
        "//api/common:common_python_proto",
        "//api/scheduling/v1alpha:scheduling_python_grpc",
        "//api/telemetry/v1alpha:telemetry_python_grpc",
        "@googleapis//google/rpc:status_py_proto",
    ],
)

python_grpc_library(
    name = "solver_python_grpc",
    protos = [":solver_proto"],
    visibility = ["//visibility:public"],
    deps = [
        "//api/nbi/v1alpha/resources:nbi_resources_python_grpc",
        "@googleapis//google/type:interval_py_proto",
    ],
)
