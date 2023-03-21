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
	"testing"
	"time"

	afpb "aalyria.com/spacetime/api/cdpi/v1alpha"
	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/internal/channels"

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestStreamStartsWithInitialReport(t *testing.T) {
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
	tb := func(ctx context.Context) (*apipb.NetworkStatsReport, error) {
		return servedReport, nil
	}

	a := newAgent(t,
		WithClock(clockwork.NewFakeClock()),
		WithServerEndpoint(ts.Start(ctx, t)),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithNode("mynode", WithTelemetryBackend(tb)))
	errCh := make(chan error)
	go func() { errCh <- a.Run(ctx) }()

	gotReport := <-ts.inChan
	assertProtosEqual(t, &afpb.TelemetryUpdate{
		Type: &afpb.TelemetryUpdate_Statistics{
			Statistics: servedReport,
		},
	}, gotReport)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestCanRequestOneOffReport(t *testing.T) {
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
	tb := func(ctx context.Context) (*apipb.NetworkStatsReport, error) {
		return servedReport, nil
	}

	a := newAgent(t,
		WithClock(clockwork.NewFakeClock()),
		WithServerEndpoint(ts.Start(ctx, t)),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithNode("mynode", WithTelemetryBackend(tb)))
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
		Type: &afpb.TelemetryUpdate_Statistics{
			Statistics: servedReport,
		},
	}, second)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestIgnoresUnknownRequestType(t *testing.T) {
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
	tb := func(ctx context.Context) (*apipb.NetworkStatsReport, error) {
		return servedReport, nil
	}

	a := newAgent(t,
		WithClock(clockwork.NewFakeClock()),
		WithServerEndpoint(ts.Start(ctx, t)),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithNode("mynode", WithTelemetryBackend(tb)))
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

	tb := func(ctx context.Context) (*apipb.NetworkStatsReport, error) { return <-reportCh, nil }

	clock := clockwork.NewFakeClock()
	a := newAgent(t,
		WithClock(clock),
		WithServerEndpoint(ts.Start(ctx, t)),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithNode("mynode", WithTelemetryBackend(tb)))

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
		Type: &afpb.TelemetryUpdate_Statistics{Statistics: initialReport},
	}, gotFirst)
	assertProtosEqual(t, &afpb.TelemetryUpdate{
		Type: &afpb.TelemetryUpdate_Statistics{Statistics: periodicReport},
	}, gotSecond)
	assertProtosEqual(t, &afpb.TelemetryUpdate{
		Type: &afpb.TelemetryUpdate_Statistics{Statistics: periodicReport},
	}, gotThird)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestInitialReportFailsToGenerate(t *testing.T) {
	ctx, cancel := context.WithTimeout(baseContext(t), time.Second)
	defer cancel()

	ts := NewTelemetryServer()

	// errors of type context.Canceled don't get retried
	fatalErr := fmt.Errorf("something went wrong: %w", context.Canceled)
	tb := func(ctx context.Context) (*apipb.NetworkStatsReport, error) { return nil, fatalErr }

	a := newAgent(t,
		WithClock(clockwork.NewFakeClock()),
		WithServerEndpoint(ts.Start(ctx, t)),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithNode("mynode", WithTelemetryBackend(tb)))

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

	tb := func(ctx context.Context) (*apipb.NetworkStatsReport, error) { return <-reportCh, nil }

	clock := clockwork.NewFakeClock()
	a := newAgent(t,
		WithClock(clock),
		WithServerEndpoint(ts.Start(ctx, t)),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithNode("mynode", WithTelemetryBackend(tb)))

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

	clock.Advance(5 * time.Second)

	select {
	case <-ts.inChan:
		t.Errorf("got unexpected third periodic report")
	default:
	}

	assertProtosEqual(t, &afpb.TelemetryUpdate{
		Type: &afpb.TelemetryUpdate_Statistics{Statistics: initialReport},
	}, gotFirst)
	assertProtosEqual(t, &afpb.TelemetryUpdate{
		Type: &afpb.TelemetryUpdate_Statistics{Statistics: periodicReport},
	}, gotSecond)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestRequestsForWrongNodeIDAreIgnored(t *testing.T) {
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

	tb := func(ctx context.Context) (*apipb.NetworkStatsReport, error) { return <-reportCh, nil }

	clock := clockwork.NewFakeClock()
	a := newAgent(t,
		WithClock(clock),
		WithServerEndpoint(ts.Start(ctx, t)),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithNode("mynode", WithTelemetryBackend(tb)))

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

func baseContext(t *testing.T) context.Context {
	log := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Stack().Caller().Logger()
	return log.WithContext(context.Background())
}

func newAgent(t *testing.T, opts ...AgentOption) *Agent {
	t.Helper()

	a, err := NewAgent(opts...)
	if err != nil {
		t.Fatalf("error creating agent: %s", err)
	}
	return a
}

func check(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}

func assertProtosEqual(t *testing.T, want, got interface{}) {
	t.Helper()

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("proto mismatch: (-want +got):\n%s", diff)
		t.FailNow()
	}
}

var rpcCanceledError = status.FromContextError(context.Canceled).Err()

func checkErrIsDueToCanceledContext(t *testing.T, err error) {
	if !errors.Is(err, context.Canceled) && !errors.Is(err, rpcCanceledError) {
		t.Error("unexpected error:", err)
	}
}
