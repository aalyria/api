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
load("@rules_proto_grpc_python//:defs.bzl", "python_grpc_library")

proto_library(
    name = "transponder_proto",
    srcs = ["transponder.proto"],
    visibility = ["//visibility:public"],
    deps = [
        "@googleapis//google/api:field_behavior_proto",
        "@googleapis//google/longrunning:operations_proto",
        "@googleapis//google/type:interval_proto",
        "@googleapis//google/type:latlng_proto",
        "@org_outernetcouncil_nmts//v1/proto/types/geophys:motion_proto",
        "@protobuf//:empty_proto",
    ],
)

go_proto_library(
    name = "transponder_go_proto",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/federation/transponder/v1alpha",
    proto = ":transponder_proto",
    visibility = ["//visibility:public"],
    deps = [
        "@com_google_cloud_go_longrunning//autogen/longrunningpb",
        "@org_golang_google_genproto//googleapis/type/interval",
        "@org_golang_google_genproto//googleapis/type/latlng",
        "@org_golang_google_genproto_googleapis_api//annotations",
        "@org_outernetcouncil_nmts//v1/proto/types/geophys:geophys_go_proto",
    ],
)

python_grpc_library(
    name = "transponder_python_grpc",
    protos = [":transponder_proto"],
    visibility = ["//visibility:public"],
    deps = [
        "//api/common:common_python_proto",
        "@googleapis//google/api:field_behavior_py_proto",
        "@googleapis//google/longrunning:operations_py_proto",
        "@googleapis//google/rpc:code_py_proto",
        "@googleapis//google/type:latlng_py_proto",
    ],
)
