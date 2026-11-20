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
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// RenderOptions configures how Render draws a GIF. Unset fields take the
// defaults given.
type RenderOptions struct {
	// Agg is the agg binary, looked up on PATH unless it is a path.
	// Default "agg".
	Agg string
	// FontSize is the font size in pixels. Default 14.
	FontSize int
	// Theme is the agg colour theme. Default "github-dark".
	Theme string
	// LastFrameDuration is how long the last frame stays on screen.
	// Default 6s.
	LastFrameDuration time.Duration
	// FPSCap caps the GIF's frame rate. Default 10.
	FPSCap int
	// Stdout and Stderr receive agg's output. Default os.Stdout and
	// os.Stderr.
	Stdout, Stderr io.Writer
}

func (o RenderOptions) withDefaults() RenderOptions {
	if o.Agg == "" {
		o.Agg = "agg"
	}
	if o.FontSize <= 0 {
		o.FontSize = 14
	}
	if o.Theme == "" {
		o.Theme = "github-dark"
	}
	if o.LastFrameDuration <= 0 {
		o.LastFrameDuration = 6 * time.Second
	}
	if o.FPSCap <= 0 {
		o.FPSCap = 10
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	return o
}

// args returns the arguments to pass agg to render castPath as gifPath.
func (o RenderOptions) args(castPath, gifPath string) []string {
	return []string{
		"--font-size", strconv.Itoa(o.FontSize),
		"--theme", o.Theme,
		"--last-frame-duration", strconv.FormatFloat(o.LastFrameDuration.Seconds(), 'f', -1, 64),
		"--fps-cap", strconv.Itoa(o.FPSCap),
		"--quiet",
		castPath, gifPath,
	}
}

// Render draws the asciicast at castPath as a GIF at gifPath by running agg
// (https://github.com/asciinema/agg), which must be installed.
func Render(ctx context.Context, castPath, gifPath string, opts RenderOptions) error {
	opts = opts.withDefaults()
	cmd := exec.CommandContext(ctx, opts.Agg, opts.args(castPath, gifPath)...)
	cmd.Stdout, cmd.Stderr = opts.Stdout, opts.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running %s: %w", opts.Agg, err)
	}
	return nil
}
