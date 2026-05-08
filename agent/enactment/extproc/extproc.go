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

	"aalyria.com/spacetime/agent/enactment"
	"aalyria.com/spacetime/agent/internal/extprocs"
	"aalyria.com/spacetime/agent/internal/protofmt"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"github.com/rs/zerolog"
)

type driver struct {
	args     []string
	protoFmt protofmt.Format
}

func New(args []string, format protofmt.Format) enactment.Driver {
	return &driver{args: args, protoFmt: format}
}

func (ed *driver) Close() error               { return nil }
func (ed *driver) Init(context.Context) error { return nil }
func (ed *driver) Stats() any {
	return struct {
		Type   string
		Args   []string
		Format string
	}{
		Type:   fmt.Sprintf("%T", ed),
		Args:   ed.args,
		Format: ed.protoFmt.String(),
	}
}

func (ed *driver) Dispatch(ctx context.Context, req *schedpb.CreateEntryRequest) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "extproc").Logger()

	js, err := ed.protoFmt.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshalling proto as %s: %w", ed.protoFmt, err)
	}
	log.Trace().Strs("args", ed.args).Msg("running enactment command")
	cmd := exec.CommandContext(ctx, ed.args[0], ed.args[1:]...)
	cmd.Stdin = bytes.NewBuffer(js)

	if err := cmd.Run(); err != nil {
		return extprocs.CommandError(err)
	}
	return nil
}
