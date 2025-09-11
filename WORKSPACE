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

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

SCIP_JAVA_VERSION = "0.0.6"

http_archive(
    name = "scip_java",
    sha256 = "7013fa54a6c764999cb5dce0b8a04a983d6335d3fa73e474f4f229dcbf2ce30b",
    strip_prefix = "scip-java-{}".format(SCIP_JAVA_VERSION),
    url = "https://github.com/ciarand/scip-java/archive/refs/tags/v{}.zip".format(SCIP_JAVA_VERSION),
)
