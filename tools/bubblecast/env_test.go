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
	"testing"
)

func TestSetTerminalEnv(t *testing.T) {
	for _, v := range []string{"TERM", "NO_COLOR", "CI", "COLORTERM", "CLICOLOR"} {
		t.Setenv(v, "1")
	}
	if err := SetTerminalEnv("xterm-256color"); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("TERM"); got != "xterm-256color" {
		t.Errorf("TERM = %q, want %q", got, "xterm-256color")
	}
	for _, v := range []string{"NO_COLOR", "CI", "COLORTERM", "CLICOLOR"} {
		if got, ok := os.LookupEnv(v); ok {
			t.Errorf("%s = %q, want it unset", v, got)
		}
	}
}

func TestWorkspacePath(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "out.gif")
	for _, tc := range []struct {
		name, workspace, path, want string
	}{
		{name: "relative under bazel run", workspace: "/ws", path: "docs/out.gif", want: filepath.Join("/ws", "docs/out.gif")},
		{name: "absolute under bazel run", workspace: "/ws", path: abs, want: abs},
		{name: "relative outside bazel run", path: "docs/out.gif", want: "docs/out.gif"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BUILD_WORKSPACE_DIRECTORY", tc.workspace)
			if got := WorkspacePath(tc.path); got != tc.want {
				t.Errorf("WorkspacePath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
