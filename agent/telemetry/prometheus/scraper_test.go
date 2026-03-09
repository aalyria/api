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

package prometheus

import (
	"context"
	_ "embed"
	"net"
	"net/http"
	"testing"
	"time"

	apipb "aalyria.com/spacetime/api/common"

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

type testCase struct {
	description string

	metrics  []byte
	expected func(clockwork.Clock) *apipb.NetworkStatsReport
}

var (
	//go:embed node_exporter_metrics_testdata.txt
	nodeExporterMetrics []byte

	testCases = []testCase{
		{
			description: "sample node_exporter output",
			metrics:     nodeExporterMetrics,
			expected: func(clock clockwork.Clock) *apipb.NetworkStatsReport {
				return &apipb.NetworkStatsReport{
					NodeId: proto.String("it's me, a cool node"),
					Timestamp: &apipb.DateTime{
						UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
					},
					InterfaceStatsById: map[string]*apipb.InterfaceStats{
						"docker0": {
							TxPackets: proto.Int64(181958),
							RxPackets: proto.Int64(70236),
							TxBytes:   proto.Int64(2895355422),
							RxBytes:   proto.Int64(4602427),
							TxDropped: proto.Int64(132),
							RxDropped: proto.Int64(97),
							TxErrors:  proto.Int64(132),
							RxErrors:  proto.Int64(12),
						},
						"ens4": {
							TxPackets: proto.Int64(5638352),
							RxPackets: proto.Int64(10269729),
							TxBytes:   proto.Int64(28704733728),
							RxBytes:   proto.Int64(49004717667),
							TxDropped: proto.Int64(37),
							RxDropped: proto.Int64(12),
							TxErrors:  proto.Int64(19),
							RxErrors:  proto.Int64(1337),
						},
						"lo": {
							TxPackets: proto.Int64(2586281),
							RxPackets: proto.Int64(2586281),
							TxBytes:   proto.Int64(769496589),
							RxBytes:   proto.Int64(769496589),
							TxDropped: proto.Int64(0),
							RxDropped: proto.Int64(0),
							TxErrors:  proto.Int64(0),
							RxErrors:  proto.Int64(0),
						},
					},
				}
			},
		},
	}
)

func assertProtosEqual(t *testing.T, want, got interface{}) {
	t.Helper()

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("proto mismatch: (-want +got):\n%s", diff)
		t.FailNow()
	}
}

func (tc testCase) createMetricServer(ctx context.Context, t *testing.T) *http.Server {
	t.Helper()

	return &http.Server{
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
			rw.WriteHeader(http.StatusOK)
			_, err := rw.Write(tc.metrics)
			if err != nil {
				t.Errorf("error writing metrics to response: %s", err)
			}
		}),
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}
}

func TestScraper(t *testing.T) {
	for _, tc := range testCases {
		func(tc testCase) {
			t.Run(tc.description, func(t *testing.T) {
				t.Parallel()
				runTest(t, tc)
			})
		}(tc)
	}
}

func listenOnUnusedPort(ctx context.Context, t *testing.T) net.Listener {
	t.Helper()

	nl, err := (&net.ListenConfig{}).Listen(ctx, "tcp", ":0")
	if err != nil {
		t.Fatal("error creating a TCP listener", err)
	}

	return nl
}

func runTest(t *testing.T, tc testCase) {
	ctx, done := context.WithTimeout(context.Background(), 1*time.Second)
	defer done()

	clock := clockwork.NewFakeClock()
	expected := tc.expected(clock)

	srv := tc.createMetricServer(ctx, t)
	nl := listenOnUnusedPort(ctx, t)
	defer nl.Close()
	srvErrCh := make(chan error)
	go func() { srvErrCh <- srv.Serve(nl) }()

	reportCh := make(chan *apipb.NetworkStatsReport)
	conf := ScraperConfig{
		Clock:          clock,
		ExporterURL:    "http://" + nl.Addr().String(),
		NodeID:         *expected.NodeId,
		ScrapeInterval: 30 * time.Second,
		Callback: func(report *apipb.NetworkStatsReport) error {
			reportCh <- report
			return nil
		},
	}

	scrapeErrCh := make(chan error)
	go func() {
		scrapeErrCh <- NewScraper(conf).Start(ctx)
	}()

	select {
	case <-ctx.Done():
		t.Errorf("context finished before NetworkStatsReport was generated: %s", ctx.Err())
	case rep := <-reportCh:
		assertProtosEqual(t, expected, rep)
	}

	srv.Shutdown(ctx)
	if err := <-srvErrCh; err != nil && err != http.ErrServerClosed {
		t.Errorf("error serving metrics: %s", err)
	}

	done()
	clock.Advance(conf.ScrapeInterval)
	if err := <-scrapeErrCh; err != nil && err != context.Canceled {
		t.Errorf("error scraping metrics: %s", err)
	}
}
