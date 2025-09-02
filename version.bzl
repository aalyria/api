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

"Release Version Number"

# Increment when making breaking changes.
# -------------------------------------------------------
# - Incompatible changes to APIs that can break depending projects code or expectations.
# - Incompatible changes in how data is written or read from config files or DBs.
# - Changes requiring manual enviroment adjustments before the new version can be used.
MAJOR = 18

# Increment after releasing a public version.
# -------------------------------------------------------------
# This allows to publish bugfixes to the released version
# through increments of PATCH segment.
MINOR = 6

# Monotonic ID of bugfixes applied to each MAJOR.MINOR release.
# -------------------------------------------------------------
# Line updated by CICD automation before bazel execution.
PATCH = 0

# Build metadata. Ignored when determining version precedence.
# -------------------------------------------------------------
# Line updated by CICD automation before bazel execution.
BUILD = ""

# Version uses Semantic Versioning 2.0.0 format. See: https://semver.org
VERSION = "{0}.{1}.{2}+{3}".format(MAJOR, MINOR, PATCH, BUILD) if len(BUILD) > 0 else "{0}.{1}.{2}".format(MAJOR, MINOR, PATCH)
