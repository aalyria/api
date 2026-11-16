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

package impl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aalyria.com/spacetime/agent/internal/pfsense"
)

func TestNewAPIClient_URLAutoCorrection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{
			name:     "endpoint with https",
			endpoint: "https://pfsense.example.com",
			expected: "https://pfsense.example.com",
		},
		{
			name:     "endpoint with http",
			endpoint: "http://pfsense.example.com",
			expected: "http://pfsense.example.com",
		},
		{
			name:     "endpoint without protocol (should default to https)",
			endpoint: "pfsense.example.com",
			expected: "https://pfsense.example.com",
		},
		{
			name:     "localhost without protocol (should default to https)",
			endpoint: "localhost:8081",
			expected: "https://localhost:8081",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := pfsense.Config{
				APIEndpoint: tt.endpoint,
				Username:    "admin",
				APIKey:      "test-api-key",
				Timeout:     30 * time.Second,
				Retries:     3,
			}

			client := NewAPIClientV2(config)
			if client == nil {
				t.Fatal("NewAPIClientV2() returned nil")
			}

			apiClient, ok := client.(*APIClientV2)
			if !ok {
				t.Fatalf("client is %T, want *APIClientV2", client)
			}
			if apiClient.endpoint != tt.expected {
				t.Errorf("endpoint = %q, want %q", apiClient.endpoint, tt.expected)
			}
		})
	}
}

