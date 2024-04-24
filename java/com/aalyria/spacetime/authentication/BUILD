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

load("//bazel/java_format_rules:def.bzl", "java_format_test")
load("//bazel/java_rules:def.bzl", "java_library")

package(default_visibility = ["//visibility:public"])

java_format_test(
    name = "java_format_test",
    srcs = glob(["*.java"]),
)

java_library(
    name = "Authentication",
    srcs = [
        "JwtManager.java",
        "SpacetimeCallCredentials.java",
    ],
    deps = [
        "@io_grpc_grpc_java//api",
        "@maven//:com_google_code_gson_gson",
        "@maven//:com_google_guava_guava",
        "@maven//:org_bouncycastle_bcprov_jdk15on",
    ],
)
