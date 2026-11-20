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
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// skipWithoutPTY skips t where pseudo-terminals can't be opened, as in some
// sandboxes.
func skipWithoutPTY(t *testing.T) {
	t.Helper()
	ptmx, tty, err := OpenPTY(10, 2)
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	ptmx.Close()
	tty.Close()
}

func TestOpenPTYSizesTheTerminal(t *testing.T) {
	t.Parallel()
	skipWithoutPTY(t)
	ptmx, tty, err := OpenPTY(123, 45)
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()
	defer tty.Close()
	ws, err := unix.IoctlGetWinsize(int(tty.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Col != 123 || ws.Row != 45 {
		t.Errorf("terminal is %dx%d, want 123x45", ws.Col, ws.Row)
	}
}

func TestRecordCapturesWhatDrawWritesAndReturnsItsError(t *testing.T) {
	t.Parallel()
	skipWithoutPTY(t)
	rec := NewRecorder(Options{Cols: 40, Rows: 5})
	errDraw := errors.New("draw failed")
	err := rec.Record(func(tty *os.File) error {
		if _, err := io.WriteString(tty, "hello"); err != nil {
			return err
		}
		return errDraw
	})
	if !errors.Is(err, errDraw) {
		t.Errorf("Record = %v, want draw's error %v", err, errDraw)
	}
	var got strings.Builder
	for _, e := range rec.events {
		got.WriteString(e.data)
	}
	if !strings.Contains(got.String(), "hello") {
		t.Errorf("Record recorded %q, want it to contain %q", got.String(), "hello")
	}
}

func TestRecordGivesDrawATerminalOfTheRecordingsSize(t *testing.T) {
	t.Parallel()
	skipWithoutPTY(t)
	rec := NewRecorder(Options{Cols: 77, Rows: 9})
	var cols, rows uint16
	if err := rec.Record(func(tty *os.File) error {
		ws, err := unix.IoctlGetWinsize(int(tty.Fd()), unix.TIOCGWINSZ)
		if err != nil {
			return err
		}
		cols, rows = ws.Col, ws.Row
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if cols != 77 || rows != 9 {
		t.Errorf("draw got a %dx%d terminal, want 77x9", cols, rows)
	}
}
