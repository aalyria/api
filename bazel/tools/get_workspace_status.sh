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

# Populate Workspace Status variables that can be referred by rules.
# See: https://bazel.build/docs/user-manual#workspace-status
#
# NOTES:
# - rules that depend on "STABLE_" prefixed variables have their cache
#   invalidated every time the variables they depend on change.
#    *  more "deterministic" behavior: if a variable change, all rules
#       that uses it are re-run with the latest value.
#    *  more cache invalidation: every time a variable change, rules
#       depending on it have their cache invalidated and need to be re-run.
#
# - variables that are NOT "STABLE_" prefixed are expected to change frequently
#   and bazel does not invalidate cache rules that depend on them, if the
#   variable value is the only change in the rules input.
#    * less "deterministic" behavior: bazel might re-use a cached output with
#      an old variable value.
#    * less cache invalidation: more performance.
#
# - Please try and keep external process usage to a minimum, even at the cost
#   of a little readability. This script gets executed on effectively every
#   single Bazel invocation, so performance matters a lot.
#
# - Keep the script portable to bash 3.2. The public aalyria/api export runs
#   it too, and macOS ships bash 3.2 as /bin/bash.

# _normalize_git_url lives in a sibling sourceable file so the unit test can
# exercise the real implementation. Bazel runs this script from the workspace
# root (--workspace_status_command), so the sibling resolves via BASH_SOURCE.
# shellcheck source=bazel/tools/git_url.sh
source "$(dirname "${BASH_SOURCE[0]}")/git_url.sh" || {
	echo "get_workspace_status.sh: cannot source git_url.sh" >&2
	exit 1
}

function _print_git_variables() {
	# Instead of calling `git show` multiple times, we ask it to print all the
	# information we want on one line and read it into three variables.
	#
	# %h: the abbreviated hash of the commit
	# %ct: the commit date as a unix timestamp
	# %cs: the commit date in short format (yyyy-mm-dd)
	local sha timestamp date
	read -r sha timestamp date < <( git show -s --format="%h %ct %cs" --abbrev=7 )
	echo STABLE_GIT_SHA "${sha}"
	echo STABLE_GIT_TIMESTAMP "${timestamp}"
	echo STABLE_GIT_DATE "${date}"

	local url
	url=$(git config --get remote.origin.url 2>/dev/null || echo "")
	echo STABLE_GIT_REPO_URL "$(_normalize_git_url "$url")"
}

function _print_spacetime_variables() {
	# The public aalyria/api export has no helm tree; it only needs the
	# version keys below.
	if [[ -f helm/spacetime/variables.bzl ]]; then
		awk < helm/spacetime/variables.bzl '
		$1 == "RELEASE_CHANNEL" && $2 == "=" {
			# When parsing RELEASE_CHANNEL we need to strip the quotes around "latest".
			gsub(/"/, "", $3);
			print "STABLE_SPACETIME_RELEASE_CHANNEL", $3
		}'
	fi
	awk < version.bzl '
	$1 == "MAJOR" && $2 == "=" { major = $3 }
	$1 == "MINOR" && $2 == "=" { minor = $3 }
	$1 == "PATCH" && $2 == "=" { patch = $3 }
	$1 == "BUILD" && $2 == "=" {
		# When parsing BUILD we need to strip the quotes around "sha".
		gsub(/"/, "", $3);
		build = $3
	}
	END {
		print "STABLE_MAJOR", major
		print "STABLE_MINOR", minor
		print "STABLE_PATCH", patch
		if (build == "") {
			print "STABLE_BUILD", "0000"
		} else {
			print "STABLE_BUILD", build
		}
		# SPACETIME_VERSION mirrors VERSION in version.bzl. It is deliberately
		# NOT STABLE_-prefixed: binaries embed it via stamped link actions
		# (rules_go x_defs placeholders, cc linkstamps), and a volatile key
		# lets those actions stay cached when only the CICD-stamped
		# PATCH/BUILD segments changed. A binary therefore reports the
		# version of the last change that actually rebuilt it, and its bytes
		# (and every container image layer holding it) stay identical across
		# unrelated commits.
		version = major "." minor "." patch
		if (build != "") {
			version = version "+" build
		}
		print "SPACETIME_VERSION", version
	}'
}

function _print_build_env_variables() {
	local today
	today=$(date +%Y-%m-%d)
	echo STABLE_BUILD_DATE "${today}"
	# Daily-rotated cache buster for vuln-scan attestations. Same-day rebuilds
	# share the cached scan; date change invalidates so the attestation never
	# claims older-than-24h knowledge.
	echo STABLE_VULN_SCAN_DATE "${today}"

	# for users with underscores due to os-login i.e john_acme_com -> john-acme-com
	# this resolves chart.metadata.version validation error:

	# USER is set by login process
	# LOGNAME set by login to the name of the users account
	local user v
	for v in USER LOGNAME; do
		if [[ -n "${!v:-}" ]]; then
			user="${!v}"
			break
		fi
	done
	if [[ -z "${user:-}" ]]; then
		user=$(whoami)
	fi
	echo STABLE_BUILD_USER_SLUG "${user//_/-}"
}

_print_git_variables
_print_spacetime_variables
_print_build_env_variables
