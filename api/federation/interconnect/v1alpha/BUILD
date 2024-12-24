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

proto_library(
    name = "interconnect_proto",
    srcs = ["interconnect.proto"],
    visibility = ["//visibility:public"],
    deps = [
        "@googleapis//google/type:interval_proto",
        "@org_outernetcouncil_nmts//v1alpha/proto/ek/physical:antenna_proto",
        "@org_outernetcouncil_nmts//v1alpha/proto/ek/physical:modem_proto",
        "@org_outernetcouncil_nmts//v1alpha/proto/ek/physical:platform_proto",
        "@org_outernetcouncil_nmts//v1alpha/proto/ek/physical:signal_processing_chain_proto",
        "@org_outernetcouncil_nmts//v1alpha/proto/ek/physical:transceiver_proto",
        "@org_outernetcouncil_nmts//v1alpha/proto/types/geophys:motion_proto",
        "@protobuf//:empty_proto",
    ],
)

go_proto_library(
    name = "interconnect_go_proto",
    compilers = ["@rules_go//proto:go_grpc"],
    importpath = "aalyria.com/spacetime/api/federation/interconnect/v1alpha",
    proto = ":interconnect_proto",
    visibility = ["//visibility:public"],
    deps = [
        "@org_golang_google_genproto//googleapis/type/interval",
        "@org_outernetcouncil_nmts//v1alpha/proto/ek/physical:physical_go_proto",
        "@org_outernetcouncil_nmts//v1alpha/proto/types/geophys:geophys_go_proto",
        "@org_outernetcouncil_nmts//v1alpha/proto/types/ietf:ietf_go_proto",
    ],
)
