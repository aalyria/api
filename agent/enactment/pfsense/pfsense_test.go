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
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"aalyria.com/spacetime/agent/internal/pfsense"
	"aalyria.com/spacetime/agent/internal/pfsense/pfsensemock"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
)

func TestNew(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	driver := New(config)

	if driver == nil {
		t.Fatal("New() returned nil")
	}
	// Don't compare the entire config since Factory is set automatically
	if driver.config.Clock != config.Clock {
		t.Errorf("config.Clock = %v, want %v", driver.config.Clock, config.Clock)
	}
	if driver.config.APIEndpoint != config.APIEndpoint {
		t.Errorf("config.APIEndpoint = %q, want %q", driver.config.APIEndpoint, config.APIEndpoint)
	}
	if driver.config.Username != config.Username {
		t.Errorf("config.Username = %q, want %q", driver.config.Username, config.Username)
	}
	if driver.config.APIKey != config.APIKey {
		t.Errorf("config.APIKey = %q, want %q", driver.config.APIKey, config.APIKey)
	}
	if driver.config.Timeout != config.Timeout {
		t.Errorf("config.Timeout = %v, want %v", driver.config.Timeout, config.Timeout)
	}
	if driver.config.Retries != config.Retries {
		t.Errorf("config.Retries = %d, want %d", driver.config.Retries, config.Retries)
	}
	if driver.client == nil {
		t.Error("driver.client is nil, want non-nil")
	}
	if driver.stats == nil {
		t.Error("driver.stats is nil, want non-nil")
	}
}

func TestNew_DefaultValues(t *testing.T) {
	t.Parallel()

	config := Config{
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	driver := New(config)

	if driver == nil {
		t.Fatal("New() returned nil")
	}
	if driver.config.Clock == nil {
		t.Error("config.Clock is nil, want non-nil")
	}
	if driver.config.Timeout != 30*time.Second {
		t.Errorf("config.Timeout = %v, want %v", driver.config.Timeout, 30*time.Second)
	}
	if driver.config.Retries != 3 {
		t.Errorf("config.Retries = %d, want 3", driver.config.Retries)
	}
}

func TestDriver_Init(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// Create a mock client for testing
	mockClient := pfsensemock.New()

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()

	if err := driver.Init(ctx); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Verify that initialization populated the mappings
	if len(driver.gatewayMap) == 0 {
		t.Error("gatewayMap is empty, want populated")
	}
}

func TestDriver_Dispatch_SetRoute_Success(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// Create a mock client for testing
	mockClient := pfsensemock.New()

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()

	// Initialize the driver
	if err := driver.Init(ctx); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Test SetRoute request
	req := &schedpb.CreateEntryRequest{
		Id: "test-route",
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "192.168.1.0/24",
				To:   "10.0.0.0/8",
				Dev:  "em0",
				Via:  "192.168.1.1",
			},
		},
	}

	if err := driver.Dispatch(ctx, req); err != nil {
		t.Errorf("Dispatch() returned error: %v", err)
	}

	// Verify stats were updated
	checkDispatchStats(t, driver, 1, 1, 0)
}

func TestDriver_Dispatch_DeleteRoute(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// The mock returns a static route to 10.0.0.0/8 so the delete/apply/flush
	// path is actually exercised, and records which calls the driver makes.
	var deletedID string
	applied, flushed := false, false
	mockClient := pfsensemock.New()
	mockClient.GetStaticRoutesFunc = func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
		return &pfsense.PfSenseAPIResponse{
			Status: "ok",
			Code:   200,
			Data: []any{
				map[string]any{"id": "7", "network": "10.0.0.0/8", "gateway": "WAN"},
			},
		}, nil
	}
	mockClient.DeleteStaticRouteFunc = func(ctx context.Context, routeID string) (*pfsense.PfSenseAPIResponse, error) {
		deletedID = routeID
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
	}
	mockClient.ApplyStaticRoutesFunc = func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
		applied = true
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
	}
	mockClient.FlushStatesFunc = func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
		flushed = true
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
	}

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()

	// Initialize the driver
	if err := driver.Init(ctx); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Test DeleteRoute request
	req := &schedpb.CreateEntryRequest{
		Id: "test-route-delete",
		ConfigurationChange: &schedpb.CreateEntryRequest_DeleteRoute{
			DeleteRoute: &schedpb.DeleteRoute{
				From: "192.168.1.0/24",
				To:   "10.0.0.0/8",
			},
		},
	}

	if err := driver.Dispatch(ctx, req); err != nil {
		t.Errorf("Dispatch() returned error: %v", err)
	}

	// The matching route must be deleted, then the change applied and states
	// flushed.
	if deletedID != "7" {
		t.Errorf("deletedID = %q, want 7", deletedID)
	}
	if !applied {
		t.Error("expected applyStaticRoutes to be called")
	}
	if !flushed {
		t.Error("expected flushFirewallStates to be called")
	}

	// Verify stats were updated
	checkDispatchStats(t, driver, 1, 1, 0)
}

