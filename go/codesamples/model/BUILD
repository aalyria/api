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

load("@rules_go//go:def.bzl", "go_binary", "go_library")

package(default_visibility = ["//visibility:public"])

go_binary(
    name = "model_client",
    embed = [":codesamples_lib"],
)

go_library(
    name = "codesamples_lib",
    srcs = ["model_client.go"],
    importpath = "aalyria.com/spacetime/github/go/codesamples/model",
    deps = [
        "//api/model/v1:v1_go_proto",
        "//auth",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@org_golang_google_grpc//:grpc",
        "@org_golang_google_grpc//credentials",
        "@org_golang_google_grpc//keepalive",
        "@org_outernetcouncil_nmts//v1/proto:nmts_go_proto",
    ],
)
