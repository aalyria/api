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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sourcegraph/conc/pool"
)

func TestRecorderCutsFramesAtGaps(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r, w := io.Pipe()
		rec := NewRecorder(Options{})
		capture := pool.New()
		capture.Go(func() { rec.Capture(r) })
		// One frame written in two pieces, then a pause, then a second frame.
		io.WriteString(w, "frame one, ")
		io.WriteString(w, "second half")
		time.Sleep(20 * frameGap)
		io.WriteString(w, "frame two")
		w.Close()
		capture.Wait()

		want := []event{
			{t: 0, data: "frame one, second half"},
			{t: 20 * frameGap, data: "frame two"},
		}
		if diff := cmp.Diff(want, rec.events, cmp.AllowUnexported(event{})); diff != "" {
			t.Errorf("events mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestRecorderHoldsAFrameWhileOutputKeepsComing checks that a frame drawn in
// pieces closer together than the gap is recorded as one event, however long
// it takes in all.
func TestRecorderHoldsAFrameWhileOutputKeepsComing(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r, w := io.Pipe()
		rec := NewRecorder(Options{})
		capture := pool.New()
		capture.Go(func() { rec.Capture(r) })
		// Ten pieces each 0.6 gaps apart: the frame spans 5.4 gaps, but no
		// pause between pieces reaches one.
		for i := range 10 {
			if i > 0 {
				time.Sleep(frameGap * 6 / 10)
			}
			io.WriteString(w, "x")
		}
		time.Sleep(20 * frameGap)
		w.Close()
		capture.Wait()

		want := []event{{t: 0, data: "xxxxxxxxxx"}}
		if diff := cmp.Diff(want, rec.events, cmp.AllowUnexported(event{})); diff != "" {
			t.Errorf("events mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestPrintStampsTextWithElapsedTime(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		rec := NewRecorder(Options{})
		time.Sleep(2 * time.Second)
		rec.Print("$ ")
		want := []event{{t: 2 * time.Second, data: "$ "}}
		if diff := cmp.Diff(want, rec.events, cmp.AllowUnexported(event{})); diff != "" {
			t.Errorf("events mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestRecorderRecordsASingleByteFrame checks that a one-byte frame, such as
// an echoed keypress, is recorded on its own.
func TestRecorderRecordsASingleByteFrame(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r, w := io.Pipe()
		rec := NewRecorder(Options{})
		capture := pool.New()
		capture.Go(func() { rec.Capture(r) })
		io.WriteString(w, "a")
		time.Sleep(20 * frameGap)
		io.WriteString(w, "b")
		w.Close()
		capture.Wait()

		want := []event{
			{t: 0, data: "a"},
			{t: 20 * frameGap, data: "b"},
		}
		if diff := cmp.Diff(want, rec.events, cmp.AllowUnexported(event{})); diff != "" {
			t.Errorf("events mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestWriteCast(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		opts   Options
		events []event
		want   string
	}{
		{
			name: "title and env",
			opts: Options{Cols: 100, Rows: 20, Title: "demo <run>", Env: map[string]string{"TERM": "xterm-256color"}},
			events: []event{
				{t: 0, data: "$ demo\r\n"},
				{t: 1234567 * time.Microsecond, data: "\x1b[1mbold\x1b[0m & done"},
			},
			want: `{"version":2,"width":100,"height":20,"title":"demo <run>","env":{"TERM":"xterm-256color"}}
[0,"o","$ demo\r\n"]
[1.235,"o","\u001b[1mbold\u001b[0m & done"]
`,
		},
		{
			name: "defaults",
			want: `{"version":2,"width":80,"height":24}
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := NewRecorder(tc.opts)
			rec.events = tc.events
			var b strings.Builder
			if err := rec.WriteCast(&b); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, b.String()); diff != "" {
				t.Errorf("cast mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestWriteFile(t *testing.T) {
	t.Parallel()
	rec := NewRecorder(Options{Cols: 10, Rows: 2})
	rec.Print("hi")
	rec.Print("")
	path := filepath.Join(t.TempDir(), "out.cast")
	if err := rec.WriteFile(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
	if len(lines) != 2 || lines[0] != `{"version":2,"width":10,"height":2}` || !strings.HasSuffix(lines[1], `,"o","hi"]`) {
		t.Errorf("WriteFile wrote %q, want a header and one event", got)
	}
}
