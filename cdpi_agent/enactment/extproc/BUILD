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

package(default_visibility = ["//visibility:public"])

go_library(
    name = "extproc",
    srcs = ["extproc.go"],
    importpath = "aalyria.com/spacetime/cdpi_agent/enactment/extproc",
    deps = [
        "//api/common:common_go_proto",
        "//api/scheduling/v1alpha:scheduling_go_grpc",
        "//cdpi_agent/enactment",
        "//cdpi_agent/internal/extprocs",
        "//cdpi_agent/internal/loggable",
        "//cdpi_agent/internal/protofmt",
        "@com_github_rs_zerolog//:zerolog",
    ],
)

go_test(
    name = "extproc_test",
    srcs = ["extproc_test.go"],
    embed = [":extproc"],
    deps = [
        "//api/common:common_go_proto",
        "//api/scheduling/v1alpha:scheduling_go_grpc",
        "//cdpi_agent/internal/protofmt",
        "@com_github_google_go_cmp//cmp",
    ],
)
