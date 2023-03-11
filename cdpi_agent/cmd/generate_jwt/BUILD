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
load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_binary(
    name = "generate_jwt",
    embed = [":generate_jwt_lib"],
    visibility = ["//visibility:public"],
)

go_library(
    name = "generate_jwt_lib",
    srcs = ["generate_jwt.go"],
    importpath = "aalyria.com/spacetime/cdpi_agent/cmd/generate_jwt",
)
