// Copyright 2024 Aalyria Technologies, Inc., and its affiliates.
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

package telemetry

import (
	"context"
	"time"

	telemetrypb "aalyria.com/spacetime/telemetry/v1alpha"
	"github.com/jonboulle/clockwork"

	"github.com/rs/zerolog"
)

type ReportGenerator interface {
	Stats() interface{}
	GenerateReport(ctx context.Context, nodeID string) (*telemetrypb.ExportMetricsRequest, error)
}

// PeriodicDriver is a telemetry driver that wraps a ReportGenerator and pushes reports from it at a
// set rate.
type PeriodicDriver struct {
	ReportGenerator
	clock            clockwork.Clock
	collectionPeriod time.Duration
}

func NewPeriodicDriver(generator ReportGenerator, clock clockwork.Clock, collectionPeriod time.Duration) *PeriodicDriver {
	return &PeriodicDriver{
		ReportGenerator:  generator,
		clock:            clock,
		collectionPeriod: collectionPeriod,
	}
}

func (pd *PeriodicDriver) Run(
	ctx context.Context,
	nodeID string,
	reportMetrics func(*telemetrypb.ExportMetricsRequest) error,
) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "extproc").Logger()

	generateAndReport := func() {
		report, err := pd.GenerateReport(ctx, nodeID)
		if err != nil {
			log.Err(err).Msg("failed to generate report")
			return
		}
		if report == nil {
			return
		}
		if err := reportMetrics(report); err != nil {
			log.Err(err).Msg("reporting metrics")
		}
	}
	generateAndReport()

	ticker := pd.clock.NewTicker(pd.collectionPeriod)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return ctx.Err()
		case <-ticker.Chan():
			generateAndReport()
		}
	}
}
