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
load("//bazel/java_rules:def.bzl", "java_test")

java_format_test(
    name = "java_format_test",
    srcs = glob(["*.java"]),
)

java_test(
    name = "SpacetimeCallCredentialsTest",
    srcs = [
        "SpacetimeCallCredentialsTest.java",
    ],
    resources = [
        "resources/test_private_key.pem",
    ],
    deps = [
        "//api/nbi/v1alpha:nbi_java_grpc",
        "//java/com/aalyria/spacetime/authentication:Authentication",
        "@maven//:com_google_code_gson_gson",
        "@maven//:io_grpc_grpc_api",
        "@maven//:io_grpc_grpc_inprocess",
        "@maven//:io_grpc_grpc_testing",
        "@maven//:io_helidon_grpc_helidon_grpc_core",
        "@maven//:junit_junit",
        "@maven//:org_mockito_mockito_core",
    ],
)
