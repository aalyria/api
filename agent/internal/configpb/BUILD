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

load("@protobuf//bazel:proto_library.bzl", "proto_library")
load("@rules_go//proto:def.bzl", "go_proto_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "configpb_proto",
    srcs = ["config.proto"],
    deps = [
        "@protobuf//:any_proto",
        "@protobuf//:duration_proto",
        "@protobuf//:empty_proto",
    ],
)

go_proto_library(
    name = "configpb_go_proto",
    importpath = "aalyria.com/spacetime/agent/internal/configpb",
    proto = ":configpb_proto",
)
