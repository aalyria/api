# Copyright (c) Aalyria Technologies, Inc., and its affiliates.
# Confidential and Proprietary. All rights reserved.
load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_binary(
    name = "extproc_agent",
    embed = [":extproc_agent_lib"],
    visibility = ["//visibility:public"],
)

go_library(
    name = "extproc_agent_lib",
    srcs = ["extproc_agent.go"],
    importpath = "aalyria.com/minkowski/airflow/agent/cmd/extproc_agent",
    visibility = ["//visibility:private"],
    deps = [
        "//airflow/agent",
        "//airflow/agent/enactment/extproc",
        "//api/airflow:airflow_go_grpc",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_rs_zerolog//:zerolog",
        "@org_golang_google_api//idtoken",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//backoff",
        "@org_golang_google_grpc//credentials",
        "@org_golang_google_grpc//credentials/insecure",
        "@org_golang_google_grpc//credentials/oauth",
        "@org_golang_google_protobuf//proto",
        "@org_golang_x_oauth2//:oauth2",
    ],
)
