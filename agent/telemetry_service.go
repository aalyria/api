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

package agent

import (
	"context"

	"aalyria.com/spacetime/agent/telemetry"
	telemetrypb "aalyria.com/spacetime/telemetry/v1alpha"
)

type telemetryService struct {
	nodeID          string
	telemetryClient telemetrypb.TelemetryClient
	td              telemetry.Driver
}

func (nc *nodeController) newTelemetryService(tc telemetrypb.TelemetryClient, td telemetry.Driver) *telemetryService {
	return &telemetryService{
		nodeID:          nc.id,
		telemetryClient: tc,
		td:              td,
	}
}

func (ts *telemetryService) Stats() interface{} { return ts.td.Stats() }

func (ts *telemetryService) run(ctx context.Context) error {
	reportMetrics := func(report *telemetrypb.ExportMetricsRequest) error {
		_, err := ts.telemetryClient.ExportMetrics(ctx, report)
		return err
	}

	return ts.td.Run(ctx, ts.nodeID, reportMetrics)
}
