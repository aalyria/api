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
// an external process to generate telemetry reports in JSON form (protojson
// encoding).
package extproc

import (
	"context"
	"fmt"
	"os/exec"

	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/telemetry"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/protojson"
)

type backend struct {
	cmdFn func(context.Context) *exec.Cmd
}

func New(cmdFn func(context.Context) *exec.Cmd) telemetry.Backend {
	return (&backend{cmdFn: cmdFn}).generateReport
}

func (tb *backend) generateReport(ctx context.Context) (*apipb.NetworkStatsReport, error) {
	log := zerolog.Ctx(ctx)

	log.Trace().Msg("running command")
	reportJS, err := tb.cmdFn(ctx).Output()
	if err != nil {
		return nil, fmt.Errorf("error running command: %w", err)
	}

	log.Trace().Msg("unmarshalling network stats report")
	report := apipb.NetworkStatsReport{}
	if err = protojson.Unmarshal(reportJS, &report); err != nil {
		return nil, fmt.Errorf("error unmarshalling command output into report proto: %w", err)
	}
	return &report, nil
}
