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

load("@rules_go//go:def.bzl", "go_library", "go_test")

package(default_visibility = ["//visibility:public"])

go_library(
    name = "telemetry",
    srcs = [
        "periodic_driver.go",
        "telemetry.go",
    ],
    importpath = "aalyria.com/spacetime/agent/telemetry",
    deps = [
        "//api/telemetry/v1alpha:telemetry_go_grpc",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_rs_zerolog//:zerolog",
    ],
)

go_test(
    name = "telemetry_test",
    srcs = ["periodic_driver_test.go"],
    embed = [":telemetry"],
    deps = [
        "//api/telemetry/v1alpha:telemetry_go_grpc",
        "@com_github_google_go_cmp//cmp",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@org_golang_google_protobuf//testing/protocmp",
        "@org_golang_google_protobuf//types/known/timestamppb",
        "@org_golang_x_sync//errgroup",
    ],
)
