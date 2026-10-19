# Copyright (c) Aalyria Technologies, Inc., and its affiliates.
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
    name = "pfsense",
    srcs = ["interface.go"],
    importpath = "aalyria.com/spacetime/agent/internal/pfsense",
)

go_test(
    name = "pfsense_test",
    srcs = ["interface_test.go"],
    embed = [":pfsense"],
    deps = [
        "@com_github_google_go_cmp//cmp",
    ],
)
