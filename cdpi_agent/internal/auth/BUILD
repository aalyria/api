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

load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "auth",
    srcs = ["auth.go"],
    importpath = "aalyria.com/spacetime/cdpi_agent/internal/auth",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_golang_jwt_jwt_v5//:jwt",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@org_golang_google_grpc//credentials",
        "@org_golang_x_oauth2//:oauth2",
        "@org_golang_x_oauth2//jwt",
    ],
)

go_test(
    name = "auth_test",
    srcs = ["auth_test.go"],
    embed = [":auth"],
    tags = ["block-network"],
    deps = ["@com_github_jonboulle_clockwork//:clockwork"],
)
