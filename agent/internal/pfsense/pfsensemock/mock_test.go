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

package pfsensemock

import (
	"context"
	"testing"

	"aalyria.com/spacetime/agent/internal/pfsense"
)

func TestNew(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockClient := New()
	if mockClient == nil {
		t.Fatal("New() returned nil")
	}

	// Test that the mock client implements the interface correctly
	if err := mockClient.TestConnection(ctx); err != nil {
		t.Errorf("TestConnection() returned error: %v", err)
	}

	gateways, err := mockClient.GetGateways(ctx)
	if err != nil {
		t.Errorf("GetGateways() returned error: %v", err)
	}
	if got, want := gateways["192.168.1.1"], "WAN"; got != want {
		t.Errorf("gateways[192.168.1.1] = %q, want %q", got, want)
	}
	if got, want := gateways["10.0.0.1"], "LAN"; got != want {
		t.Errorf("gateways[10.0.0.1] = %q, want %q", got, want)
	}
}

func TestClient_FuncOverride(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockClient := New()
	mockClient.GetStaticRoutesFunc = func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
		return &pfsense.PfSenseAPIResponse{
			Status: "ok",
			Code:   200,
			Data:   []any{map[string]any{"id": "7", "network": "10.0.0.0/8"}},
		}, nil
	}

	resp, err := mockClient.GetStaticRoutes(ctx)
	if err != nil {
		t.Fatalf("GetStaticRoutes() returned error: %v", err)
	}
	routes, ok := resp.Data.([]any)
	if !ok {
		t.Fatalf("resp.Data is %T, want []any", resp.Data)
	}
	if len(routes) != 1 {
		t.Errorf("len(routes) = %d, want 1", len(routes))
	}
}
