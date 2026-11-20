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
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// OpenPTY opens a pseudo-terminal of the given size, returning its
// controlling end, which reads what is drawn, and the terminal end to draw
// on. The caller closes both. It is only supported on Linux.
func OpenPTY(cols, rows int) (ptmx, tty *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	// Close the controlling end on every failure below. The error returns
	// clear the named results, so the deferred close must use m.
	defer func() {
		if err != nil {
			m.Close()
		}
	}()
	fd := int(m.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		return nil, nil, fmt.Errorf("unlocking pty: %w", err)
	}
	n, err := unix.IoctlGetUint32(fd, unix.TIOCGPTN)
	if err != nil {
		return nil, nil, fmt.Errorf("getting pty number: %w", err)
	}
	tty, err = os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	ws := &unix.Winsize{Col: uint16(cols), Row: uint16(rows)}
	if err := unix.IoctlSetWinsize(int(tty.Fd()), unix.TIOCSWINSZ, ws); err != nil {
		tty.Close()
		return nil, nil, fmt.Errorf("sizing pty: %w", err)
	}
	return m, tty, nil
}
