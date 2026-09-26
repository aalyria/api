# Copyright 2024 Aalyria Technologies, Inc., and its affiliates.
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

load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_go//go:def.bzl", "go_binary", "go_library")
load("@rules_img//img:image.bzl", "image_manifest")
load("@rules_img//img:layer.bzl", "image_layer")

package(default_visibility = ["//visibility:public"])

go_binary(
    name = "netlink_telemetry",
    embed = [":netlink_telemetry_lib"],
    pure = "on",
    static = "on",
)

go_library(
    name = "netlink_telemetry_lib",
    srcs = ["netlink_telemetry.go"],
    importpath = "aalyria.com/spacetime/agent/telemetry/netlink/container_test",
    deps = [
        "//agent/telemetry/netlink",
        "//api/telemetry/v1alpha:telemetry_go_grpc",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_vishvananda_netlink//:netlink",
        "@org_golang_x_sync//errgroup",
    ],
)

image_layer(
    name = "netlink_telemetry_layer",
    srcs = {
        "/netlink_telemetry": ":netlink_telemetry",
    },
)

image_manifest(
    name = "netlink_telemetry_image",
    base = "@alpine_base",
    layers = [":netlink_telemetry_layer"],
    user = "root",
)

filegroup(
    name = "netlink_telemetry_image_layout",
    srcs = [":netlink_telemetry_image"],
    output_group = "oci_layout",
)

# You'll need sudoless docker to run this:
# https://docs.docker.com/engine/install/linux-postinstall/#manage-docker-as-a-non-root-user
container_structure_test(
    name = "netlink_telemetry_tests",
    configs = [":netlink_telemetry_tests.yaml"],
    driver = "docker",
    image = ":netlink_telemetry_image_layout",
    tags = [
        "manual",
        "no_ci_pipeline",
    ],
)
