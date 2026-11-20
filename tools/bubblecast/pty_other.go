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

//go:build !linux

package bubblecast

import (
	"errors"
	"os"
)

// OpenPTY opens a pseudo-terminal of the given size, returning its
// controlling end, which reads what is drawn, and the terminal end to draw
// on. It is only supported on Linux, and returns an error here.
func OpenPTY(cols, rows int) (ptmx, tty *os.File, err error) {
	return nil, nil, errors.New("bubblecast: pseudo-terminals are only supported on Linux")
}
