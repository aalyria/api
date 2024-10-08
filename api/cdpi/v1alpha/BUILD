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

# Protobufs for the AirFlow Control to Data-Plane Interface (CDPI) and its
# associated Configuration & Management Interface (AF-CONFIG).  Unlike
# contemporary SDN protocols, such as OpenFlow and OF-CONFIG, AirFlow supports
# L1-L3 control of networks with wireless and mobile network platforms.

load("@protobuf//bazel:proto_library.bzl", "proto_library")
load("@rules_go//proto:def.bzl", "go_proto_library")
load("@rules_proto_grpc_cpp//:defs.bzl", "cpp_grpc_library")
load("@rules_proto_grpc_java//:defs.bzl", "java_grpc_library")
load("@rules_proto_grpc_python//:defs.bzl", "python_grpc_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "cdpi_proto",
    srcs = [
        "cdpi.proto",
    ],
    deps = [
        "//api/common:common_proto",
        "@googleapis//google/rpc:status_proto",
        "@protobuf//:empty_proto",
        "@protobuf//:timestamp_proto",
    ],
)

cpp_grpc_library(
    name = "cdpi_cpp_grpc",
    protos = [":cdpi_proto"],
    deps = [
        "//api/common:common_cpp_proto",
        "@googleapis//google/rpc:status_cc_proto",
    ],
)

go_proto_library(
    name = "cdpi_go_grpc",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/api/cdpi/v1alpha",
    proto = ":cdpi_proto",
    deps = [
        "//api/common:common_go_proto",
        "@org_golang_google_genproto_googleapis_rpc//status",
    ],
)

java_grpc_library(
    name = "cdpi_java_grpc",
    protos = [":cdpi_proto"],
    deps = [
        "//api/common:common_java_proto",
        "@googleapis//google/rpc:rpc_java_proto",
    ],
)

python_grpc_library(
    name = "cdpi_python_grpc",
    protos = [":cdpi_proto"],
    deps = [
        "//api/common:common_python_proto",
    ],
)