func TestDriver_Dispatch_SetRoute_UpdatesExistingRoute(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// An existing route to 10.0.0.0/8 via a different gateway must be updated in
	// place, not deleted and re-created.
	var updatedID string
	var updatedGateway any
	created, deleted := false, false
	mockClient := pfsensemock.New()
	mockClient.GetStaticRoutesFunc = func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
		return &pfsense.PfSenseAPIResponse{
			Status: "ok",
			Code:   200,
			Data: []any{
				map[string]any{"id": "3", "network": "10.0.0.0/8", "gateway": "LAN"},
			},
		}, nil
	}
	mockClient.UpdateStaticRouteFunc = func(ctx context.Context, routeID string, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
		updatedID = routeID
		updatedGateway = routeData["gateway"]
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
	}
	mockClient.CreateStaticRouteFunc = func(ctx context.Context, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
		created = true
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
	}
	mockClient.DeleteStaticRouteFunc = func(ctx context.Context, routeID string) (*pfsense.PfSenseAPIResponse, error) {
		deleted = true
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
	}

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()
	if err := driver.Init(ctx); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	req := &schedpb.CreateEntryRequest{
		Id: "test-route-update",
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "192.168.1.0/24",
				To:   "10.0.0.0/8",
				Dev:  "em0",
				Via:  "192.168.1.1",
			},
		},
	}

	if err := driver.Dispatch(ctx, req); err != nil {
		t.Fatalf("Dispatch() returned error: %v", err)
	}

	// The existing route is updated in place to the new gateway; nothing is
	// created or deleted.
	if updatedID != "3" {
		t.Errorf("updatedID = %q, want 3", updatedID)
	}
	if updatedGateway != "WAN" {
		t.Errorf("updatedGateway = %v, want WAN", updatedGateway)
	}
	if created {
		t.Error("expected no createStaticRoute call")
	}
	if deleted {
		t.Error("expected no deleteStaticRoute call")
	}
}

func TestDriver_Dispatch_SetRoute_ExistingRouteStillAppliesAndFlushes(t *testing.T) {
	t.Parallel()

	config := Config{
		Clock:       clockwork.NewFakeClock(),
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// The exact desired route (10.0.0.0/8 via WAN) already exists in the config.
	// A prior apply may have failed, so the driver must still apply and flush
	// without creating, updating, or deleting anything.
	created, updated, deleted := false, false, false
	applied, flushed := false, false
	mockClient := pfsensemock.New()
	mockClient.GetStaticRoutesFunc = func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
		return &pfsense.PfSenseAPIResponse{
			Status: "ok",
			Code:   200,
			Data: []any{
				map[string]any{"id": "1", "network": "10.0.0.0/8", "gateway": "WAN"},
			},
		}, nil
	}
	mockClient.CreateStaticRouteFunc = func(ctx context.Context, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
		created = true
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
	}
	mockClient.UpdateStaticRouteFunc = func(ctx context.Context, routeID string, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
		updated = true
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
	}
	mockClient.DeleteStaticRouteFunc = func(ctx context.Context, routeID string) (*pfsense.PfSenseAPIResponse, error) {
		deleted = true
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
	}
	mockClient.ApplyStaticRoutesFunc = func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
		applied = true
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
	}
	mockClient.FlushStatesFunc = func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
		flushed = true
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
	}

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()
	if err := driver.Init(ctx); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	req := &schedpb.CreateEntryRequest{
		Id: "test-route-exists",
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "192.168.1.0/24",
				To:   "10.0.0.0/8",
				Dev:  "em0",
				Via:  "192.168.1.1",
			},
		},
	}

	if err := driver.Dispatch(ctx, req); err != nil {
		t.Fatalf("Dispatch() returned error: %v", err)
	}

	if created || updated || deleted {
		t.Errorf("config mutated: created=%v updated=%v deleted=%v, want all false", created, updated, deleted)
	}
	if !applied {
		t.Error("expected applyStaticRoutes to be called even when the route already exists")
	}
	if !flushed {
		t.Error("expected flushFirewallStates to be called even when the route already exists")
	}
}

