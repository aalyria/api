// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bubblecast

import (
	"os"
	"path/filepath"
)

// colourEnv lists the variables that, when set, change how programs detect
// the colours a terminal supports.
var colourEnv = []string{"NO_COLOR", "CI", "COLORTERM", "CLICOLOR"}

// SetTerminalEnv sets TERM to term and unsets the variables that override
// colour detection (NO_COLOR, CI, COLORTERM and CLICOLOR), so a program
// recorded in this process, or started from it, picks the colours a real
// terminal of that type would offer whatever terminal, CI job or sandbox
// the recording runs in.
func SetTerminalEnv(term string) error {
	if err := os.Setenv("TERM", term); err != nil {
		return err
	}
	for _, v := range colourEnv {
		if err := os.Unsetenv(v); err != nil {
			return err
		}
	}
	return nil
}

// WorkspacePath resolves a relative path against the workspace the program
// was started from by `bazel run`, which starts it in its runfiles instead.
// Absolute paths, and any path outside `bazel run`, are returned unchanged.
func WorkspacePath(p string) string {
	if ws := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); ws != "" && !filepath.IsAbs(p) {
		return filepath.Join(ws, p)
	}
	return p
}
