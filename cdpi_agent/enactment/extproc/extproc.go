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
	"bytes"
	"context"
	"fmt"
	"os/exec"

	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/enactment"
	"aalyria.com/spacetime/cdpi_agent/internal/extprocs"
	"aalyria.com/spacetime/cdpi_agent/internal/loggable"
	"aalyria.com/spacetime/cdpi_agent/internal/protofmt"

	"github.com/rs/zerolog"
)

type backend struct {
	cmdFn    func(context.Context) *exec.Cmd
	protoFmt protofmt.Format
}

func New(cmdFn func(context.Context) *exec.Cmd, format protofmt.Format) enactment.Backend {
	return (&backend{cmdFn: cmdFn, protoFmt: format}).handleRequest
}

func (eb *backend) handleRequest(ctx context.Context, req *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
	log := zerolog.Ctx(ctx).With().Str("backend", "extproc").Logger()

	js, err := eb.protoFmt.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshalling proto as %s: %w", eb.protoFmt, err)
	}

	stdinBuf := bytes.NewBuffer(js)
	stdoutBuf := bytes.NewBuffer(nil)

	cmd := eb.cmdFn(ctx)
	cmd.Stdin = stdinBuf

	log.Trace().Msg("starting command")
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting command: %w", err)
	}

	log.Trace().Msg("waiting for command")
	if err = cmd.Wait(); err != nil {
		return nil, extprocs.CommandError(err)
	}

	reportData := stdoutBuf.Bytes()
	if len(reportData) == 0 {
		log.Trace().Msg("command exited with no new status")
		return nil, nil
	}

	log.Trace().Int("bytes", len(reportData)).Msg("unmarshalling control plane state")
	stateMsg := &apipb.ControlPlaneState{}
	if err = eb.protoFmt.Unmarshal(reportData, stateMsg); err != nil {
		return nil, fmt.Errorf("marshalling command output into state proto: %w", err)
	}
	log.Trace().Interface("state", loggable.Proto(stateMsg)).Msg("command returned new state")
	return stateMsg, nil
}
