// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
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

package pfsense

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"aalyria.com/spacetime/agent/internal/pfsense"
	"aalyria.com/spacetime/agent/internal/pfsense/impl"
	apipb "aalyria.com/spacetime/api/common"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
)

// Driver implements the telemetry.Driver interface for pfSense.
type Driver struct {
	client           pfsense.APIClientInterface
	collectionPeriod time.Duration

	// mu guards stats, which is read via Stats() from a different goroutine
	// than the collection loop that updates it.
	mu    sync.Mutex
	stats *Stats
}

// Stats holds telemetry driver statistics.
type Stats struct {
	LastCollectionTime time.Time
	CollectionCount    int64
	ErrorCount         int64
}

// NewDriver creates a new pfSense telemetry driver.
func NewDriver(config pfsense.Config, collectionPeriod time.Duration) *Driver {
	// Always use v2 API client
	client := impl.NewAPIClientV2(config)

	return &Driver{
		client:           client,
		collectionPeriod: collectionPeriod,
		stats:            &Stats{},
	}
}

// Run implements the telemetry.Driver interface.
func (d *Driver) Run(
	ctx context.Context,
	nodeID string,
	reportMetrics func(*telemetrypb.ExportMetricsRequest) error,
) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Str("nodeID", nodeID).Logger()
	log.Info().Msg("starting pfSense telemetry driver")

	// time.NewTicker panics on a non-positive duration, so reject a misconfigured
	// collection period here rather than crashing the agent at driver start.
	if d.collectionPeriod <= 0 {
		return fmt.Errorf("collectionPeriod must be greater than zero, got %v", d.collectionPeriod)
	}

	// Test connection to pfSense API
	if err := d.client.TestConnection(ctx); err != nil {
		log.Error().Err(err).Msg("failed to connect to pfSense API")
		return err
	}

	ticker := time.NewTicker(d.collectionPeriod)
	defer ticker.Stop()

	// Report an initial sample immediately so metrics are available before the
	// first tick fires.
	d.collectOnce(ctx, nodeID, reportMetrics)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("pfSense telemetry driver stopped")
			return context.Cause(ctx)
		case <-ticker.C:
			d.collectOnce(ctx, nodeID, reportMetrics)
		}
	}
}

// collectOnce performs a single collection cycle and records the outcome in
// stats.
func (d *Driver) collectOnce(
	ctx context.Context,
	nodeID string,
	reportMetrics func(*telemetrypb.ExportMetricsRequest) error,
) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()

	err := d.collectAndReportMetrics(ctx, nodeID, reportMetrics)

	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil {
		log.Error().Err(err).Msg("failed to collect and report metrics")
		d.stats.ErrorCount++
		return
	}
	d.stats.CollectionCount++
	d.stats.LastCollectionTime = time.Now()
}

// collectAndReportMetrics collects metrics from pfSense and reports them.
func (d *Driver) collectAndReportMetrics(
	ctx context.Context,
	nodeID string,
	reportMetrics func(*telemetrypb.ExportMetricsRequest) error,
) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()

	// Collect interface statistics using the correct endpoint
	interfaces, err := d.client.Get(ctx, "/api/v2/status/interfaces")
	if err != nil {
		log.Error().Err(err).Msg("failed to get interface statistics")
		return err
	}

	// do() returns a completed non-2xx response as (resp, nil), so a 401/5xx from
	// pfSense must be checked explicitly before parsing; otherwise an error body
	// carrying "data": [] would be reported as a successful collection of zero
	// interfaces.
	if interfaces.Status != "ok" || interfaces.Code != 200 {
		statusErr := fmt.Errorf("failed to get interface statistics: status=%s, code=%d, message=%s", interfaces.Status, interfaces.Code, interfaces.Message)
		log.Error().Err(statusErr).Msg("get interface statistics failed")
		return statusErr
	}

	log.Info().
		Interface("interfaces", interfaces).
		Msg("collected pfSense interface metrics")

	// Parse interface data to extract RX/TX statistics
	interfaceStats, err := d.parseInterfaceStats(ctx, interfaces)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse interface statistics")
		return err
	}

	// Create telemetry report with interface statistics
	report := &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: d.convertToInterfaceMetrics(ctx, nodeID, interfaceStats),
	}

	// Log the collected statistics
	log.Debug().
		Interface("interface_stats", interfaceStats).
		Msg("parsed interface statistics")

	return reportMetrics(report)
}

