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

// Package extproc provides an enactment.Backend implementation that relies on
// an external process to enact changes.
//
// It works by piping incoming change requests to an external process and
// reading the new state from the process's stdout. Requests are
// single-threaded per-node, but if the backend is registered for multiple
// nodes there may be multiple processes invoked in parallel. If the process
// returns with a non-zero exit code within the range of GRPC status codes
// (https://pkg.go.dev/google.golang.org/grpc/codes#Code) then the appropriate
// status will be returned, otherwise the errors will be translated into a
// generic Unknown status (Code = 2).
package extproc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/enactment"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

// The first invalid codes.Code value. Used to coerce exit codes into
// reasonable gRPC codes. This should match the constants in the gRPC codes
// package:
// https://github.com/grpc/grpc-go/blob/fe39661ffe8a83227c5c40591f335176aa7e5153/codes/codes.go#L195
const maxCode = 17

type backend struct {
	cmdFn func(context.Context) *exec.Cmd
}

func New(cmdFn func(context.Context) *exec.Cmd) enactment.Backend {
	return (&backend{cmdFn: cmdFn}).handleRequest
}

func (eb *backend) handleRequest(ctx context.Context, req *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
	log := zerolog.Ctx(ctx)

	log.Trace().Msg("extproc: HandleRequest called")
	defer func() { log.Trace().Msg("extproc: HandleRequest finished") }()

	cmd := eb.cmdFn(ctx)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("error connecting to command's stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("error connecting to command's stdout: %w", err)
	}

	log.Trace().Msg("extproc: marshalling json")
	js, err := protojson.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshalling proto as JSON: %w", err)
	}

	log.Trace().Msg("extproc: starting command")
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("error starting command: %w", err)
	}

	log.Trace().Msg("extproc: writing to stdin")
	if _, err = stdin.Write(js); err != nil {
		return nil, fmt.Errorf("error writing JSON to stdin: %w", err)
	}
	if err = stdin.Close(); err != nil {
		log.Error().Err(err).Msg("extproc: error closing stdin")
		// not a fatal error, so we can continue
	}

	log.Trace().Msg("extproc: reading stdout")
	stateJS, err := io.ReadAll(stdout)
	if err != nil {
		return nil, fmt.Errorf("error reading command output: %w", err)
	}

	log.Trace().Msg("extproc: waiting for command")
	if err = cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			c := codes.Code(exitErr.ExitCode())
			if c >= maxCode {
				c = codes.Unknown
			}
			return nil, status.Error(c, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("unknown error running command: %w", err)
	}

	log.Trace().Msg("extproc: unmarshalling control plane state")
	stateMsg := apipb.ControlPlaneState{}
	if err = protojson.Unmarshal(stateJS, &stateMsg); err != nil {
		return nil, fmt.Errorf("error marshalling command output into state proto: %w", err)
	}
	return &stateMsg, nil
}
