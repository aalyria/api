# Copyright (c) Aalyria Technologies, Inc., and its affiliates.
# Confidential and Proprietary. All rights reserved.
load("@io_bazel_rules_go//go:def.bzl", "go_library")

package(default_visibility = ["//airflow/agent:__subpackages__"])

go_library(
    name = "extproc",
    srcs = ["extproc.go"],
    importpath = "aalyria.com/minkowski/airflow/agent/enactment/extproc",
    deps = [
        "//airflow/agent",
        "//api/airflow:airflow_go_grpc",
        "//api/common:common_go_proto",
        "@com_github_rs_zerolog//:zerolog",
        "@org_golang_google_grpc//codes",
        "@org_golang_google_grpc//status",
        "@org_golang_google_protobuf//encoding/protojson",
    ],
)