func TestDriver_Dispatch_InvalidRequest(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// Create a mock client for testing
	mockClient := pfsensemock.New()

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()

	// Initialize the driver
	if err := driver.Init(ctx); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Test invalid request (no configuration change)
	req := &schedpb.CreateEntryRequest{
		Id: "invalid-request",
		// No ConfigurationChange field
	}

	err := driver.Dispatch(ctx, req)
	if err == nil {
		t.Fatal("Dispatch() returned nil error, want error")
	}

	// Check that it's a gRPC error
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("status.FromError(%v) ok = false, want true", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want %v", st.Code(), codes.InvalidArgument)
	}

	// Verify stats were updated
	stats := checkDispatchStats(t, driver, 1, 0, 1)
	if len(stats.RecentErrors) != 1 {
		t.Errorf("len(RecentErrors) = %d, want 1", len(stats.RecentErrors))
	}
}

func TestDriver_Dispatch_NotInitialized(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// Create a mock client for testing
	mockClient := pfsensemock.New()

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()

	// Don't initialize the driver

	// Test SetRoute request
	req := &schedpb.CreateEntryRequest{
		Id: "test-route",
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "192.168.1.0/24",
				To:   "10.0.0.0/8",
				Dev:  "em0",
				Via:  "192.168.1.1",
			},
		},
	}

	err := driver.Dispatch(ctx, req)
	if err == nil {
		t.Fatal("Dispatch() returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "driver not initialized - call Init() first") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "driver not initialized - call Init() first")
	}

	// Verify stats were updated
	checkDispatchStats(t, driver, 1, 0, 1)
}

func TestDriver_Dispatch_SetRoute_InvalidGateway(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// Create a mock client for testing
	mockClient := pfsensemock.New()

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()

	// Initialize the driver
	if err := driver.Init(ctx); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Test SetRoute request with invalid gateway
	req := &schedpb.CreateEntryRequest{
		Id: "test-route-invalid-gateway",
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "192.168.1.0/24",
				To:   "10.0.0.0/8",
				Dev:  "em0",
				Via:  "invalid-gateway", // This gateway doesn't exist in our mock
			},
		},
	}

	err := driver.Dispatch(ctx, req)
	if err == nil {
		t.Fatal("Dispatch() returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "Could not find gateway invalid-gateway") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "Could not find gateway invalid-gateway")
	}

	// Verify stats were updated
	checkDispatchStats(t, driver, 1, 0, 1)
}

func TestDriver_Dispatch_SetRoute_EmptyFields(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// Create a mock client for testing
	mockClient := pfsensemock.New()

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()

	// Initialize the driver
	if err := driver.Init(ctx); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Test SetRoute request with empty fields
	req := &schedpb.CreateEntryRequest{
		Id: "test-route-empty",
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "", // Empty From field
				To:   "10.0.0.0/8",
				Dev:  "em0",
				Via:  "192.168.1.1",
			},
		},
	}

	if err := driver.Dispatch(ctx, req); err != nil {
		t.Errorf("Dispatch() returned error: %v", err)
	}

	// Verify stats were updated
	checkDispatchStats(t, driver, 1, 1, 0)
}

func TestDriver_Stats_InitialState(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// Create a mock client for testing
	mockClient := pfsensemock.New()

	driver := NewWithClient(config, mockClient)

	stats := driver.Stats()
	if stats == nil {
		t.Fatal("Stats() returned nil")
	}

	typedStats, ok := stats.(Stats)
	if !ok {
		t.Fatalf("Stats() returned %T, want Stats", stats)
	}
	if typedStats.InitCount != 0 {
		t.Errorf("InitCount = %d, want 0", typedStats.InitCount)
	}
	if typedStats.DispatchCount != 0 {
		t.Errorf("DispatchCount = %d, want 0", typedStats.DispatchCount)
	}
	if typedStats.SuccessCount != 0 {
		t.Errorf("SuccessCount = %d, want 0", typedStats.SuccessCount)
	}
	if typedStats.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", typedStats.FailureCount)
	}
	if len(typedStats.RecentErrors) != 0 {
		t.Errorf("len(RecentErrors) = %d, want 0", len(typedStats.RecentErrors))
	}
	if !typedStats.LastDispatchTime.IsZero() {
		t.Errorf("LastDispatchTime = %v, want zero", typedStats.LastDispatchTime)
	}
	if typedStats.TotalDispatchTime != 0 {
		t.Errorf("TotalDispatchTime = %v, want 0", typedStats.TotalDispatchTime)
	}
	if typedStats.AverageDispatchTime != 0 {
		t.Errorf("AverageDispatchTime = %v, want 0", typedStats.AverageDispatchTime)
	}
}

