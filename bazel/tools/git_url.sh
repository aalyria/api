#!/usr/bin/env bash
#
# Copyright 2026 Aalyria Technologies, Inc., and its affiliates.
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

# Git URL helpers, kept in a standalone sourceable file (no side effects on
# source) so get_workspace_status.sh and its unit test can both source the real
# implementation instead of copying or re-extracting it.

# Normalize a git remote URL to its https form, dropping any trailing ".git".
# SSH URLs (git@example.com:org/repo.git) become https form
# (https://example.com/org/repo). Any userinfo in a URL with a scheme is
# dropped: GitLab CI clones from https://gitlab-ci-token:<token>@host/..., and
# the token must not reach the stamped STABLE_GIT_REPO_URL. Other forms (https,
# empty) are returned unchanged apart from the ".git" strip.
function _normalize_git_url() {
	local url="$1"
	local userinfo_re='^([A-Za-z][A-Za-z0-9+.-]*://)[^/@]*@(.*)$'
	url="${url%.git}"
	if [[ "$url" =~ ^git@([^:]+):(.+)$ ]]; then
		url="https://${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
	elif [[ "$url" =~ $userinfo_re ]]; then
		url="${BASH_REMATCH[1]}${BASH_REMATCH[2]}"
	fi
	echo "$url"
}
