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
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestRenderArgs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts RenderOptions
		want []string
	}{
		{
			name: "defaults",
			want: []string{"--font-size", "14", "--theme", "github-dark", "--last-frame-duration", "6", "--fps-cap", "10", "--quiet", "in.cast", "out.gif"},
		},
		{
			name: "overridden",
			opts: RenderOptions{FontSize: 20, Theme: "monokai", LastFrameDuration: 1500 * time.Millisecond, FPSCap: 30},
			want: []string{"--font-size", "20", "--theme", "monokai", "--last-frame-duration", "1.5", "--fps-cap", "30", "--quiet", "in.cast", "out.gif"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.opts.withDefaults().args("in.cast", "out.gif")
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("agg args mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRenderDefaultsToAggOnPath(t *testing.T) {
	t.Parallel()
	if got := (RenderOptions{}).withDefaults().Agg; got != "agg" {
		t.Errorf("default Agg = %q, want %q", got, "agg")
	}
}

func TestRenderReportsAggFailure(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "no-such-agg")
	var stderr bytes.Buffer
	err := Render(context.Background(), "in.cast", "out.gif", RenderOptions{Agg: missing, Stderr: &stderr})
	if err == nil || !strings.Contains(err.Error(), "running "+missing) {
		t.Errorf("Render with a missing agg = %v, want an error naming it", err)
	}
}

func TestRenderPassesItsArgsToAgg(t *testing.T) {
	t.Parallel()
	// A stand-in agg that prints the arguments it was given.
	dir := t.TempDir()
	agg := filepath.Join(dir, "agg")
	if err := os.WriteFile(agg, []byte("#!/bin/sh\necho \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Render(context.Background(), "in.cast", "out.gif", RenderOptions{Agg: agg, Stdout: &stdout}); err != nil {
		t.Skipf("can't run a shell script here: %v", err)
	}
	want := "--font-size 14 --theme github-dark --last-frame-duration 6 --fps-cap 10 --quiet in.cast out.gif\n"
	if got := stdout.String(); got != want {
		t.Errorf("agg got %q, want %q", got, want)
	}
}