func TestDriver_Stats_AfterOperations(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// Create a mock client for testing
	mockClient := pfsensemock.New()

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()

	// Initialize the driver
	if err := driver.Init(ctx); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Perform some operations
	req := &schedpb.CreateEntryRequest{
		Id: "test-route",
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "192.168.1.0/24",
				To:   "10.0.0.0/8",
				Dev:  "em0",
				Via:  "192.168.1.1",
			},
		},
	}

	if err := driver.Dispatch(ctx, req); err != nil {
		t.Fatalf("Dispatch() returned error: %v", err)
	}

	stats := driver.Stats().(Stats)
	if stats.InitCount != 1 {
		t.Errorf("InitCount = %d, want 1", stats.InitCount)
	}
	if stats.DispatchCount != 1 {
		t.Errorf("DispatchCount = %d, want 1", stats.DispatchCount)
	}
	if stats.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", stats.SuccessCount)
	}
	if stats.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", stats.FailureCount)
	}
	if len(stats.RecentErrors) != 0 {
		t.Errorf("len(RecentErrors) = %d, want 0", len(stats.RecentErrors))
	}
	if stats.LastDispatchTime.IsZero() {
		t.Error("LastDispatchTime is zero, want non-zero")
	}
	// Note: With fake clock, durations will be 0 since time doesn't advance
	// In real usage, these would be greater than 0
	if stats.TotalDispatchTime != 0 {
		t.Errorf("TotalDispatchTime = %v, want 0", stats.TotalDispatchTime)
	}
	if stats.AverageDispatchTime != 0 {
		t.Errorf("AverageDispatchTime = %v, want 0", stats.AverageDispatchTime)
	}
}

func TestDriver_Stats_ErrorHandling(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// Create a mock client for testing
	mockClient := pfsensemock.New()

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()

	// Initialize the driver
	if err := driver.Init(ctx); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Perform operations that will fail
	req1 := &schedpb.CreateEntryRequest{
		Id: "test-route-1",
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "192.168.1.0/24",
				To:   "10.0.0.0/8",
				Dev:  "em0",
				Via:  "invalid-gateway", // This will fail
			},
		},
	}

	req2 := &schedpb.CreateEntryRequest{
		Id: "test-route-2",
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "192.168.1.0/24",
				To:   "10.0.0.0/8",
				Dev:  "em0",
				Via:  "192.168.1.1", // This will succeed
			},
		},
	}

	req3 := &schedpb.CreateEntryRequest{
		Id: "test-route-3",
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "192.168.1.0/24",
				To:   "10.0.0.0/8",
				Dev:  "em0",
				Via:  "another-invalid", // This will fail
			},
		},
	}

	// Execute the requests
	_ = driver.Dispatch(ctx, req1) // Should fail
	_ = driver.Dispatch(ctx, req2) // Should succeed
	_ = driver.Dispatch(ctx, req3) // Should fail

	stats := driver.Stats().(Stats)
	if stats.InitCount != 1 {
		t.Errorf("InitCount = %d, want 1", stats.InitCount)
	}
	if stats.DispatchCount != 3 {
		t.Errorf("DispatchCount = %d, want 3", stats.DispatchCount)
	}
	if stats.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", stats.SuccessCount)
	}
	if stats.FailureCount != 2 {
		t.Errorf("FailureCount = %d, want 2", stats.FailureCount)
	}
	if len(stats.RecentErrors) != 2 {
		t.Errorf("len(RecentErrors) = %d, want 2", len(stats.RecentErrors))
	}
}

