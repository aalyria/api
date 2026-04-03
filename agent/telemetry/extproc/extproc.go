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
	"time"

	"aalyria.com/spacetime/agent/internal/extprocs"
	"aalyria.com/spacetime/agent/internal/loggable"
	"aalyria.com/spacetime/agent/internal/protofmt"
	"aalyria.com/spacetime/agent/telemetry"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
)

var errEmptyReport = errors.New("command generated an empty response")

type reportGenerator struct {
	args     []string
	protoFmt protofmt.Format
}

func NewDriver(args []string, format protofmt.Format, collectionPeriod time.Duration) (telemetry.Driver, error) {
	return telemetry.NewPeriodicDriver(&reportGenerator{
		args:     args,
		protoFmt: format,
	}, clockwork.NewRealClock(), collectionPeriod)
}

func (rg *reportGenerator) Stats() interface{} {
	return struct {
		Type   string
		Args   []string
		Format string
	}{
		Type:   fmt.Sprintf("%T", rg),
		Args:   rg.args,
		Format: rg.protoFmt.String(),
	}
}

func (rg *reportGenerator) GenerateReport(ctx context.Context, nodeID string) (*telemetrypb.ExportMetricsRequest, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "extproc").Logger()

	log.Trace().Strs("args", rg.args).Msg("running telemetry command")
	// nosemgrep: dangerous-exec-command
	cmd := exec.CommandContext(ctx, rg.args[0], rg.args[1:]...)
	reportData, err := cmd.Output()
	if err != nil {
		return nil, extprocs.CommandError(err)
	}

	if len(reportData) == 0 {
		return nil, errEmptyReport
	}

	report := &telemetrypb.ExportMetricsRequest{}
	if err = rg.protoFmt.Unmarshal(reportData, report); err != nil {
		return nil, fmt.Errorf("unmarshalling command output into report proto: %w", err)
	}
	log.Trace().Interface("state", loggable.Proto(report)).Msg("command generated report")
	return report, nil
}
