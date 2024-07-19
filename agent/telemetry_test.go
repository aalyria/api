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
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	afpb "aalyria.com/spacetime/api/cdpi/v1alpha"
	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/agent/internal/channels"

	"github.com/jonboulle/clockwork"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

type CannedReportBackend struct {
	fn func(context.Context, string) (*apipb.NetworkStatsReport, error)
}

func (c *CannedReportBackend) GenerateReport(ctx context.Context, nodeID string) (*apipb.NetworkStatsReport, error) {
	return c.fn(ctx, nodeID)
}

func (c *CannedReportBackend) Init(ctx context.Context) error { return nil }
func (c *CannedReportBackend) Close() error                   { return nil }
func (c *CannedReportBackend) Stats() interface{}             { return nil }

func TestStreamStartsWithInitialReport(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(baseContext(t), time.Second)
	defer cancel()

	ts := NewTelemetryServer()
	servedReport := &apipb.NetworkStatsReport{
		NodeId: proto.String("foobar"),
		InterfaceStatsById: map[string]*apipb.InterfaceStats{
			"lo0": {
				TxBytes: proto.Int64(1),
				RxBytes: proto.Int64(12),
			},
		},
	}
	tb := &CannedReportBackend{fn: func(ctx context.Context, nodeID string) (*apipb.NetworkStatsReport, error) {
		return servedReport, nil
	}}

	srvAddr := ts.Start(ctx, t)
	a := newAgent(t,
		WithClock(clockwork.NewFakeClock()),
		WithNode("mynode", WithTelemetryDriver(srvAddr, tb, grpc.WithTransportCredentials(insecure.NewCredentials()))))
	errCh := make(chan error)
	go func() { errCh <- a.Run(ctx) }()

	gotReport := <-ts.inChan
	assertProtosEqual(t, &afpb.TelemetryUpdate{
		NodeId: proto.String("mynode"),
		Type: &afpb.TelemetryUpdate_Statistics{
			Statistics: servedReport,
		},
	}, gotReport)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestBackendReceivesNodeID(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(baseContext(t), time.Second)
	defer cancel()

	wantNodeID := "foobar"

	ts := NewTelemetryServer()
	servedReport := &apipb.NetworkStatsReport{
		NodeId: proto.String(wantNodeID),
		InterfaceStatsById: map[string]*apipb.InterfaceStats{
			"lo0": {
				TxBytes: proto.Int64(1),
				RxBytes: proto.Int64(12),
			},
		},
	}
	tb := &CannedReportBackend{fn: func(ctx context.Context, nodeID string) (*apipb.NetworkStatsReport, error) {
		if nodeID != wantNodeID {
			t.Errorf("Unexpected nodeID. Want %s got %s", wantNodeID, nodeID)
		}
		return servedReport, nil
	}}

	srvAddr := ts.Start(ctx, t)
	a := newAgent(t,
		WithClock(clockwork.NewFakeClock()),
		WithNode(wantNodeID, WithTelemetryDriver(srvAddr, tb, grpc.WithTransportCredentials(insecure.NewCredentials()))))
	errCh := make(chan error)
	go func() { errCh <- a.Run(ctx) }()

	gotReport := <-ts.inChan
	assertProtosEqual(t, &afpb.TelemetryUpdate{
		NodeId: proto.String("foobar"),
		Type: &afpb.TelemetryUpdate_Statistics{
			Statistics: servedReport,
		},
	}, gotReport)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestCanRequestOneOffReport(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(baseContext(t), time.Second)
	defer cancel()

	ts := NewTelemetryServer()
	servedReport := &apipb.NetworkStatsReport{
		NodeId: proto.String("foobar"),
		InterfaceStatsById: map[string]*apipb.InterfaceStats{
			"lo0": {
				TxBytes: proto.Int64(1),
				RxBytes: proto.Int64(12),
			},
		},
	}
	tb := &CannedReportBackend{fn: func(ctx context.Context, nodeID string) (*apipb.NetworkStatsReport, error) {
		return servedReport, nil
	}}

	srvAddr := ts.Start(ctx, t)
	a := newAgent(t,
		WithClock(clockwork.NewFakeClock()),
		WithNode("mynode", WithTelemetryDriver(srvAddr, tb, grpc.WithTransportCredentials(insecure.NewCredentials()))))
	errCh := make(chan error)
	go func() { errCh <- a.Run(ctx) }()

	select {
	case <-ts.inChan:
	case <-ctx.Done():
		t.Errorf("timed out waiting for initial report")
	}

	ts.outChan <- &afpb.TelemetryRequest{
		NodeId: proto.String("mynode"),
		Type:   &afpb.TelemetryRequest_QueryStatistics{},
	}
	var second *afpb.TelemetryUpdate
	select {
	case second = <-ts.inChan:
	case <-ctx.Done():
		t.Errorf("timed out waiting for second report")
	}

	assertProtosEqual(t, &afpb.TelemetryUpdate{
		NodeId: proto.String("mynode"),
		Type: &afpb.TelemetryUpdate_Statistics{
			Statistics: servedReport,
		},
	}, second)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestIgnoresUnknownRequestType(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(baseContext(t), time.Second)
	defer cancel()

	ts := NewTelemetryServer()
	servedReport := &apipb.NetworkStatsReport{
		NodeId: proto.String("foobar"),
		InterfaceStatsById: map[string]*apipb.InterfaceStats{
			"lo0": {
				TxBytes: proto.Int64(1),
				RxBytes: proto.Int64(12),
			},
		},
	}
	tb := &CannedReportBackend{fn: func(ctx context.Context, nodeID string) (*apipb.NetworkStatsReport, error) {
		return servedReport, nil
	}}

	srvAddr := ts.Start(ctx, t)
	a := newAgent(t,
		WithClock(clockwork.NewFakeClock()),
		WithNode("mynode", WithTelemetryDriver(srvAddr, tb, grpc.WithTransportCredentials(insecure.NewCredentials()))))
	errCh := make(chan error)
	go func() { errCh <- a.Run(ctx) }()

	select {
	case <-ts.inChan:
	case <-ctx.Done():
		t.Errorf("timed out waiting for initial report")
	}

	ts.outChan <- &afpb.TelemetryRequest{
		NodeId: proto.String("mynode"),
		// unknown type
		Type: nil,
	}

	timer := time.NewTimer(100 * time.Millisecond)
	select {
	case <-ts.inChan:
		t.Errorf("got unexpected second report")
		if !timer.Stop() {
			<-timer.C
		}
	case <-timer.C:
	}

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestPeriodicUpdates(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(baseContext(t), time.Second)
	defer cancel()

	ts := NewTelemetryServer()
	initialReport := &apipb.NetworkStatsReport{
		NodeId: proto.String("mynode"),
		InterfaceStatsById: map[string]*apipb.InterfaceStats{
			"lo0": {
				TxBytes: proto.Int64(1),
				RxBytes: proto.Int64(12),
			},
		},
	}
	periodicReport := &apipb.NetworkStatsReport{
		NodeId: proto.String("mynode"),
		InterfaceStatsById: map[string]*apipb.InterfaceStats{
			"lo0": {
				TxBytes: proto.Int64(3),
				RxBytes: proto.Int64(15),
			},
		},
	}

	reportCh := make(chan *apipb.NetworkStatsReport, 3)
	reportCh <- initialReport
	reportCh <- periodicReport
	reportCh <- periodicReport

	tb := &CannedReportBackend{fn: func(ctx context.Context, nodeID string) (*apipb.NetworkStatsReport, error) { return <-reportCh, nil }}

	clock := clockwork.NewFakeClock()
	srvAddr := ts.Start(ctx, t)
	a := newAgent(t,
		WithClock(clock),
		WithNode("mynode", WithTelemetryDriver(srvAddr, tb, grpc.WithTransportCredentials(insecure.NewCredentials()))))

	errCh := make(chan error)
	go func() { errCh <- a.Run(ctx) }()

	gotFirst := <-ts.inChan

	// request one update per second
	ts.outChan <- &afpb.TelemetryRequest{
		NodeId: proto.String("mynode"),
		Type: &afpb.TelemetryRequest_StatisticsPublishRateHz{
			StatisticsPublishRateHz: *proto.Float64(1),
		},
	}

	clock.BlockUntil(1)

	// check there's no second update yet (clock hasn't advanced)
	select {
	case r := <-ts.inChan:
		t.Errorf("got unexpected report even though clock is frozen: %#v", r)
		return
	default:
	}

	clock.Advance(1 * time.Second)

	var gotSecond *afpb.TelemetryUpdate
	select {
	case gotSecond = <-ts.inChan:
	case <-ctx.Done():
		t.Errorf("timed out waiting for second periodic report")
	}
	clock.Advance(1 * time.Second)
	var gotThird *afpb.TelemetryUpdate
	select {
	case gotThird = <-ts.inChan:
	case <-ctx.Done():
		t.Errorf("timed out waiting for third periodic report")
	}

	assertProtosEqual(t, &afpb.TelemetryUpdate{
		NodeId: proto.String("mynode"),
		Type:   &afpb.TelemetryUpdate_Statistics{Statistics: initialReport},
	}, gotFirst)
	assertProtosEqual(t, &afpb.TelemetryUpdate{
		NodeId: proto.String("mynode"),
		Type:   &afpb.TelemetryUpdate_Statistics{Statistics: periodicReport},
	}, gotSecond)
	assertProtosEqual(t, &afpb.TelemetryUpdate{
		NodeId: proto.String("mynode"),
		Type:   &afpb.TelemetryUpdate_Statistics{Statistics: periodicReport},
	}, gotThird)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestInitialReportFailsToGenerate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(baseContext(t), time.Second)
	defer cancel()

	ts := NewTelemetryServer()

	// errors of type context.Canceled don't get retried
	fatalErr := fmt.Errorf("something went wrong: %w", context.Canceled)
	tb := &CannedReportBackend{fn: func(ctx context.Context, nodeID string) (*apipb.NetworkStatsReport, error) { return nil, fatalErr }}

	srvAddr := ts.Start(ctx, t)
	a := newAgent(t,
		WithClock(clockwork.NewFakeClock()),
		WithNode("mynode", WithTelemetryDriver(srvAddr, tb, grpc.WithTransportCredentials(insecure.NewCredentials()))))

	errCh := make(chan error)
	go func() { errCh <- a.Run(ctx) }()

	select {
	case err := <-errCh:
		if !errors.Is(err, fatalErr) {
			t.Errorf("unexpected error: %s", err)
		}
	case <-ctx.Done():
		t.Errorf("timed out waiting for error")
	}
}

func TestPeriodicUpdatesAreStoppedWhenHzIsZero(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(baseContext(t), time.Second)
	defer cancel()

	ts := NewTelemetryServer()
	initialReport := &apipb.NetworkStatsReport{
		NodeId: proto.String("mynode"),
		InterfaceStatsById: map[string]*apipb.InterfaceStats{
			"lo0": {
				TxBytes: proto.Int64(1),
				RxBytes: proto.Int64(12),
			},
		},
	}
	periodicReport := &apipb.NetworkStatsReport{
		NodeId: proto.String("mynode"),
		InterfaceStatsById: map[string]*apipb.InterfaceStats{
			"lo0": {
				TxBytes: proto.Int64(3),
				RxBytes: proto.Int64(15),
			},
		},
	}

	times := &atomic.Int64{}
	tb := &CannedReportBackend{fn: func(_ context.Context, _ string) (*apipb.NetworkStatsReport, error) {
		if times.Add(1) == 1 {
			return initialReport, nil
		}
		return periodicReport, nil
	}}

	clock := clockwork.NewFakeClock()
	srvAddr := ts.Start(ctx, t)
	a := newAgent(t,
		WithClock(clock),
		WithNode("mynode", WithTelemetryDriver(srvAddr, tb, grpc.WithTransportCredentials(insecure.NewCredentials()))))

	errCh := make(chan error)
	go func() { errCh <- a.Run(ctx) }()

	gotFirst := <-ts.inChan

	// request one update per second
	ts.outChan <- &afpb.TelemetryRequest{
		NodeId: proto.String("mynode"),
		Type: &afpb.TelemetryRequest_StatisticsPublishRateHz{
			StatisticsPublishRateHz: *proto.Float64(1),
		},
	}

	clock.BlockUntil(1)

	// check there's no second update yet (clock hasn't advanced)
	select {
	case r := <-ts.inChan:
		t.Errorf("got unexpected report even though clock is frozen: %#v", r)
		return
	default:
	}

	clock.Advance(1 * time.Second)

	var gotSecond *afpb.TelemetryUpdate
	select {
	case gotSecond = <-ts.inChan:
	case <-ctx.Done():
		t.Errorf("timed out waiting for second periodic report")
	}

	// disable periodic uploads
	ts.outChan <- &afpb.TelemetryRequest{
		NodeId: proto.String("mynode"),
		Type: &afpb.TelemetryRequest_StatisticsPublishRateHz{
			StatisticsPublishRateHz: *proto.Float64(0),
		},
	}

	// We don't have any way of ensuring the telemetry service has processed
	// the disabling request yet and because of how the fake clockwork.Clock
	// works we can't synchronize on the underlying ticker being stopped.
	// Instead, we'll send a one-off report request here and wait for the
	// response which will ensure the disabling request has been processed in
	// time.
	ts.outChan <- &afpb.TelemetryRequest{
		NodeId: proto.String("mynode"),
		Type:   &afpb.TelemetryRequest_QueryStatistics{},
	}
	select {
	case <-ts.inChan:
	case <-ctx.Done():
		t.Errorf("timed out waiting for one-off report")
	}

	// now that we know the disabling request has been processed, we can check
	// to ensure that there's no more periodic updates
	clock.Advance(1 * time.Second)
	select {
	case <-ts.inChan:
		t.Errorf("got unexpected third periodic report")
	case <-ctx.Done():
		t.Errorf("test took too long")
	case <-time.After(100 * time.Millisecond):
	}

	assertProtosEqual(t, &afpb.TelemetryUpdate{
		NodeId: proto.String("mynode"), Type: &afpb.TelemetryUpdate_Statistics{Statistics: initialReport},
	}, gotFirst)
	assertProtosEqual(t, &afpb.TelemetryUpdate{
		NodeId: proto.String("mynode"), Type: &afpb.TelemetryUpdate_Statistics{Statistics: periodicReport},
	}, gotSecond)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestRequestsForWrongNodeIDAreIgnored(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(baseContext(t), time.Second)
	defer cancel()

	ts := NewTelemetryServer()
	initialReport := &apipb.NetworkStatsReport{
		NodeId: proto.String("mynode"),
		InterfaceStatsById: map[string]*apipb.InterfaceStats{
			"lo0": {
				TxBytes: proto.Int64(1),
				RxBytes: proto.Int64(12),
			},
		},
	}
	periodicReport := &apipb.NetworkStatsReport{
		NodeId: proto.String("mynode"),
		InterfaceStatsById: map[string]*apipb.InterfaceStats{
			"lo0": {
				TxBytes: proto.Int64(3),
				RxBytes: proto.Int64(15),
			},
		},
	}

	reportCh := make(chan *apipb.NetworkStatsReport, 2)
	reportCh <- initialReport
	reportCh <- periodicReport

	tb := &CannedReportBackend{fn: func(ctx context.Context, nodeID string) (*apipb.NetworkStatsReport, error) { return <-reportCh, nil }}

	clock := clockwork.NewFakeClock()
	srvAddr := ts.Start(ctx, t)
	a := newAgent(t,
		WithClock(clock),
		WithNode("mynode", WithTelemetryDriver(srvAddr, tb, grpc.WithTransportCredentials(insecure.NewCredentials()))))

	errCh := make(chan error)
	go func() { errCh <- a.Run(ctx) }()

	select {
	case <-ts.inChan:
	case <-ctx.Done():
	}

	// request one update per second for the wrong node
	ts.outChan <- &afpb.TelemetryRequest{
		NodeId: proto.String("some-other-node"),
		Type: &afpb.TelemetryRequest_StatisticsPublishRateHz{
			StatisticsPublishRateHz: *proto.Float64(1),
		},
	}
	// request one update per 5s for the right node
	ts.outChan <- &afpb.TelemetryRequest{
		NodeId: proto.String("mynode"),
		Type: &afpb.TelemetryRequest_StatisticsPublishRateHz{
			StatisticsPublishRateHz: *proto.Float64(0.2),
		},
	}

	clock.BlockUntil(1)
	clock.Advance(1 * time.Second)

	// check there's no periodic update yet (1hz)
	select {
	case r := <-ts.inChan:
		t.Errorf("got unexpected report %#v even though request used incorrect node ID", r)
		return
	default:
	}

	clock.Advance(5 * time.Second)
	// check there's a periodic update for the right node (5hz)
	select {
	case <-ctx.Done():
		t.Errorf("timed out waiting for periodic report")
		return
	case <-ts.inChan:
	}

	// check there's no more periodic updates (wrong node ID)
	select {
	case r := <-ts.inChan:
		t.Errorf("got unexpected report %#v even though request used incorrect node ID", r)
		return
	default:
	}

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

type telemetryServer struct {
	inChan  chan *afpb.TelemetryUpdate
	outChan chan *afpb.TelemetryRequest

	afpb.UnimplementedNetworkTelemetryStreamingServer
}

func NewTelemetryServer() *telemetryServer {
	return &telemetryServer{
		inChan:  make(chan *afpb.TelemetryUpdate),
		outChan: make(chan *afpb.TelemetryRequest),
	}
}

func (ts *telemetryServer) Start(ctx context.Context, t *testing.T) (addr string) {
	nl, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("telemetryServer: couldn't Listen: %s", err)
	}

	grpcSrv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	afpb.RegisterNetworkTelemetryStreamingServer(grpcSrv, ts)

	errCh := make(chan error)
	go func() { errCh <- grpcSrv.Serve(nl) }()
	go func() {
		defer nl.Close()
		for {
			select {
			case <-ctx.Done():
				grpcSrv.Stop()
			case err := <-errCh:
				if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
					t.Errorf("error serving: %s", err)
				}
				return
			}
		}
	}()

	return nl.Addr().(*net.TCPAddr).String()
}

func (ts *telemetryServer) TelemetryInterface(stream afpb.NetworkTelemetryStreaming_TelemetryInterfaceServer) error {
	g, ctx := errgroup.WithContext(stream.Context())

	g.Go(channels.NewSink(ts.inChan).FillFrom(stream.Recv).WithCtx(ctx))
	g.Go(channels.NewSource(ts.outChan).ForwardTo(stream.Send).WithCtx(ctx))

	return g.Wait()
}
