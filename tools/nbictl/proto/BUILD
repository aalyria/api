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

load("@protobuf//bazel:proto_library.bzl", "proto_library")
load("@rules_go//proto:def.bzl", "go_proto_library")
load("@rules_proto_grpc_java//:defs.bzl", "java_proto_library")
load("@rules_proto_grpc_python//:defs.bzl", "python_proto_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "nbictl_proto",
    srcs = [
        "nbi_ctl_config.proto",
    ],
    deps = [
        "@protobuf//:empty_proto",
    ],
)

go_proto_library(
    name = "nbictl_go_proto",
    importpath = "aalyria.com/spacetime/tools/nbictl/nbictlpb",
    proto = ":nbictl_proto",
)

proto_library(
    name = "provisioning_proto",
    srcs = ["provisioning.proto"],
    deps = ["//api/provisioning/v1alpha:provisioning_proto"],
)

java_proto_library(
    name = "provisioning_java_proto",
    protos = [":provisioning_proto"],
    deps = ["//api/provisioning/v1alpha:provisioning_java_grpc"],
)

python_proto_library(
    name = "provisioning_python_proto",
    protos = [":provisioning_proto"],
    deps = ["//api/provisioning/v1alpha:provisioning_python_grpc"],
)

go_proto_library(
    name = "provisioning_go_proto",
    compilers = [
        "@rules_go//proto:go_proto",
        "@rules_go//proto:go_grpc_v2",
    ],
    importpath = "aalyria.com/spacetime/tools/nbictl/provisioning",
    proto = ":provisioning_proto",
    deps = ["//api/provisioning/v1alpha:provisioning_go_grpc"],
)
