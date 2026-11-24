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

// Package bubblecast records a program drawing on a terminal as an
// asciicast, and renders the recording as a GIF with agg.
//
// The program draws on a real pseudo-terminal, so it sees a terminal of a
// fixed size and draws exactly as it would for a user: a bubbletea program
// given the terminal as its input and output, or a urfave/cli app with its
// Writer and ErrWriter set to it, needs no changes to be recorded.
//
//	if err := bubblecast.SetTerminalEnv("xterm-256color"); err != nil {
//		return err
//	}
//	rec := bubblecast.NewRecorder(bubblecast.Options{Cols: 100, Rows: 20})
//	rec.Print("$ mytool list\r\n")
//	err := rec.Record(func(tty *os.File) error {
//		app.Writer, app.ErrWriter = tty, tty
//		return app.RunContext(ctx, []string{"mytool", "list"})
//	})
//	...
//	err = rec.WriteFile("list.cast")
//	...
//	err = bubblecast.Render(ctx, "list.cast", "list.gif", bubblecast.RenderOptions{})
package bubblecast // import "aalyria.com/spacetime/tools/bubblecast"

import (
	"os"

	"github.com/sourcegraph/conc/pool"
)

// Record opens a pseudo-terminal of the recording's size, calls draw with
// the terminal end for the program to draw on, and records everything drawn
// until draw returns. It then closes the terminal and returns once the last
// of the output has been recorded, with draw's error.
//
// Anything else holding the terminal open when draw returns, such as a
// goroutine draw started and left running, keeps Record waiting until it
// closes too.
//
// Record leaves the environment alone; call SetTerminalEnv first so the
// program detects the colours a real terminal would offer.
func (r *Recorder) Record(draw func(tty *os.File) error) error {
	ptmx, tty, err := OpenPTY(r.opts.Cols, r.opts.Rows)
	if err != nil {
		return err
	}
	defer ptmx.Close()
	capture := pool.New()
	capture.Go(func() { r.Capture(ptmx) })
	err = draw(tty)
	tty.Close()
	capture.Wait()
	return err
}
