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
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"

	"aalyria.com/spacetime/agent/internal/pfsense"
	"aalyria.com/spacetime/agent/internal/pfsense/pfsensemock"
	apipb "aalyria.com/spacetime/api/common"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
)

// errReportFailed is a sentinel error used to exercise the report-failure path.
var errReportFailed = errors.New("report failed")

func TestNewDriver(t *testing.T) {
	config := pfsense.Config{
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	collectionPeriod := 60 * time.Second
	driver := NewDriver(config, collectionPeriod)

	if driver == nil {
		t.Fatal("NewDriver() returned nil")
	}
	if driver.collectionPeriod != collectionPeriod {
		t.Errorf("collectionPeriod = %v, want %v", driver.collectionPeriod, collectionPeriod)
	}
	if driver.client == nil {
		t.Error("driver.client is nil, want non-nil")
	}
	if driver.stats == nil {
		t.Error("driver.stats is nil, want non-nil")
	}
}

func TestDriver_CollectAndReportMetrics_Success(t *testing.T) {
	// Create a mock client that returns valid data
	mockClient := pfsensemock.New()

	driver := &Driver{
		client:           mockClient,
		collectionPeriod: 100 * time.Millisecond,
		stats:            &Stats{},
	}

	ctx := context.Background()

	// Create a mock report function
	reportCalled := false
	var reportedRequest *telemetrypb.ExportMetricsRequest
	reportMetrics := func(req *telemetrypb.ExportMetricsRequest) error {
		reportCalled = true
		reportedRequest = req
		return nil
	}

	if err := driver.collectAndReportMetrics(ctx, "test-node", reportMetrics); err != nil {
		t.Errorf("collectAndReportMetrics() returned error: %v", err)
	}
	if !reportCalled {
		t.Error("Report function should be called on successful collection")
	}
	if reportedRequest == nil {
		t.Error("Report request should not be nil")
	}
}

func TestDriver_CollectAndReportMetrics_ReportFailure(t *testing.T) {
	// Create a mock client that returns valid data
	mockClient := pfsensemock.New()

	driver := &Driver{
		client:           mockClient,
		collectionPeriod: 100 * time.Millisecond,
		stats:            &Stats{},
	}

	ctx := context.Background()

	// Create a mock report function that fails
	reportMetrics := func(req *telemetrypb.ExportMetricsRequest) error {
		return errReportFailed
	}

	err := driver.collectAndReportMetrics(ctx, "test-node", reportMetrics)
	if err == nil {
		t.Fatal("collectAndReportMetrics() returned nil error, want error")
	}
	if !errors.Is(err, errReportFailed) {
		t.Errorf("error = %v, want %v", err, errReportFailed)
	}
}

func TestDriver_CollectAndReportMetrics_APIErrorStatus(t *testing.T) {
	// pfSense returns a completed non-2xx response as (resp, nil). Even when its
	// error body carries an empty "data" array, the collection must fail rather
	// than report zero interfaces as a success.
	mockClient := pfsensemock.New()
	mockClient.GetFunc = func(ctx context.Context, path string) (*pfsense.PfSenseAPIResponse, error) {
		return &pfsense.PfSenseAPIResponse{Status: "error", Code: 500, Message: "boom", Data: []any{}}, nil
	}

	driver := &Driver{
		client:           mockClient,
		collectionPeriod: 100 * time.Millisecond,
		stats:            &Stats{},
	}

	reportCalled := false
	reportMetrics := func(req *telemetrypb.ExportMetricsRequest) error {
		reportCalled = true
		return nil
	}

	err := driver.collectAndReportMetrics(context.Background(), "test-node", reportMetrics)
	if err == nil {
		t.Fatal("collectAndReportMetrics() returned nil error on API error status, want error")
	}
	if reportCalled {
		t.Error("reportMetrics should not be called when the API returns an error status")
	}
}

func TestDriver_Run_NonPositiveCollectionPeriod(t *testing.T) {
	// A non-positive collection period would panic time.NewTicker; Run must
	// return an error instead.
	driver := &Driver{
		client:           pfsensemock.New(),
		collectionPeriod: 0,
		stats:            &Stats{},
	}

	err := driver.Run(context.Background(), "test-node", func(*telemetrypb.ExportMetricsRequest) error { return nil })
	if err == nil {
		t.Fatal("Run() returned nil error for non-positive collection period, want error")
	}
}

func TestDriver_ConvertToInterfaceMetrics(t *testing.T) {
	driver := &Driver{}

	stats := []InterfaceStats{
		{
			Name:        "wan",
			Description: "WAN Interface",
			RXBytes:     1024000,
			TXBytes:     512000,
			RXPackets:   1000,
			TXPackets:   500,
			RXErrors:    0,
			TXErrors:    0,
			Status:      "up",
		},
		{
			Name:        "lan",
			Description: "LAN Interface",
			RXBytes:     2048000,
			TXBytes:     1024000,
			RXPackets:   2000,
			TXPackets:   1000,
			RXErrors:    1,
			TXErrors:    0,
			Status:      "up",
		},
	}

	metrics := driver.convertToInterfaceMetrics(context.Background(), "test-node", stats)

	if len(metrics) != 2 {
		t.Fatalf("len(metrics) = %d, want 2", len(metrics))
	}

	// Check first interface. The interface ID is a prototext-marshalled
	// NetworkInterfaceId that carries the node association.
	node, iface := parseInterfaceID(t, *metrics[0].InterfaceId)
	if node != "test-node" {
		t.Errorf("metrics[0] node = %q, want test-node", node)
	}
	if iface != "wan" {
		t.Errorf("metrics[0] interface = %q, want wan", iface)
	}
	if got := len(metrics[0].StandardInterfaceStatisticsDataPoints); got != 1 {
		t.Fatalf("metrics[0] data points = %d, want 1", got)
	}

	dataPoint := metrics[0].StandardInterfaceStatisticsDataPoints[0]
	checkDataPoint(t, "metrics[0]", dataPoint, 1024000, 512000, 1000, 500, 0, 0)

	// Check second interface
	node, iface = parseInterfaceID(t, *metrics[1].InterfaceId)
	if node != "test-node" {
		t.Errorf("metrics[1] node = %q, want test-node", node)
	}
	if iface != "lan" {
		t.Errorf("metrics[1] interface = %q, want lan", iface)
	}
	if got := len(metrics[1].StandardInterfaceStatisticsDataPoints); got != 1 {
		t.Fatalf("metrics[1] data points = %d, want 1", got)
	}

	dataPoint = metrics[1].StandardInterfaceStatisticsDataPoints[0]
	checkDataPoint(t, "metrics[1]", dataPoint, 2048000, 1024000, 2000, 1000, 1, 0)
}

// checkDataPoint verifies that a data point's counters match the expected values.
func checkDataPoint(t *testing.T, label string, dp *telemetrypb.StandardInterfaceStatisticsDataPoint, rxBytes, txBytes, rxPackets, txPackets, rxErrors, txErrors int64) {
	t.Helper()
	if got := *dp.RxBytes; got != rxBytes {
		t.Errorf("%s RxBytes = %d, want %d", label, got, rxBytes)
	}
	if got := *dp.TxBytes; got != txBytes {
		t.Errorf("%s TxBytes = %d, want %d", label, got, txBytes)
	}
	if got := *dp.RxPackets; got != rxPackets {
		t.Errorf("%s RxPackets = %d, want %d", label, got, rxPackets)
	}
	if got := *dp.TxPackets; got != txPackets {
		t.Errorf("%s TxPackets = %d, want %d", label, got, txPackets)
	}
	if got := *dp.RxErrors; got != rxErrors {
		t.Errorf("%s RxErrors = %d, want %d", label, got, rxErrors)
	}
	if got := *dp.TxErrors; got != txErrors {
		t.Errorf("%s TxErrors = %d, want %d", label, got, txErrors)
	}
}

func TestDriver_ConvertToInterfaceMetrics_EmptyStats(t *testing.T) {
	driver := &Driver{}

	metrics := driver.convertToInterfaceMetrics(context.Background(), "test-node", []InterfaceStats{})
	if len(metrics) != 0 {
		t.Errorf("len(metrics) = %d, want 0", len(metrics))
	}
}

// parseInterfaceID unmarshals a prototext-encoded NetworkInterfaceId and returns
// its node and interface IDs.
func parseInterfaceID(t *testing.T, encoded string) (nodeID, interfaceID string) {
	t.Helper()
	var id apipb.NetworkInterfaceId
	if err := prototext.Unmarshal([]byte(encoded), &id); err != nil {
		t.Fatalf("prototext.Unmarshal(%q) returned error: %v", encoded, err)
	}
	return id.GetNodeId(), id.GetInterfaceId()
}

func TestParseInterfaceStats(t *testing.T) {
	driver := &Driver{}

	// Test with sample pfSense interface data using correct API field names
	sampleData := &pfsense.PfSenseAPIResponse{
		Status: "ok",
		Code:   200,
		Data: []any{
			map[string]any{
				"name":     "wan",
				"descr":    "WAN Interface",
				"status":   "up",
				"inbytes":  float64(1024000),
				"outbytes": float64(512000),
				"inpkts":   float64(1000),
				"outpkts":  float64(500),
				"inerrs":   float64(0),
				"outerrs":  float64(0),
			},
			map[string]any{
				"name":     "lan",
				"descr":    "LAN Interface",
				"status":   "up",
				"inbytes":  float64(2048000),
				"outbytes": float64(1024000),
				"inpkts":   float64(2000),
				"outpkts":  float64(1000),
				"inerrs":   float64(1),
				"outerrs":  float64(0),
			},
		},
	}

	stats, err := driver.parseInterfaceStats(context.Background(), sampleData)
	if err != nil {
		t.Fatalf("parseInterfaceStats() returned error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("len(stats) = %d, want 2", len(stats))
	}

	// Check first interface
	want0 := InterfaceStats{
		Name:        "wan",
		Description: "WAN Interface",
		Status:      "up",
		RXBytes:     1024000,
		TXBytes:     512000,
		RXPackets:   1000,
		TXPackets:   500,
		RXErrors:    0,
		TXErrors:    0,
	}
	if stats[0] != want0 {
		t.Errorf("stats[0] = %+v, want %+v", stats[0], want0)
	}

	// Check second interface
	want1 := InterfaceStats{
		Name:        "lan",
		Description: "LAN Interface",
		Status:      "up",
		RXBytes:     2048000,
		TXBytes:     1024000,
		RXPackets:   2000,
		TXPackets:   1000,
		RXErrors:    1,
		TXErrors:    0,
	}
	if stats[1] != want1 {
		t.Errorf("stats[1] = %+v, want %+v", stats[1], want1)
	}
}

func TestParseInterfaceStats_WithDataField(t *testing.T) {
	driver := &Driver{}

	// Test with data wrapped in a "data" field
	sampleData := &pfsense.PfSenseAPIResponse{
		Status: "ok",
		Code:   200,
		Data: map[string]any{
			"data": []any{
				map[string]any{
					"name":     "wan",
					"descr":    "WAN Interface",
					"status":   "up",
					"inbytes":  float64(1024000),
					"outbytes": float64(512000),
					"inpkts":   float64(1000),
					"outpkts":  float64(500),
					"inerrs":   float64(0),
					"outerrs":  float64(0),
				},
			},
		},
	}

	stats, err := driver.parseInterfaceStats(context.Background(), sampleData)
	if err != nil {
		t.Fatalf("parseInterfaceStats() returned error: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("len(stats) = %d, want 1", len(stats))
	}

	if stats[0].Name != "wan" {
		t.Errorf("stats[0].Name = %q, want wan", stats[0].Name)
	}
	if stats[0].Description != "WAN Interface" {
		t.Errorf("stats[0].Description = %q, want %q", stats[0].Description, "WAN Interface")
	}
	if stats[0].RXBytes != 1024000 {
		t.Errorf("stats[0].RXBytes = %d, want 1024000", stats[0].RXBytes)
	}
}

func TestParseInterfaceStats_WithNestedArray(t *testing.T) {
	driver := &Driver{}

	// Test with data containing nested arrays
	sampleData := &pfsense.PfSenseAPIResponse{
		Status: "ok",
		Code:   200,
		Data: map[string]any{
			"interfaces": []any{
				map[string]any{
					"name":     "wan",
					"descr":    "WAN Interface",
					"status":   "up",
					"inbytes":  float64(1024000),
					"outbytes": float64(512000),
					"inpkts":   float64(1000),
					"outpkts":  float64(500),
					"inerrs":   float64(0),
					"outerrs":  float64(0),
				},
			},
		},
	}

	stats, err := driver.parseInterfaceStats(context.Background(), sampleData)
	if err != nil {
		t.Fatalf("parseInterfaceStats() returned error: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("len(stats) = %d, want 1", len(stats))
	}

	if stats[0].Name != "wan" {
		t.Errorf("stats[0].Name = %q, want wan", stats[0].Name)
	}
	if stats[0].RXBytes != 1024000 {
		t.Errorf("stats[0].RXBytes = %d, want 1024000", stats[0].RXBytes)
	}
}

func TestParseInterfaceStatsEmptyData(t *testing.T) {
	driver := &Driver{}

	// Test with empty data
	emptyData := &pfsense.PfSenseAPIResponse{
		Status: "ok",
		Code:   200,
		Data:   []any{},
	}

	stats, err := driver.parseInterfaceStats(context.Background(), emptyData)
	if err != nil {
		t.Fatalf("parseInterfaceStats() returned error: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("len(stats) = %d, want 0", len(stats))
	}
}

func TestParseInterfaceStatsInvalidData(t *testing.T) {
	driver := &Driver{}

	// Test with invalid data structure
	invalidData := &pfsense.PfSenseAPIResponse{
		Status: "ok",
		Code:   200,
		Data:   "not an array",
	}

	_, err := driver.parseInterfaceStats(context.Background(), invalidData)
	if err == nil {
		t.Fatal("parseInterfaceStats() returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "could not find interfaces array") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "could not find interfaces array")
	}
}

func TestParseInterfaceStats_MissingFields(t *testing.T) {
	driver := &Driver{}

	// Test with interface missing some fields
	sampleData := &pfsense.PfSenseAPIResponse{
		Status: "ok",
		Code:   200,
		Data: []any{
			map[string]any{
				"name": "wan",
				// Missing other fields
			},
			map[string]any{
				"name":     "lan",
				"descr":    "LAN Interface",
				"status":   "up",
				"inbytes":  float64(2048000),
				"outbytes": float64(1024000),
				"inpkts":   float64(2000),
				"outpkts":  float64(1000),
				"inerrs":   float64(1),
				"outerrs":  float64(0),
			},
		},
	}

	stats, err := driver.parseInterfaceStats(context.Background(), sampleData)
	if err != nil {
		t.Fatalf("parseInterfaceStats() returned error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("len(stats) = %d, want 2", len(stats))
	}

	// First interface should have default values for missing fields
	want0 := InterfaceStats{Name: "wan"}
	if stats[0] != want0 {
		t.Errorf("stats[0] = %+v, want %+v", stats[0], want0)
	}

	// Second interface should have all values
	if stats[1].Name != "lan" {
		t.Errorf("stats[1].Name = %q, want lan", stats[1].Name)
	}
	if stats[1].Description != "LAN Interface" {
		t.Errorf("stats[1].Description = %q, want %q", stats[1].Description, "LAN Interface")
	}
	if stats[1].RXBytes != 2048000 {
		t.Errorf("stats[1].RXBytes = %d, want 2048000", stats[1].RXBytes)
	}
}

func TestParseInterfaceStats_InterfaceWithoutName(t *testing.T) {
	driver := &Driver{}

	// Test with interface missing name (should be skipped)
	sampleData := &pfsense.PfSenseAPIResponse{
		Status: "ok",
		Code:   200,
		Data: []any{
			map[string]any{
				// Missing name
				"descr":    "WAN Interface",
				"status":   "up",
				"inbytes":  float64(1024000),
				"outbytes": float64(512000),
			},
			map[string]any{
				"name":     "lan",
				"descr":    "LAN Interface",
				"status":   "up",
				"inbytes":  float64(2048000),
				"outbytes": float64(1024000),
			},
		},
	}

	stats, err := driver.parseInterfaceStats(context.Background(), sampleData)
	if err != nil {
		t.Fatalf("parseInterfaceStats() returned error: %v", err)
	}
	if len(stats) != 1 { // Only lan interface should be included
		t.Fatalf("len(stats) = %d, want 1", len(stats))
	}

	if stats[0].Name != "lan" {
		t.Errorf("stats[0].Name = %q, want lan", stats[0].Name)
	}
}

func TestDriverStats(t *testing.T) {
	driver := &Driver{
		stats: &Stats{
			LastCollectionTime: time.Now(),
			CollectionCount:    10,
			ErrorCount:         1,
		},
	}

	stats := driver.Stats()
	if stats == nil {
		t.Fatal("Stats() returned nil")
	}

	// Type assertion to check the stats structure
	driverStats, ok := stats.(Stats)
	if !ok {
		t.Fatalf("Stats() returned %T, want Stats", stats)
	}
	if driverStats.CollectionCount != 10 {
		t.Errorf("CollectionCount = %d, want 10", driverStats.CollectionCount)
	}
	if driverStats.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", driverStats.ErrorCount)
	}
}

func TestDriverStats_NilStats(t *testing.T) {
	driver := &Driver{
		stats: nil,
	}

	if stats := driver.Stats(); stats != nil {
		t.Errorf("Stats() = %v, want nil", stats)
	}
}
