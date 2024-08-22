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

proto_library(
    name = "solver_proto",
    srcs = ["solver.proto"],
    visibility = ["//visibility:public"],
    deps = [
        "//api/nbi/v1alpha/resources:resources_proto",
        "@com_google_googleapis//google/type:interval_proto",
    ],
)

go_proto_library(
    name = "solver_go_proto",
    compilers = [
        "@io_bazel_rules_go//proto:go_proto",
        "@io_bazel_rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/solver/v1alpha",
    proto = ":solver_proto",
    visibility = ["//visibility:public"],
    deps = [
        "//api/nbi/v1alpha/resources:nbi_resources_go_grpc",
        "@org_golang_google_genproto//googleapis/type/interval",
    ],
)
