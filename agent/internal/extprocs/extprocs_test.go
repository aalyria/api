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

package extprocs

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcError interface {
	GRPCStatus() *status.Status
}

func TestCommandError_compliantCommand(t *testing.T) {
	_, err := exec.Command("/bin/sh", "-c", `echo >&2 "my error message"; exit 3;`).Output()
	cmdErr := CommandError(err)
	se, ok := cmdErr.(grpcError)
	if !ok {
		t.Errorf("expected CommandError to convert %v into a gRPC error, but got %v", err, cmdErr)
	}

	st := se.GRPCStatus()
	if want := codes.InvalidArgument; st.Code() != want {
		t.Errorf("expected CommandError to have a code of %v, but got %v", want, st.Code())
	}
	if want := "my error message"; st.Message() != want {
		t.Errorf("expected CommandError to have a message of %q, but got %q", want, st.Message())
	}
}

func TestCommandError_unknownError(t *testing.T) {
	_, err := exec.Command("/bin/sh", "-c", fmt.Sprintf(`echo >&2 "some error message"; exit %d;`, maxCode+1)).Output()
	cmdErr := CommandError(err)
	se, ok := cmdErr.(grpcError)
	if !ok {
		t.Errorf("expected CommandError to convert %v into a gRPC error, but got %v", err, cmdErr)
	}

	st := se.GRPCStatus()
	if want := codes.Unknown; st.Code() != want {
		t.Errorf("expected CommandError to have a code of %v, but got %v", want, st.Code())
	}
	if want := "some error message"; st.Message() != want {
		t.Errorf("expected CommandError to have a message of %q, but got %q", want, st.Message())
	}
}

func TestCommandError_notAnExitError(t *testing.T) {
	err := errors.New("some non-exit error")
	cmdErr := CommandError(err)
	_, ok := cmdErr.(grpcError)
	if ok {
		t.Errorf("expected CommandError NOT to convert %v into a gRPC error, but got %v", err, cmdErr)
	}
	if want := "running command: some non-exit error"; cmdErr.Error() != want {
		t.Errorf("expected CommandError to return %q, but got %q", want, cmdErr.Error())
	}
}
