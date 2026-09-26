load("@rules_go//proto:def.bzl", "go_proto_library")

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

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "simulation_proto",
    srcs = [
        "simulation.proto",
    ],
    deps = [
        "//api/provisioning/v1alpha:provisioning_proto",
        "@googleapis//google/api:field_behavior_proto",
        "@googleapis//google/longrunning:operations_proto",
        "@googleapis//google/type:interval_proto",
        "@org_outernetcouncil_nmts//v1/proto:nmts_proto",
        "@protobuf//:duration_proto",
        "@protobuf//:empty_proto",
        "@protobuf//:timestamp_proto",
    ],
)

go_proto_library(
    name = "simulation_go_proto",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/simulation/v1alpha",
    proto = ":simulation_proto",
    deps = [
        "//api/provisioning/v1alpha:provisioning_go_grpc",
        "@com_google_cloud_go_longrunning//autogen/longrunningpb",
        "@org_golang_google_genproto//googleapis/type/interval",
        "@org_golang_google_genproto_googleapis_api//annotations",
        "@org_outernetcouncil_nmts//v1/proto:nmts_go_proto",
    ],
)
