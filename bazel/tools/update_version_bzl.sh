#!/bin/bash
#
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

set -euo pipefail

# Check arguments
if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <versions_bzl_file_path>"
  exit 1
fi

versions_bzl_file_path=$1

# Instead of calling `git show` multiple times, we ask it to print all the
# information we want as null-separated values and then read it into the
# `git_data` array.
#
# git_data[0] - %h: the abbreviated hash of the commit
# git_data[1] - %ct: the commit date as a unix timestamp
# git_data[2] - %cs: the commit date in short format (yyyy-mm-dd)
declare -a git_data
readarray -d '' git_data < <( git show -s --format="%h%x00%ct%x00%cs%x00" --abbrev=7 )

if ! grep -q "^PATCH = " "$versions_bzl_file_path"; then
  echo "Error: Line starting with 'PATCH = ' not found in $versions_bzl_file_path."
  exit 1
fi

if ! grep -q "^BUILD = " "$versions_bzl_file_path"; then
  echo "Error: Line starting with 'BUILD = ' not found in $versions_bzl_file_path."
  exit 1
fi

sed -i "/^PATCH = /c\\
PATCH = ${git_data[1]}" "$versions_bzl_file_path"

sed -i "/^BUILD = /c\\
BUILD = \"${git_data[0]}\"" "$versions_bzl_file_path"

#FIXME