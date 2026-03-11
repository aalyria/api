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

# @com_github_vishvananda_netlink//:netlink

load("@container_structure_test//:defs.bzl", "container_structure_test")
load("@rules_go//go:def.bzl", "go_binary", "go_library")
load("@rules_oci//oci:defs.bzl", "oci_image", "oci_load")
load("@rules_pkg//:pkg.bzl", "pkg_tar")

package(default_visibility = ["//visibility:public"])

go_binary(
    name = "netlink_exercise",
    embed = [":netlink_exercise_lib"],
    pure = "on",
    static = "on",
)

go_library(
    name = "netlink_exercise_lib",
    srcs = ["netlink_exercise.go"],
    importpath = "aalyria.com/spacetime/agent/enactment/netlink/container_test",
    deps = [
        "//agent/enactment/netlink",
        "//api/scheduling/v1alpha:scheduling_go_grpc",
        "@com_github_jonboulle_clockwork//:clockwork",
        "@com_github_vishvananda_netlink//:netlink",
    ],
)

pkg_tar(
    name = "netlink_exercise_tar",
    srcs = [
        ":debug.sh",
        ":netlink_exercise",
    ],
)

oci_image(
    name = "netlink_exercise_image",
    base = "@alpine_base",
    tars = [":netlink_exercise_tar"],
    user = "root",
)

oci_load(
    name = "netlink_exercise_image.docker.tar",
    format = "docker",
    image = ":netlink_exercise_image",
    repo_tags = ["netlink_exercise_image:latest"],
)

# To make this work in cloudtop (due to docker socket permissions errors) I had to:
# sudo usermod -aG docker $USER
# reboot cloudtop (thanks Ciaran!)
container_structure_test(
    name = "netlink_exercise_tests",
    configs = [":netlink_exercise_tests.yaml"],
    driver = "docker",
    image = ":netlink_exercise_image",
    tags = [
        "manual",
        "no_ci_pipeline",
    ],
)
