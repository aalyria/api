# Copyright (c) Aalyria Technologies, Inc., and its affiliates.
# Confidential and Proprietary. All rights reserved.

load("@rules_proto_grpc//cpp:defs.bzl", "cpp_proto_library")
load("@rules_proto_grpc//go:defs.bzl", "go_proto_library")
load("@rules_proto_grpc//java:defs.bzl", "java_proto_library")

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
        "@com_google_googleapis//google/rpc:status_proto",
        "@com_google_googleapis//google/type:interval_proto",
        "@com_google_protobuf//:duration_proto",
        "@com_google_protobuf//:empty_proto",
        "@com_google_protobuf//:timestamp_proto",
    ],
)

cpp_proto_library(
    name = "common_cpp_proto",
    protos = [":common_proto"],
    deps = [
        "@com_google_googleapis//google/rpc:status_cc_proto",
        "@com_google_googleapis//google/type:interval_cc_proto",
    ],
)

go_proto_library(
    name = "common_go_proto",
    importpath = "aalyria.com/minkowski/api/common",
    protos = [":common_proto"],
    deps = [
        "@go_googleapis//google/rpc:status_go_proto",
        "@go_googleapis//google/type:interval_go_proto",
    ],
)

java_proto_library(
    name = "common_java_proto",
    protos = [":common_proto"],
    deps = [
        "@com_google_googleapis//google/rpc:rpc_java_proto",
        "@com_google_googleapis//google/type:type_java_proto",
    ],
)
