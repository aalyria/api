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
load("@gazelle//:def.bzl", "gazelle", "gazelle_test")
load("//:version.bzl", "VERSION")

gazelle(name = "gazelle")

gazelle_test(
    name = "gazelle_test",
    tags = ["manual"],
    workspace = "//:BUILD",
)

genrule(
    name = "version",
    outs = ["print_version.sh"],
    cmd = "echo '#!/bin/bash\necho " + VERSION + "' > $@",
    executable = True,
)
