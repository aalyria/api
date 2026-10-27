#!/usr/bin/env bash
#
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

# Runs rules_go's gopackagesdriver for gopls. The driver is built for the host
# platform once per MODULE.bazel, then run directly: `bazel run --config=host`
# would make Bazel discard its analysis cache on every query.

set -euo pipefail

workspace="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Cache path keyed on the module files and CPU, so a dependency change or another arch gets its own build.
key="$(cat "${workspace}/MODULE.bazel" "${workspace}/MODULE.bazel.lock" | cksum | cut -d" " -f1)"
driver="${XDG_CACHE_HOME:-${HOME}/.cache}/spacetime/gopackagesdriver/${key}-$(uname -m)/gopackagesdriver"

# First run for this key: build the driver for the host platform and copy it into the cache.
if [[ ! -x "${driver}" ]]; then
  (
    cd "${workspace}"
    label=@rules_go//go/tools/gopackagesdriver
    bazel build --platforms=@platforms//host "${label}"
    built="$(bazel cquery --platforms=@platforms//host --output=files "${label}")"
    mkdir -p "$(dirname "${driver}")"
    tmp="$(mktemp "${driver}.XXXXXX")"
    install -m 0755 "${built}" "${tmp}"
    mv -f "${tmp}" "${driver}"
  ) >&2 # stdout is the driver's reply to gopls.
fi

# Run the cached driver with the variables `bazel run` would have set, so it finds the workspace and resolves relative patterns.
BUILD_WORKSPACE_DIRECTORY="${workspace}" BUILD_WORKING_DIRECTORY="${PWD}" exec "${driver}" "${@}"
