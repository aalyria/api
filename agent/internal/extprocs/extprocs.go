// Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package extprocs provides common utilities shared between the extproc
// backends.
package extprocs

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The first invalid codes.Code value. Used to coerce exit codes into
// reasonable gRPC codes. This should match the constants in the gRPC codes
// package:
// https://github.com/grpc/grpc-go/blob/fe39661ffe8a83227c5c40591f335176aa7e5153/codes/codes.go#L195
const maxCode = 17

// CommandError turns an *exec.ExitError into a gRPC-flavored error using the
// command's exit code as the gRPC code (if valid) and the command's stderr (or
// an excerpt from it) as the error message. If the provided error is not an
// *exec.ExitError then the error is wrapped with a ready-to-display message.
func CommandError(err error) error {
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return fmt.Errorf("running command: %w", err)
		}

		c := codes.Code(exitErr.ExitCode())
		if c >= maxCode {
			c = codes.Unknown
		}

		return status.Error(c, strings.TrimSpace(string(exitErr.Stderr)))
	}

	// shouldn't happen
	return nil
}