// convertToInterfaceMetrics converts our internal stats to protobuf format
func (d *Driver) convertToInterfaceMetrics(ctx context.Context, nodeID string, stats []InterfaceStats) []*telemetrypb.InterfaceMetrics {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	var metrics []*telemetrypb.InterfaceMetrics

	for _, stat := range stats {
		// The SBI keys interface metrics by a prototext-marshalled
		// NetworkInterfaceId{node_id, interface_id} (matching the netlink and
		// snmp drivers); emitting the bare pfSense name would leave the exported
		// metrics with no node association.
		interfaceID, err := prototext.Marshal(&apipb.NetworkInterfaceId{
			NodeId:      proto.String(nodeID),
			InterfaceId: proto.String(stat.Name),
		})
		if err != nil {
			log.Err(err).Str("interface", stat.Name).Msg("marshalling textproto interface ID")
			continue
		}

		// Create a data point with current timestamp
		now := time.Now()
		timestamp := &timestamppb.Timestamp{
			Seconds: now.Unix(),
			Nanos:   int32(now.Nanosecond()),
		}

		// pfSense's counters are cumulative since interface/boot time, not since
		// this sample, so StartTime is left unset (matching the netlink driver)
		// rather than pinned to the collection time, which would give every
		// point a zero-width accumulation window and break rate derivation.
		// Only the counters pfSense actually reports are populated; the other
		// optional fields are left unset to preserve the not-reported vs.
		// actually-zero distinction.
		dataPoint := &telemetrypb.StandardInterfaceStatisticsDataPoint{
			Time:      timestamp,
			RxBytes:   &stat.RXBytes,
			TxBytes:   &stat.TXBytes,
			RxPackets: &stat.RXPackets,
			TxPackets: &stat.TXPackets,
			RxErrors:  &stat.RXErrors,
			TxErrors:  &stat.TXErrors,
		}

		// Create interface metrics
		interfaceMetric := &telemetrypb.InterfaceMetrics{
			InterfaceId: proto.String(string(interfaceID)),
			StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{
				dataPoint,
			},
		}

		metrics = append(metrics, interfaceMetric)
	}

	return metrics
}

// InterfaceStats represents the statistics for a network interface
type InterfaceStats struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	RXBytes     int64  `json:"rx_bytes"`
	TXBytes     int64  `json:"tx_bytes"`
	RXPackets   int64  `json:"rx_packets"`
	TXPackets   int64  `json:"tx_packets"`
	RXErrors    int64  `json:"rx_errors"`
	TXErrors    int64  `json:"tx_errors"`
	Status      string `json:"status"`
}

// parseInterfaceStats extracts RX/TX statistics from pfSense interface data
func (d *Driver) parseInterfaceStats(ctx context.Context, apiResponse *pfsense.PfSenseAPIResponse) ([]InterfaceStats, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	var stats []InterfaceStats

	// Try to extract interface data from the API response
	var interfaces []any

	// Method 1: Direct access if Data is already an array
	if interfacesArray, ok := apiResponse.Data.([]any); ok {
		interfaces = interfacesArray
		log.Debug().Msg("found interfaces as direct array")
	} else if dataMap, ok := apiResponse.Data.(map[string]any); ok {
		// Method 2: Access via "data" field
		if interfacesArray, ok := dataMap["data"].([]any); ok {
			interfaces = interfacesArray
			log.Debug().Msg("found interfaces in data field")
		} else {
			// Method 3: Try to find any array in the response
			for key, value := range dataMap {
				if interfacesArray, ok := value.([]any); ok {
					log.Debug().Str("found_array_key", key).Msg("found interfaces array in response")
					interfaces = interfacesArray
					break
				}
			}
		}
	}

	if interfaces == nil {
		log.Error().Interface("data_type", apiResponse.Data).Msg("could not find interfaces array in response")
		return nil, fmt.Errorf("could not find interfaces array in response, data type: %T", apiResponse.Data)
	}

	log.Debug().Int("interface_count", len(interfaces)).Msg("processing interfaces")

	// Parse each interface
	for i, iface := range interfaces {
		ifaceMap, ok := iface.(map[string]any)
		if !ok {
			log.Debug().Int("interface_index", i).Interface("interface", iface).Msg("interface is not a map")
			continue
		}

		stat := InterfaceStats{}

		// Extract interface name
		if name, ok := ifaceMap["name"].(string); ok {
			stat.Name = name
		}

		// Extract description
		if desc, ok := ifaceMap["descr"].(string); ok {
			stat.Description = desc
		}

		// Extract status
		if status, ok := ifaceMap["status"].(string); ok {
			stat.Status = status
		}

		// Extract statistics using the actual pfSense API field names
		// Based on the API documentation provided
		if inbytes, ok := ifaceMap["inbytes"].(float64); ok {
			stat.RXBytes = int64(inbytes)
		}
		if outbytes, ok := ifaceMap["outbytes"].(float64); ok {
			stat.TXBytes = int64(outbytes)
		}
		if inpkts, ok := ifaceMap["inpkts"].(float64); ok {
			stat.RXPackets = int64(inpkts)
		}
		if outpkts, ok := ifaceMap["outpkts"].(float64); ok {
			stat.TXPackets = int64(outpkts)
		}
		if inerrs, ok := ifaceMap["inerrs"].(float64); ok {
			stat.RXErrors = int64(inerrs)
		}
		if outerrs, ok := ifaceMap["outerrs"].(float64); ok {
			stat.TXErrors = int64(outerrs)
		}

		// Only add interfaces that have meaningful data
		if stat.Name != "" {
			stats = append(stats, stat)
			log.Debug().
				Int("interface_index", i).
				Str("name", stat.Name).
				Str("description", stat.Description).
				Str("status", stat.Status).
				Int64("rx_bytes", stat.RXBytes).
				Int64("tx_bytes", stat.TXBytes).
				Int64("rx_packets", stat.RXPackets).
				Int64("tx_packets", stat.TXPackets).
				Int64("rx_errors", stat.RXErrors).
				Int64("tx_errors", stat.TXErrors).
				Msg("parsed interface statistics")
		} else {
			log.Debug().Int("interface_index", i).Msg("skipping interface with no name")
		}
	}

	log.Info().Int("parsed_interfaces", len(stats)).Msg("successfully parsed interface statistics")
	return stats, nil
}

// Stats returns a snapshot of the driver's internal statistics.
func (d *Driver) Stats() any {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stats == nil {
		return nil
	}
	return *d.stats
}
