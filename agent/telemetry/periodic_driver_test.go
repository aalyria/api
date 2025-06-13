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
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	telemetrypb "aalyria.com/spacetime/telemetry/v1alpha"

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type constantGenerator struct {
	clock clockwork.Clock
}

func (cg *constantGenerator) Stats() interface{} { return nil }
func (cg *constantGenerator) GenerateReport(ctx context.Context, nodeID string) (*telemetrypb.ExportMetricsRequest, error) {
	return &telemetrypb.ExportMetricsRequest{
		ModemMetrics: []*telemetrypb.ModemMetrics{{
			DataRateDataPoints: []*telemetrypb.DataRateDataPoint{
				{
					Time: timestamppb.New(cg.clock.Now()),
				},
			},
		}},
	}, nil
}

func TestPeriodicDriver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clock := clockwork.NewFakeClock()
	collectionPeriod := 1 * time.Second
	driver, err := NewPeriodicDriver(&constantGenerator{clock: clock}, clock, collectionPeriod)
	if err != nil {
		t.Fatalf("NewPeriodicDriver: %v", err)
	}

	reportedStats := make(chan *telemetrypb.ExportMetricsRequest)
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return driver.Run(ctx, "node ID doesn't matter", func(gotReport *telemetrypb.ExportMetricsRequest) error {
			reportedStats <- gotReport
			return nil
		})
	})

	// First stats are reported immediately
	firstStats := <-reportedStats
	if diff := cmp.Diff(&telemetrypb.ExportMetricsRequest{
		ModemMetrics: []*telemetrypb.ModemMetrics{{
			DataRateDataPoints: []*telemetrypb.DataRateDataPoint{
				{
					Time: timestamppb.New(clock.Now()),
				},
			},
		}},
	}, firstStats, protocmp.Transform()); diff != "" {
		t.Fatalf("unexpected stats (-want +got):\n%s", diff)
	}

	// Next stats aren't reported until the collection period passes
	clock.BlockUntil(1)
	clock.Advance(collectionPeriod - 1*time.Nanosecond)
	if len(reportedStats) > 0 {
		t.Fatalf("new stats reported before collection period passed")
	}

	clock.Advance(1 * time.Nanosecond)
	secondStats := <-reportedStats
	if diff := cmp.Diff(&telemetrypb.ExportMetricsRequest{
		ModemMetrics: []*telemetrypb.ModemMetrics{{
			DataRateDataPoints: []*telemetrypb.DataRateDataPoint{
				{
					Time: timestamppb.New(clock.Now()),
				},
			},
		}},
	}, secondStats, protocmp.Transform()); diff != "" {
		t.Fatalf("unexpected stats (-want +got):\n%s", diff)
	}

	// Keeps reporting indefinitely...
	clock.BlockUntil(1)
	clock.Advance(collectionPeriod)
	<-reportedStats

	clock.BlockUntil(1)
	clock.Advance(collectionPeriod)
	<-reportedStats

	clock.BlockUntil(1)
	clock.Advance(collectionPeriod)
	<-reportedStats

	cancel()
	g.Wait()
}
