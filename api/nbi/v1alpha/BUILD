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

load("@rules_proto_grpc//cpp:defs.bzl", "cpp_grpc_library")
load("@rules_proto_grpc//go:defs.bzl", "go_grpc_library")
load("@rules_proto_grpc//java:defs.bzl", "java_grpc_library")

package(default_visibility = ["//visibility:public"])

proto_library(
    name = "nbi_proto",
    srcs = [
        "nbi.proto",
    ],
    deps = [
        "//api/common:common_proto",
        "//api/nbi/v1alpha/resources:nbi_resources_proto",
        "@com_google_googleapis//google/api:annotations_proto",
        "@com_google_googleapis//google/rpc:code_proto",
        "@com_google_googleapis//google/type:interval_proto",
        "@com_google_protobuf//:any_proto",
        "@com_google_protobuf//:duration_proto",
        "@com_google_protobuf//:timestamp_proto",
    ],
)

cpp_grpc_library(
    name = "nbi_cpp_grpc",
    generate_mocks = True,
    protos = [":nbi_proto"],
    deps = [
        "//api/common:common_cpp_proto",
        "//api/nbi/v1alpha/resources:nbi_resources_cpp_grpc",
        "@com_google_googleapis//google/api:annotations_cc_proto",
        "@com_google_googleapis//google/rpc:code_cc_proto",
        "@com_google_googleapis//google/type:interval_cc_proto",
    ],
)

go_grpc_library(
    name = "nbi_go_grpc",
    importpath = "aalyria.com/spacetime/api/nbi/v1alpha",
    protos = [":nbi_proto"],
    deps = [
        "//api/common:common_go_proto",
        "//api/nbi/v1alpha/resources:nbi_resources_go_grpc",
        "@go_googleapis//google/api:annotations_go_proto",
        "@go_googleapis//google/rpc:code_go_proto",
        "@go_googleapis//google/type:interval_go_proto",
    ],
)

java_grpc_library(
    name = "nbi_java_grpc",
    protos = [":nbi_proto"],
    deps = [
        "//api/common:common_java_proto",
        "//api/nbi/v1alpha/resources:nbi_resources_java_grpc",
        "@com_google_googleapis//google/api:api_java_proto",
        "@com_google_googleapis//google/rpc:rpc_java_proto",
        "@com_google_googleapis//google/type:type_java_proto",
    ],
)