func TestDriver_Close(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// Create a mock client for testing
	mockClient := pfsensemock.New()

	driver := NewWithClient(config, mockClient)

	if err := driver.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestDriver_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	config := Config{
		Clock:       clock,
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	// Create a mock client for testing
	mockClient := pfsensemock.New()

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()

	// Initialize the driver
	if err := driver.Init(ctx); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Test concurrent access to stats
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			req := &schedpb.CreateEntryRequest{
				Id: "concurrent-test",
				ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
					SetRoute: &schedpb.SetRoute{
						From: "192.168.1.0/24",
						To:   "10.0.0.0/8",
						Dev:  "em0",
						Via:  "192.168.1.1",
					},
				},
			}

			_ = driver.Dispatch(ctx, req)
			_ = driver.Stats()
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	stats := driver.Stats().(Stats)
	if stats.DispatchCount != 10 {
		t.Errorf("DispatchCount = %d, want 10", stats.DispatchCount)
	}
	if stats.SuccessCount != 10 {
		t.Errorf("SuccessCount = %d, want 10", stats.SuccessCount)
	}
}

func TestDriver_Stats_AddError(t *testing.T) {
	t.Parallel()

	stats := &Stats{}

	// Add errors and verify they're stored correctly
	stats.addError("error 1")
	if len(stats.RecentErrors) != 1 {
		t.Fatalf("len(RecentErrors) = %d, want 1", len(stats.RecentErrors))
	}
	if stats.RecentErrors[0] != "error 1" {
		t.Errorf("RecentErrors[0] = %q, want %q", stats.RecentErrors[0], "error 1")
	}

	stats.addError("error 2")
	if len(stats.RecentErrors) != 2 {
		t.Fatalf("len(RecentErrors) = %d, want 2", len(stats.RecentErrors))
	}
	if stats.RecentErrors[0] != "error 2" { // Most recent first
		t.Errorf("RecentErrors[0] = %q, want %q", stats.RecentErrors[0], "error 2")
	}
	if stats.RecentErrors[1] != "error 1" {
		t.Errorf("RecentErrors[1] = %q, want %q", stats.RecentErrors[1], "error 1")
	}

	// Add more errors to test the limit
	stats.addError("error 3")
	stats.addError("error 4")
	stats.addError("error 5")
	stats.addError("error 6") // This should cause the oldest to be dropped

	if len(stats.RecentErrors) != 5 { // Max is 5
		t.Fatalf("len(RecentErrors) = %d, want 5", len(stats.RecentErrors))
	}
	if stats.RecentErrors[0] != "error 6" { // Most recent
		t.Errorf("RecentErrors[0] = %q, want %q", stats.RecentErrors[0], "error 6")
	}
	if stats.RecentErrors[4] != "error 2" { // Oldest remaining
		t.Errorf("RecentErrors[4] = %q, want %q", stats.RecentErrors[4], "error 2")
	}
}

func TestGetAllStaticRoutes_UnparseablePayload(t *testing.T) {
	t.Parallel()

	config := Config{
		Clock:       clockwork.NewFakeClock(),
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
	}

	mockClient := pfsensemock.New()
	// A successful HTTP response whose payload is neither an array nor a map
	// carrying a "data" array must be reported as an error rather than treated
	// as an empty route table (which would let DeleteRoute claim success while
	// the route is still installed, and SetRoute create a duplicate).
	mockClient.GetStaticRoutesFunc = func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
		return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200, Data: "not a route table"}, nil
	}

	driver := NewWithClient(config, mockClient)
	ctx := context.Background()

	if _, err := driver.getAllStaticRoutes(ctx); err == nil {
		t.Fatal("getAllStaticRoutes() returned nil error, want error")
	} else if got := status.Code(err); got != codes.Internal {
		t.Errorf("status code = %v, want %v", got, codes.Internal)
	}
}

// checkDispatchStats asserts the driver's dispatch/success/failure counters and
// returns the stats snapshot for any additional per-test assertions.
func checkDispatchStats(t *testing.T, driver *Driver, dispatch, success, failure int) Stats {
	t.Helper()
	stats := driver.Stats().(Stats)
	if stats.DispatchCount != dispatch {
		t.Errorf("DispatchCount = %d, want %d", stats.DispatchCount, dispatch)
	}
	if stats.SuccessCount != success {
		t.Errorf("SuccessCount = %d, want %d", stats.SuccessCount, success)
	}
	if stats.FailureCount != failure {
		t.Errorf("FailureCount = %d, want %d", stats.FailureCount, failure)
	}
	return stats
}
