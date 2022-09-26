# Protobufs for the AirFlow Control to Data-Plane Interface (CDPI) and its
# associated Configuration & Management Interface (AF-CONFIG).  Unlike
# contemporary SDN protocols, such as OpenFlow and OF-CONFIG, AirFlow supports
# L1-L3 control of networks with wireless and mobile network platforms.
#
# Copyright (c) Aalyria Technologies, Inc., and its affiliates.
# Confidential and Proprietary. All rights reserved.

load("@rules_proto_grpc//cpp:defs.bzl", "cpp_grpc_library")
load("@rules_proto_grpc//go:defs.bzl", "go_grpc_library")
load("@rules_proto_grpc//java:defs.bzl", "java_grpc_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "airflow_proto",
    srcs = [
        "cdpi.proto",
    ],
    deps = [
        "//api/common:common_proto",
        "@com_google_googleapis//google/rpc:status_proto",
        "@com_google_protobuf//:empty_proto",
        "@com_google_protobuf//:timestamp_proto",
    ],
)

cpp_grpc_library(
    name = "airflow_cpp_grpc",
    protos = [":airflow_proto"],
    deps = [
        "//api/common:common_cpp_proto",
        "@com_google_googleapis//google/rpc:status_cc_proto",
    ],
)

go_grpc_library(
    name = "airflow_go_grpc",
    importpath = "aalyria.com/minkowski/api/airflow",
    protos = [":airflow_proto"],
    deps = [
        "//api/common:common_go_proto",
        "@go_googleapis//google/rpc:status_go_proto",
    ],
)

java_grpc_library(
    name = "airflow_java_grpc",
    protos = [":airflow_proto"],
    deps = [
        "//api/common:common_java_proto",
        "@com_google_googleapis//google/rpc:rpc_java_proto",
    ],
)
