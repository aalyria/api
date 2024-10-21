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
	"net"
	"testing"
	"time"

	apipb "aalyria.com/spacetime/api/common"
	telemetrypb "aalyria.com/spacetime/telemetry/v1alpha"

	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func textPBIfaceID(t *testing.T, nodeID, ifaceID string) string {
	b, _ := prototext.Marshal(&apipb.NetworkInterfaceId{
		NodeId:      proto.String(nodeID),
		InterfaceId: proto.String(ifaceID),
	})
	return string(b)
}

type manualReportDriver struct {
	reports chan *telemetrypb.ExportMetricsRequest
}

func newManualReportDriver() *manualReportDriver {
	return &manualReportDriver{
		reports: make(chan *telemetrypb.ExportMetricsRequest),
	}
}

func (mrd *manualReportDriver) Stats() interface{} { return nil }
func (mrd *manualReportDriver) Run(ctx context.Context, nodeID string, reportMetrics func(*telemetrypb.ExportMetricsRequest) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case report := <-mrd.reports:
			reportMetrics(report)
		}
	}
}

func TestRelaysMetricsFromDriverToController(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(baseContext(t), time.Second)
	defer cancel()

	ts := NewTelemetryServer()
	td := newManualReportDriver()

	srvAddr := ts.Start(ctx, t)
	a := newAgent(t,
		WithClock(clockwork.NewFakeClock()),
		WithNode("mynode", WithTelemetryDriver(srvAddr, td, grpc.WithTransportCredentials(insecure.NewCredentials()))))
	errCh := make(chan error)
	go func() { errCh <- a.Run(ctx) }()

	servedReport := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: []*telemetrypb.InterfaceMetrics{{
			InterfaceId: textPBIfaceID(t, "foobar", "lo0"),
			StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{{
				TxBytes: 1,
				RxBytes: 12,
			}},
		}},
	}
	td.reports <- servedReport

	gotReport := <-ts.reportedMetrics
	assertProtosEqual(t, servedReport, gotReport)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

type telemetryServer struct {
	reportedMetrics chan *telemetrypb.ExportMetricsRequest

	telemetrypb.UnimplementedTelemetryServer
}

func NewTelemetryServer() *telemetryServer {
	return &telemetryServer{
		reportedMetrics: make(chan *telemetrypb.ExportMetricsRequest),
	}
}

func (ts *telemetryServer) Start(ctx context.Context, t *testing.T) (addr string) {
	nl, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("telemetryServer: couldn't Listen: %s", err)
	}

	grpcSrv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	telemetrypb.RegisterTelemetryServer(grpcSrv, ts)

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

func (ts *telemetryServer) ExportMetrics(ctx context.Context, req *telemetrypb.ExportMetricsRequest) (*emptypb.Empty, error) {
	ts.reportedMetrics <- req
	return nil, nil
}
