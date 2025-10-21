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
	"io"
	"math/big"
	"net/http"
	"time"

	apipb "aalyria.com/spacetime/api/common"

	"github.com/jonboulle/clockwork"
	promcli "github.com/prometheus/client_model/go"
	"github.com/prometheus/prom2json"
	"google.golang.org/protobuf/proto"
)

// ScraperConfig represents the configuration for a prometheus scraper.
type ScraperConfig struct {
	Clock clockwork.Clock

	// ExporterURL is the URL that hosts the metrics page for a Prometheus
	// exporter.
	ExporterURL string
	// NodeID is the ID of the node associated with the generated
	// NetworkStatsReports.
	NodeID string
	// ScrapeInterval is how frequently the scraper should scrape metrics.
	ScrapeInterval time.Duration
	// Callback gets called once a NetworkStatsReport is generated. A non-nil
	// error will stop the scraper.
	Callback func(*apipb.NetworkStatsReport) error
}

// Scraper is a
type Scraper struct {
	conf ScraperConfig
}

// NewScraper returns a Scraper with the provided configuration.
func NewScraper(conf ScraperConfig) *Scraper {
	return &Scraper{conf: conf}
}

// Start kicks off the scraping process in the current goroutine. Will stop when
// the provided context is cancelled or on the first encountered error.
func (s *Scraper) Start(ctx context.Context) error {
	for {
		if err := s.scrape(ctx); err != nil {
			return err
		}

		timer := s.conf.Clock.NewTimer(s.conf.ScrapeInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.Chan()
			}
			return ctx.Err()

		case <-timer.Chan():
		}
	}
}

func (s *Scraper) scrape(ctx context.Context) error {
	stats, err := s.fetchMetrics(ctx)
	if err != nil {
		return err
	}

	ifaceStats, err := s.extractInterfaceStats(stats)
	if err != nil {
		return err
	}

	return s.conf.Callback(&apipb.NetworkStatsReport{
		NodeId: &s.conf.NodeID,
		Timestamp: &apipb.DateTime{
			UnixTimeUsec: proto.Int64(s.conf.Clock.Now().UnixMicro()),
		},
		InterfaceStatsById: ifaceStats,
	})
}

func (s *Scraper) fetchMetrics(ctx context.Context) ([]*prom2json.Family, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.conf.ExporterURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseReader(resp.Body)
}

// parseReader wraps the asynchronous prom2json.ParseReader function in a
// synchronous interface.
func parseReader(r io.Reader) ([]*prom2json.Family, error) {
	inCh := make(chan *promcli.MetricFamily)
	outCh := make(chan []*prom2json.Family)

	go func() {
		stats := []*prom2json.Family{}
		for s := range inCh {
			stats = append(stats, prom2json.NewFamily(s))
		}
		outCh <- stats
	}()

	err := prom2json.ParseReader(r, inCh)
	stats := <-outCh
	return stats, err
}

func (s *Scraper) extractInterfaceStats(stats []*prom2json.Family) (map[string]*apipb.InterfaceStats, error) {
	result := map[string]*apipb.InterfaceStats{}

	for _, mf := range stats {
		addMetricToReport := func(is *apipb.InterfaceStats, value *int64) {}

		// TODO: parameterize these key/values via the ScraperConfig
		switch mf.Name {
		case "node_network_transmit_bytes_total":
			addMetricToReport = func(is *apipb.InterfaceStats, value *int64) { is.TxBytes = value }
		case "node_network_receive_bytes_total":
			addMetricToReport = func(is *apipb.InterfaceStats, value *int64) { is.RxBytes = value }
		case "node_network_transmit_packets_total":
			addMetricToReport = func(is *apipb.InterfaceStats, value *int64) { is.TxPackets = value }
		case "node_network_receive_packets_total":
			addMetricToReport = func(is *apipb.InterfaceStats, value *int64) { is.RxPackets = value }
		case "node_network_transmit_drop_total":
			addMetricToReport = func(is *apipb.InterfaceStats, value *int64) { is.TxDropped = value }
		case "node_network_receive_drop_total":
			addMetricToReport = func(is *apipb.InterfaceStats, value *int64) { is.RxDropped = value }
		case "node_network_transmit_errs_total":
			addMetricToReport = func(is *apipb.InterfaceStats, value *int64) { is.TxErrors = value }
		case "node_network_receive_errs_total":
			addMetricToReport = func(is *apipb.InterfaceStats, value *int64) { is.RxErrors = value }
		default:
			continue
		}

		for _, m := range mf.Metrics {
			metric, ok := m.(prom2json.Metric)
			if !ok {
				continue
			}
			dev, ok := metric.Labels["device"]
			if !ok {
				continue
			}
			rep, ok := result[dev]
			if !ok {
				rep = &apipb.InterfaceStats{}
				result[dev] = rep
			}

			flt, _, err := big.ParseFloat(metric.Value, 10, 0, big.ToNearestEven)
			if err != nil {
				return nil, err
			}
			valAsInt, _ := flt.Int64()
			addMetricToReport(rep, proto.Int64(valAsInt))
		}
	}

	return result, nil
}
