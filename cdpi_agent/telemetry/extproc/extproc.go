// Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
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

// Package extproc provides a telemetry.Backend implementation that relies on
// an external process to generate telemetry reports in the specified form.
package extproc

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/internal/extprocs"
	"aalyria.com/spacetime/cdpi_agent/internal/loggable"
	"aalyria.com/spacetime/cdpi_agent/internal/protofmt"
	"aalyria.com/spacetime/cdpi_agent/telemetry"

	"github.com/rs/zerolog"
)

var errEmptyReport = errors.New("command generated an empty response")

type backend struct {
	cmdFn    func(ctx context.Context, nodeID string) *exec.Cmd
	protoFmt protofmt.Format
}

func New(cmdFn func(ctx context.Context, nodeID string) *exec.Cmd, format protofmt.Format) telemetry.Backend {
	return (&backend{cmdFn: cmdFn, protoFmt: format}).generateReport
}

func (tb *backend) generateReport(ctx context.Context, nodeID string) (*apipb.NetworkStatsReport, error) {
	log := zerolog.Ctx(ctx).With().Str("backend", "extproc").Logger()

	log.Trace().Msg("running command")
	reportData, err := tb.cmdFn(ctx, nodeID).Output()
	if err != nil {
		return nil, extprocs.CommandError(err)
	}

	if len(reportData) == 0 {
		return nil, errEmptyReport
	}

	report := &apipb.NetworkStatsReport{}
	if err = tb.protoFmt.Unmarshal(reportData, report); err != nil {
		return nil, fmt.Errorf("unmarshalling command output into report proto: %w", err)
	}
	log.Trace().Interface("state", loggable.Proto(report)).Msg("command generated report")
	return report, nil
}