func TestNewAPIClient(t *testing.T) {
	t.Parallel()

	config := pfsense.Config{
		APIEndpoint: "https://pfsense.example.com",
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	client := NewAPIClientV2(config)
	if client == nil {
		t.Fatal("NewAPIClientV2() returned nil")
	}

	apiClient, ok := client.(*APIClientV2)
	if !ok {
		t.Fatalf("client is %T, want *APIClientV2", client)
	}
	if apiClient.endpoint != config.APIEndpoint {
		t.Errorf("endpoint = %q, want %q", apiClient.endpoint, config.APIEndpoint)
	}
	if apiClient.username != config.Username {
		t.Errorf("username = %q, want %q", apiClient.username, config.Username)
	}
	if apiClient.apiKey != config.APIKey {
		t.Errorf("apiKey = %q, want %q", apiClient.apiKey, config.APIKey)
	}
	if apiClient.timeout != config.Timeout {
		t.Errorf("timeout = %v, want %v", apiClient.timeout, config.Timeout)
	}
	if apiClient.retries != config.Retries {
		t.Errorf("retries = %d, want %d", apiClient.retries, config.Retries)
	}
	if apiClient.client == nil {
		t.Fatal("apiClient.client is nil")
	}
	if apiClient.client.Timeout != config.Timeout {
		t.Errorf("client.Timeout = %v, want %v", apiClient.client.Timeout, config.Timeout)
	}
}

func TestNewAPIClient_WithInsecureSkipVerify(t *testing.T) {
	t.Parallel()

	config := pfsense.Config{
		APIEndpoint:        "https://pfsense.example.com",
		Username:           "admin",
		APIKey:             "test-api-key",
		Timeout:            30 * time.Second,
		Retries:            3,
		InsecureSkipVerify: true,
	}

	client := NewAPIClientV2(config)
	if client == nil {
		t.Fatal("NewAPIClientV2() returned nil")
	}

	apiClient, ok := client.(*APIClientV2)
	if !ok {
		t.Fatalf("client is %T, want *APIClientV2", client)
	}
	if apiClient.client.Transport == nil {
		t.Error("client.Transport is nil, want non-nil")
	}
}

func TestAPIClient_TestConnection_Success(t *testing.T) {
	t.Parallel()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/status/system" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v2/status/system")
		}
		if r.Method != "GET" {
			t.Errorf("method = %q, want GET", r.Method)
		}

		response := pfsense.PfSenseAPIResponse{
			Status: "ok",
			Code:   200,
			Data:   map[string]any{"version": "2.7.0"},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := pfsense.Config{
		APIEndpoint: server.URL,
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	client := NewAPIClientV2(config)
	ctx := context.Background()
	if err := client.TestConnection(ctx); err != nil {
		t.Errorf("TestConnection() returned error: %v", err)
	}
}

func TestAPIClient_TestConnection_Error(t *testing.T) {
	t.Parallel()

	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := pfsense.PfSenseAPIResponse{
			Status:  "error",
			Code:    500,
			Message: "Internal server error",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := pfsense.Config{
		APIEndpoint: server.URL,
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	client := NewAPIClientV2(config)
	ctx := context.Background()
	err := client.TestConnection(ctx)
	if err == nil {
		t.Fatal("TestConnection() returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "pfSense API returned error status") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "pfSense API returned error status")
	}
}

func TestAPIClient_GetGateways_Success(t *testing.T) {
	t.Parallel()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/routing/gateways" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v2/routing/gateways")
		}
		if r.Method != "GET" {
			t.Errorf("method = %q, want GET", r.Method)
		}

		response := pfsense.PfSenseAPIResponse{
			Status: "ok",
			Code:   200,
			Data: []any{
				map[string]any{
					"name": "WAN",
					"ip":   "192.168.1.1",
				},
				map[string]any{
					"name": "LAN",
					"ip":   "10.0.0.1",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := pfsense.Config{
		APIEndpoint: server.URL,
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	client := NewAPIClientV2(config)
	ctx := context.Background()
	gateways, err := client.GetGateways(ctx)
	if err != nil {
		t.Errorf("GetGateways() returned error: %v", err)
	}
	if gateways == nil {
		t.Error("GetGateways() returned nil gateways, want non-nil")
	}
	// Note: The actual implementation would need to parse the gateway data
	// This test verifies the basic HTTP call works
}

func TestAPIClient_GetGateways_Error(t *testing.T) {
	t.Parallel()

	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := pfsense.PfSenseAPIResponse{
			Status:  "error",
			Code:    500,
			Message: "Internal server error",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := pfsense.Config{
		APIEndpoint: server.URL,
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	client := NewAPIClientV2(config)
	ctx := context.Background()
	gateways, err := client.GetGateways(ctx)
	if err == nil {
		t.Fatal("GetGateways() returned nil error, want error")
	}
	if gateways != nil {
		t.Errorf("GetGateways() returned %v, want nil", gateways)
	}
	if !strings.Contains(err.Error(), "gateways API returned error status") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "gateways API returned error status")
	}
}

func TestAPIClient_Get_Success(t *testing.T) {
	t.Parallel()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/test" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v2/test")
		}
		if r.Method != "GET" {
			t.Errorf("method = %q, want GET", r.Method)
		}

		response := pfsense.PfSenseAPIResponse{
			Status: "ok",
			Code:   200,
			Data:   map[string]any{"test": "data"},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := pfsense.Config{
		APIEndpoint: server.URL,
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	client := NewAPIClientV2(config)
	ctx := context.Background()
	resp, err := client.Get(ctx, "/api/v2/test")
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("resp.Status = %q, want ok", resp.Status)
	}
	if resp.Code != 200 {
		t.Errorf("resp.Code = %d, want 200", resp.Code)
	}
}

func TestAPIClient_Post_Success(t *testing.T) {
	t.Parallel()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/test" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v2/test")
		}
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}

		response := pfsense.PfSenseAPIResponse{
			Status: "ok",
			Code:   200,
			Data:   map[string]any{"created": true},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := pfsense.Config{
		APIEndpoint: server.URL,
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	client := NewAPIClientV2(config)
	ctx := context.Background()
	data := map[string]any{"name": "test"}
	resp, err := client.Post(ctx, "/api/v2/test", data)
	if err != nil {
		t.Fatalf("Post() returned error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("resp.Status = %q, want ok", resp.Status)
	}
	if resp.Code != 200 {
		t.Errorf("resp.Code = %d, want 200", resp.Code)
	}
}

func TestAPIClient_Put_Success(t *testing.T) {
	t.Parallel()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/test" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v2/test")
		}
		if r.Method != "PUT" {
			t.Errorf("method = %q, want PUT", r.Method)
		}

		response := pfsense.PfSenseAPIResponse{
			Status: "ok",
			Code:   200,
			Data:   map[string]any{"updated": true},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := pfsense.Config{
		APIEndpoint: server.URL,
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	client := NewAPIClientV2(config)
	ctx := context.Background()
	data := map[string]any{"name": "updated"}
	resp, err := client.Put(ctx, "/api/v2/test", data)
	if err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("resp.Status = %q, want ok", resp.Status)
	}
	if resp.Code != 200 {
		t.Errorf("resp.Code = %d, want 200", resp.Code)
	}
}

func TestAPIClient_Patch_Success(t *testing.T) {
	t.Parallel()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/test" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v2/test")
		}
		if r.Method != "PATCH" {
			t.Errorf("method = %q, want PATCH", r.Method)
		}

		response := pfsense.PfSenseAPIResponse{
			Status: "ok",
			Code:   200,
			Data:   map[string]any{"patched": true},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := pfsense.Config{
		APIEndpoint: server.URL,
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	client := NewAPIClientV2(config)
	ctx := context.Background()
	data := map[string]any{"field": "value"}
	resp, err := client.Patch(ctx, "/api/v2/test", data)
	if err != nil {
		t.Fatalf("Patch() returned error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("resp.Status = %q, want ok", resp.Status)
	}
	if resp.Code != 200 {
		t.Errorf("resp.Code = %d, want 200", resp.Code)
	}
}

func TestAPIClient_Delete_Success(t *testing.T) {
	t.Parallel()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/test" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v2/test")
		}
		if r.Method != "DELETE" {
			t.Errorf("method = %q, want DELETE", r.Method)
		}

		response := pfsense.PfSenseAPIResponse{
			Status: "ok",
			Code:   200,
			Data:   map[string]any{"deleted": true},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := pfsense.Config{
		APIEndpoint: server.URL,
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     30 * time.Second,
		Retries:     3,
	}

	client := NewAPIClientV2(config)
	ctx := context.Background()
	resp, err := client.Delete(ctx, "/api/v2/test")
	if err != nil {
		t.Fatalf("Delete() returned error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("resp.Status = %q, want ok", resp.Status)
	}
	if resp.Code != 200 {
		t.Errorf("resp.Code = %d, want 200", resp.Code)
	}
}

func TestAPIClient_HTTP_Error(t *testing.T) {
	t.Parallel()

	// Create a test server that doesn't respond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't respond, causing a timeout
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	config := pfsense.Config{
		APIEndpoint: server.URL,
		Username:    "admin",
		APIKey:      "test-api-key",
		Timeout:     1 * time.Second, // Short timeout
		Retries:     1,
	}

	client := NewAPIClientV2(config)
	ctx := context.Background()
	resp, err := client.Get(ctx, "/api/v2/test")
	if err == nil {
		t.Fatal("Get() returned nil error, want error")
	}
	if resp != nil {
		t.Errorf("Get() returned %v, want nil", resp)
	}
}
