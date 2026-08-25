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
	"time"
)

// PfSenseAPIResponse represents a standard pfSense API response.
type PfSenseAPIResponse struct {
	Status  string `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// APIClientInterface defines the interface for pfSense API operations.
// This allows both enactment and telemetry drivers to use the same API client.
type APIClientInterface interface {
	// Connection and validation
	TestConnection(ctx context.Context) error

	// Gateway operations
	GetGateways(ctx context.Context) (map[string]string, error)
	GetDefaultGateway(ctx context.Context) (*PfSenseAPIResponse, error)
	SetDefaultGateway(ctx context.Context, gatewayName string) (*PfSenseAPIResponse, error)

	// Routing operations
	GetRoutingConfig(ctx context.Context) (*PfSenseAPIResponse, error)
	CreateRoute(ctx context.Context, routeData map[string]any) (*PfSenseAPIResponse, error)
	DeleteRoute(ctx context.Context, routeID string) (*PfSenseAPIResponse, error)
	UpdateFirewallRule(ctx context.Context, ruleID int, updateData map[string]any) (*PfSenseAPIResponse, error)

	// Static route operations
	GetStaticRoutes(ctx context.Context) (*PfSenseAPIResponse, error)
	CreateStaticRoute(ctx context.Context, routeData map[string]any) (*PfSenseAPIResponse, error)
	UpdateStaticRoute(ctx context.Context, routeID string, routeData map[string]any) (*PfSenseAPIResponse, error)
	DeleteStaticRoute(ctx context.Context, routeID string) (*PfSenseAPIResponse, error)
	ApplyStaticRoutes(ctx context.Context) (*PfSenseAPIResponse, error)

	// Firewall operations
	ApplyFirewall(ctx context.Context) (*PfSenseAPIResponse, error)
	FlushStates(ctx context.Context) (*PfSenseAPIResponse, error)

	// Generic HTTP operations
	Get(ctx context.Context, path string) (*PfSenseAPIResponse, error)
	Post(ctx context.Context, path string, data any) (*PfSenseAPIResponse, error)
	Put(ctx context.Context, path string, data any) (*PfSenseAPIResponse, error)
	Patch(ctx context.Context, path string, data any) (*PfSenseAPIResponse, error)
	Delete(ctx context.Context, path string) (*PfSenseAPIResponse, error)
	DeleteWithBody(ctx context.Context, path string, data any) (*PfSenseAPIResponse, error)
}

// Config holds the configuration for pfSense API client. (Refer to
// configpb.PfSenseTelemetry for the corresponding protobuf message and
// description.)
type Config struct {
	APIEndpoint        string
	Username           string
	APIKey             string
	Timeout            time.Duration
	Retries            int
	InsecureSkipVerify bool
}
