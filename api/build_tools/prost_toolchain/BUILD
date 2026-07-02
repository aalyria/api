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

load("@rules_rust//rust:defs.bzl", "rust_library_group")
load("@rules_rust_prost//:defs.bzl", "rust_prost_toolchain")

rust_library_group(
    name = "prost_runtime",
    deps = [
        "@st_crates_io//:prost",
    ],
)

rust_library_group(
    name = "tonic_runtime",
    deps = [
        ":prost_runtime",
        "@st_crates_io//:tonic",
        "@st_crates_io//:tonic-prost",
    ],
)

rust_prost_toolchain(
    name = "prost_toolchain_impl",
    compile_well_known_types = False,
    prost_plugin = "@st_crates_io//:protoc-gen-prost__protoc-gen-prost",
    prost_runtime = ":prost_runtime",
    prost_types = "@st_crates_io//:prost-types",
    tonic_plugin = "@st_crates_io//:protoc-gen-tonic__protoc-gen-tonic",
    tonic_runtime = ":tonic_runtime",
)

toolchain(
    name = "prost_toolchain",
    toolchain = "prost_toolchain_impl",
    toolchain_type = "@rules_rust_prost//:toolchain_type",
)
