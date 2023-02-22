# Copyright (c) Aalyria Technologies, Inc., and its affiliates.
# Confidential and Proprietary. All rights reserved.
load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_binary(
    name = "generate_jwt",
    embed = [":generate_jwt_lib"],
    visibility = ["//visibility:public"],
)

go_library(
    name = "generate_jwt_lib",
    srcs = ["generate_jwt.go"],
    importpath = "aalyria.com/minkowski/airflow/agent/cmd/generate_jwt",
)
