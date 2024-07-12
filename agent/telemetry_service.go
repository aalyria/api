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
	"fmt"

	afpb "aalyria.com/spacetime/api/cdpi/v1alpha"
	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/agent/internal/channels"
	"aalyria.com/spacetime/agent/internal/loggable"
	"aalyria.com/spacetime/agent/internal/task"
	"aalyria.com/spacetime/agent/telemetry"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

type telemetryService struct {
	nodeID          string
	nc              *nodeController
	telemetryClient afpb.NetworkTelemetryStreamingClient
	td              telemetry.Driver
}

func (nc *nodeController) newTelemetryService(tc afpb.NetworkTelemetryStreamingClient, tb telemetry.Driver) *telemetryService {
	return &telemetryService{
		nodeID:          nc.id,
		nc:              nc,
		telemetryClient: tc,
		td:              tb,
	}
}

func (ts *telemetryService) Stats() interface{} { return ts.td.Stats() }

func (ts *telemetryService) run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	if err := ts.td.Init(ctx); err != nil {
		return fmt.Errorf("%T.Init() failed: %w", ts.td, err)
	}

	ti, err := ts.telemetryClient.TelemetryInterface(ctx)
	if err != nil {
		return err
	}

	// sendCh contains the messages queued to send over the gRPC stream
	sendCh := make(chan *afpb.TelemetryUpdate)
	// recvCh contains the messages received over the gRPC stream
	recvCh := make(chan *afpb.TelemetryRequest)
	// triggerCh contains empty messages that trigger sending a TelemetryUpdate
	// via the sendCh.
	triggerCh := make(chan struct{})

	// Something like this:
	//
	// ┌──────┐        ┌────────┐┌────────┐       ┌──────┐
	// │recvCh│        │mainLoop││statLoop│       │sendCh│
	// └──┬───┘        └───┬────┘└───┬────┘       └──┬───┘
	//    │                │         │               │
	//    │TelemetryRequest│         │               │
	//    │───────────────>│         │               │
	//    │                │         │               │
	//    │                │struct{} │               │
	//    │                │────────>│               │
	//    │                │         │               │
	//    │                │         │TelemetryUpdate│
	//    │                │         │──────────────>│
	// ┌──┴───┐        ┌───┴────┐┌───┴────┐       ┌──┴───┐
	// │recvCh│        │mainLoop││statLoop│       │sendCh│
	// └──────┘        └────────┘└────────┘       └──────┘

	g.Go(channels.NewSource(sendCh).ForwardTo(ti.Send).
		WithStartingStoppingLogs("sendLoop", zerolog.TraceLevel).
		WithLogField("task", "send").
		WithNewSpan("sendLoop").
		WithPanicCatcher().
		WithCtx(ctx))

	g.Go(channels.NewSink(recvCh).FillFrom(ti.Recv).
		WithStartingStoppingLogs("recvLoop", zerolog.TraceLevel).
		WithLogField("task", "recv").
		WithNewSpan("recvLoop").
		WithPanicCatcher().
		WithCtx(ctx))

	g.Go(ts.statLoop(triggerCh, sendCh).
		WithStartingStoppingLogs("statLoop", zerolog.TraceLevel).
		WithLogField("task", "stat").
		WithNewSpan("statLoop").
		WithPanicCatcher().
		WithCtx(ctx))

	g.Go(ts.mainLoop(triggerCh, recvCh).
		WithStartingStoppingLogs("mainLoop", zerolog.TraceLevel).
		WithLogField("task", "main").
		WithNewSpan("mainLoop").
		WithPanicCatcher().
		WithCtx(ctx))

	select {
	case triggerCh <- struct{}{}:
	case <-ctx.Done():
	}

	return g.Wait()
}

// statLoop reads values from `triggerCh`, generates telemetry updates, then
// forwards them to the `sendCh`.
func (ts *telemetryService) statLoop(triggerCh <-chan struct{}, sendCh chan<- *afpb.TelemetryUpdate) task.Task {
	mapFn := func(ctx context.Context, _ struct{}) (*afpb.TelemetryUpdate, error) {
		var report *apipb.NetworkStatsReport
		if err := task.Task(func(ctx context.Context) (err error) {
			report, err = ts.td.GenerateReport(ctx, ts.nodeID)
			return err
		}).WithNewSpan("telemetry.Driver")(ctx); err != nil {
			return nil, err
		}
		zerolog.Ctx(ctx).Trace().
			Object("report", loggable.Proto(report)).
			Msg("generated stats report")

		return &afpb.TelemetryUpdate{NodeId: &ts.nodeID, Type: &afpb.TelemetryUpdate_Statistics{Statistics: report}}, nil
	}

	return channels.MapBetween(triggerCh, sendCh, mapFn)
}

// mainLoop processes incoming telemetry requests and manages triggering stats
// generation for both periodic and one-off requests.
func (ts *telemetryService) mainLoop(triggerCh chan<- struct{}, recvCh <-chan *afpb.TelemetryRequest) task.Task {
	return func(ctx context.Context) error {
		log := zerolog.Ctx(ctx)
		ticker := newReusableTicker(ts.nc.clock)

		span := oteltrace.SpanFromContext(ctx)

		for {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)

			case <-ticker.Chan():
				span.AddEvent("periodic telemetry report upload triggered")
				select {
				case triggerCh <- struct{}{}:
				case <-ctx.Done():
					return context.Cause(ctx)
				}

			case req := <-recvCh:
				if req.GetNodeId() != ts.nc.id {
					log.Warn().
						Str("requestedID", req.GetNodeId()).
						Object("req", loggable.Proto(req)).
						Msg("ignoring request with mismatched node ID")
					span.AddEvent(
						"ignoring request with mismatched node ID",
						oteltrace.WithAttributes(attribute.String("aalyria.requestedID", req.GetNodeId())))
					continue
				}

				switch req.Type.(type) {
				case *afpb.TelemetryRequest_QueryStatistics:
					span.AddEvent("received request to trigger one-off report generation")
					select {
					case triggerCh <- struct{}{}:
					case <-ctx.Done():
						return context.Cause(ctx)
					}

				case *afpb.TelemetryRequest_StatisticsPublishRateHz:
					newHz := req.GetStatisticsPublishRateHz()
					span.AddEvent(
						"received request to update periodic publish rate",
						oteltrace.WithAttributes(attribute.Float64("aalyria.hz", newHz)))

					if newHz > 0 {
						dur := hzToDuration(newHz)
						log.Trace().
							Float64("hz", newHz).
							Dur("dur", dur).
							Msg("updating publish rate")
						ticker.Start(dur)
					} else {
						log.Trace().Msg("disabling periodic report publishing")
						ticker.Stop()
					}

				default:
					log.Warn().
						Object("req", loggable.Proto(req)).
						Msg("unknown telemetry request type received")
					span.AddEvent("unknown telemetry request type received")
				}
			}
		}
	}
}
