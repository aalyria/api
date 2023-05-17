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

load("@rules_proto//proto:defs.bzl", "proto_library")
load("@rules_proto_grpc//cpp:defs.bzl", "cpp_proto_library")
load("@rules_proto_grpc//go:defs.bzl", "go_proto_library")
load("@rules_proto_grpc//java:defs.bzl", "java_proto_library")
load("@rules_proto_grpc//python:defs.bzl", "python_proto_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "common_proto",
    srcs = [
        "channel.proto",
        "control.proto",
        "control_beam.proto",
        "control_flow.proto",
        "control_radio.proto",
        "control_tunnel.proto",
        "coordinates.proto",
        "network.proto",
        "platform.proto",
        "platform_antenna.proto",
        "telemetry.proto",
        "time.proto",
        "tunnel.proto",
        "wireless.proto",
        "wireless_modcod.proto",
        "wireless_propagation.proto",
        "wireless_receiver.proto",
        "wireless_transceiver.proto",
        "wireless_transmitter.proto",
    ],
    deps = [
        "//api/types:types_proto",
        "@com_google_protobuf//:duration_proto",
        "@com_google_protobuf//:empty_proto",
        "@com_google_protobuf//:timestamp_proto",
        "@go_googleapis//google/rpc:status_proto",
        "@go_googleapis//google/type:interval_proto",
    ],
)

cpp_proto_library(
    name = "common_cpp_proto",
    protos = [":common_proto"],
    deps = [
        "//api/types:types_cpp_proto",
        "@com_google_googleapis//google/rpc:status_cc_proto",
        "@com_google_googleapis//google/type:interval_cc_proto",
    ],
)

go_proto_library(
    name = "common_go_proto",
    importpath = "aalyria.com/spacetime/api/common",
    protos = [":common_proto"],
    deps = [
        "//api/types:types_go_proto",
        "@go_googleapis//google/rpc:status_go_proto",
        "@go_googleapis//google/type:interval_go_proto",
    ],
)

java_proto_library(
    name = "common_java_proto",
    protos = [":common_proto"],
    deps = [
        "//api/types:types_java_proto",
        "@com_google_googleapis//google/rpc:rpc_java_proto",
        "@com_google_googleapis//google/type:type_java_proto",
    ],
)

python_proto_library(
    name = "common_python_proto",
    protos = [":common_proto"],
    deps = [
        "//api/types:types_python_proto",
        "@com_google_googleapis//google/rpc:status_py_proto",
        "@com_google_googleapis//google/type:interval_py_proto",
    ],
)
