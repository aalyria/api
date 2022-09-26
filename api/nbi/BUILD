# Copyright (c) Aalyria Technologies, Inc., and its affiliates.
# Confidential and Proprietary. All rights reserved.

load("@rules_proto_grpc//cpp:defs.bzl", "cpp_grpc_library")
load("@rules_proto_grpc//go:defs.bzl", "go_grpc_library")
load("@rules_proto_grpc//java:defs.bzl", "java_grpc_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "nbi_proto",
    srcs = [
        "antenna_pattern.proto",
        "app_routing.proto",
        "coverage.proto",
        "czml.proto",
        "intent.proto",
        "nbi.proto",
        "network_element.proto",
        "network_link.proto",
        "visualization.proto",
        "wireless_interference.proto",
    ],
    deps = [
        "//api/common:common_proto",
        "@com_google_googleapis//google/api:annotations_proto",
        "@com_google_googleapis//google/rpc:code_proto",
        "@com_google_googleapis//google/type:interval_proto",
        "@com_google_protobuf//:any_proto",
        "@com_google_protobuf//:duration_proto",
        "@com_google_protobuf//:timestamp_proto",
    ],
)

cpp_grpc_library(
    name = "nbi_cpp_grpc",
    generate_mocks = True,
    protos = [":nbi_proto"],
    deps = [
        "//api/common:common_cpp_proto",
        "@com_google_googleapis//google/api:annotations_cc_proto",
        "@com_google_googleapis//google/rpc:code_cc_proto",
        "@com_google_googleapis//google/type:interval_cc_proto",
    ],
)

go_grpc_library(
    name = "nbi_go_grpc",
    importpath = "aalyria.com/minkowski/api/nbi",
    protos = [":nbi_proto"],
    deps = [
        "//api/common:common_go_proto",
        "@go_googleapis//google/api:annotations_go_proto",
        "@go_googleapis//google/rpc:code_go_proto",
        "@go_googleapis//google/type:interval_go_proto",
    ],
)

java_grpc_library(
    name = "nbi_java_grpc",
    protos = [":nbi_proto"],
    deps = [
        "//api/common:common_java_proto",
        "@com_google_googleapis//google/api:api_java_proto",
        "@com_google_googleapis//google/rpc:rpc_java_proto",
        "@com_google_googleapis//google/type:type_java_proto",
    ],
)
