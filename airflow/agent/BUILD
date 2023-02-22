# Build rules for the Go CDPI agent. These shouldn't depend on any other
# internal packages apart from those found in //api.
#
# Copyright (c) Aalyria Technologies, Inc., and its affiliates.
# Confidential and Proprietary. All rights reserved.
load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

package(default_visibility = ["//airflow/agent:__subpackages__"])

go_library(
    name = "agent",
    srcs = ["agent.go"],
    importpath = "aalyria.com/minkowski/airflow/agent",
    deps = [
        "//api/airflow:airflow_go_grpc",
        "//api/common:common_go_proto",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_rs_zerolog//:zerolog",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//codes",
        "@org_golang_google_grpc//status",
        "@org_golang_google_protobuf//proto",
        "@org_golang_google_protobuf//types/known/timestamppb",
        "@org_golang_x_sync//errgroup",
    ],
)

go_test(
    name = "agent_test",
    srcs = ["agent_test.go"],
    deps = [
        ":agent",
        "//api/airflow:airflow_go_grpc",
        "//api/common:common_go_proto",
        "@com_github_google_go_cmp//cmp",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//codes",
        "@org_golang_google_grpc//credentials/insecure",
        "@org_golang_google_grpc//status",
        "@org_golang_google_protobuf//proto",
        "@org_golang_google_protobuf//testing/protocmp",
        "@org_golang_google_protobuf//types/known/timestamppb",
        "@org_golang_x_sync//errgroup",
    ],
)
