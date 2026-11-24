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
	"encoding/json"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"github.com/sourcegraph/conc/pool"
)

// frameGap is how long the terminal must stay quiet after output for that
// output to count as a whole frame. bubbletea writes each frame in one go, so
// a frame split across reads arrives well within it.
const frameGap = 5 * time.Millisecond

const (
	defaultCols = 80
	defaultRows = 24
)

// Options describes a recording.
type Options struct {
	// Cols and Rows are the size of the terminal, 80x24 if unset.
	Cols, Rows int
	// Title is the recording's title in the asciicast header, if any.
	Title string
	// Env is the environment recorded in the asciicast header, such as
	// TERM, if any. It is only recorded, not applied; see SetTerminalEnv.
	Env map[string]string
}

// Recorder collects terminal output as timed asciicast events. Its methods
// are safe to call concurrently.
type Recorder struct {
	opts  Options
	start time.Time

	mu      sync.Mutex
	events  []event
	pending []byte
	// pendingAt is when the first byte of pending arrived.
	pendingAt time.Duration
}

type event struct {
	t    time.Duration
	data string
}

// NewRecorder returns a Recorder for a recording described by opts, whose
// clock starts now.
func NewRecorder(opts Options) *Recorder {
	if opts.Cols <= 0 {
		opts.Cols = defaultCols
	}
	if opts.Rows <= 0 {
		opts.Rows = defaultRows
	}
	return &Recorder{opts: opts, start: time.Now()}
}

// Print records text as if drawn on the terminal now, such as a shell
// prompt shown before or after the program runs. It bypasses the terminal,
// so line ends must be written as "\r\n".
func (r *Recorder) Print(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s != "" {
		r.events = append(r.events, event{t: time.Since(r.start), data: s})
	}
}

// Capture records everything read from ptmx, the controlling end of a
// pseudo-terminal, until a read fails, as it does once the terminal end is
// closed. Output is held until the terminal has been quiet for a few
// milliseconds, then recorded as one event stamped when it began, so a frame
// split across reads is never shown half drawn.
func (r *Recorder) Capture(ptmx io.Reader) {
	reads := make(chan []byte)
	reader := pool.New()
	defer reader.Wait()
	reader.Go(func() {
		defer close(reads)
		buf := make([]byte, 64<<10)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				reads <- bytes.Clone(buf[:n])
			}
			if err != nil {
				return
			}
		}
	})
	quiet := time.NewTimer(frameGap)
	defer quiet.Stop()
	for {
		select {
		case p, ok := <-reads:
			if !ok {
				r.flush()
				return
			}
			r.feed(p)
			quiet.Reset(frameGap)
		case <-quiet.C:
			r.flush()
		}
	}
}

// feed holds output read from the pseudo-terminal until the frame it belongs
// to is complete.
func (r *Recorder) feed(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) == 0 {
		r.pendingAt = time.Since(r.start)
	}
	r.pending = append(r.pending, p...)
}

// flush records the output held so far as one event.
func (r *Recorder) flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) > 0 {
		r.events = append(r.events, event{t: r.pendingAt, data: string(r.pending)})
		r.pending = nil
	}
}

// WriteCast writes the recording to w as an asciicast v2 file: a JSON
// header line, then one JSON line per event.
func (r *Recorder) WriteCast(w io.Writer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	header := struct {
		Version int               `json:"version"`
		Width   int               `json:"width"`
		Height  int               `json:"height"`
		Title   string            `json:"title,omitempty"`
		Env     map[string]string `json:"env,omitempty"`
	}{
		Version: 2,
		Width:   r.opts.Cols,
		Height:  r.opts.Rows,
		Title:   r.opts.Title,
		Env:     r.opts.Env,
	}
	if err := enc.Encode(header); err != nil {
		return err
	}
	for _, e := range r.events {
		secs := math.Round(e.t.Seconds()*1000) / 1000
		if err := enc.Encode([]any{secs, "o", e.data}); err != nil {
			return err
		}
	}
	return nil
}

// WriteFile writes the recording to the file at path as an asciicast v2
// file, replacing any file already there.
func (r *Recorder) WriteFile(path string) error {
	var b bytes.Buffer
	if err := r.WriteCast(&b); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}
