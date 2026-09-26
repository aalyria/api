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

go_library(
    name = "quictransport",
    srcs = ["quictransport.go"],
    importpath = "aalyria.com/spacetime/common/quictransport",
    visibility = ["//visibility:public"],
    deps = [
        "@com_github_quic_go_quic_go//:quic-go",
        "@com_github_quic_go_quic_go//http3",
        "@org_golang_x_net//http2",
    ],
)

go_test(
    name = "quictransport_test",
    srcs = ["quictransport_test.go"],
    embed = [":quictransport"],
    deps = [
        "@com_github_quic_go_quic_go//:quic-go",
        "@com_github_quic_go_quic_go//http3",
        "@org_golang_google_grpc//:grpc",
        "@org_golang_google_grpc//credentials/insecure",
        "@org_golang_google_grpc//health",
        "@org_golang_google_grpc//health/grpc_health_v1",
    ],
)
