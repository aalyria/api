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

// Package pfsensemock provides a configurable mock implementation of the pfSense
// APIClientInterface for use in tests. It lives in its own testonly library so
// the mock is never linked into production agent binaries.
package pfsensemock

import (
	"context"

	"aalyria.com/spacetime/agent/internal/pfsense"
)

// New returns a Client whose methods return generic success responses. Tests
// that need specific behaviour set the corresponding Func field before use.
func New() *Client {
	return &Client{}
}

// Client is a configurable mock implementation of pfsense.APIClientInterface.
// Each method delegates to its matching Func field when set, and otherwise
// returns a default success response.
type Client struct {
	TestConnectionFunc     func(ctx context.Context) error
	GetGatewaysFunc        func(ctx context.Context) (map[string]string, error)
	GetDefaultGatewayFunc  func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error)
	SetDefaultGatewayFunc  func(ctx context.Context, gatewayName string) (*pfsense.PfSenseAPIResponse, error)
	GetRoutingConfigFunc   func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error)
	CreateRouteFunc        func(ctx context.Context, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error)
	DeleteRouteFunc        func(ctx context.Context, routeID string) (*pfsense.PfSenseAPIResponse, error)
	GetStaticRoutesFunc    func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error)
	CreateStaticRouteFunc  func(ctx context.Context, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error)
	UpdateStaticRouteFunc  func(ctx context.Context, routeID string, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error)
	DeleteStaticRouteFunc  func(ctx context.Context, routeID string) (*pfsense.PfSenseAPIResponse, error)
	ApplyStaticRoutesFunc  func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error)
	UpdateFirewallRuleFunc func(ctx context.Context, ruleID int, updateData map[string]any) (*pfsense.PfSenseAPIResponse, error)
	ApplyFirewallFunc      func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error)
	FlushStatesFunc        func(ctx context.Context) (*pfsense.PfSenseAPIResponse, error)
	GetFunc                func(ctx context.Context, path string) (*pfsense.PfSenseAPIResponse, error)
	PostFunc               func(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error)
	PutFunc                func(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error)
	PatchFunc              func(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error)
	DeleteFunc             func(ctx context.Context, path string) (*pfsense.PfSenseAPIResponse, error)
	DeleteWithBodyFunc     func(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error)
}

// Client must satisfy the production interface.
var _ pfsense.APIClientInterface = (*Client)(nil)

// ok returns a generic successful response.
func ok() (*pfsense.PfSenseAPIResponse, error) {
	return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200}, nil
}

// okData returns a generic successful response carrying data.
func okData(data any) (*pfsense.PfSenseAPIResponse, error) {
	return &pfsense.PfSenseAPIResponse{Status: "ok", Code: 200, Data: data}, nil
}

func (c *Client) TestConnection(ctx context.Context) error {
	if c.TestConnectionFunc != nil {
		return c.TestConnectionFunc(ctx)
	}
	return nil
}

func (c *Client) GetGateways(ctx context.Context) (map[string]string, error) {
	if c.GetGatewaysFunc != nil {
		return c.GetGatewaysFunc(ctx)
	}
	return map[string]string{
		"192.168.1.1": "WAN",
		"10.0.0.1":    "LAN",
	}, nil
}

func (c *Client) GetDefaultGateway(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	if c.GetDefaultGatewayFunc != nil {
		return c.GetDefaultGatewayFunc(ctx)
	}
	return okData(map[string]any{"defaultgw4": "WAN"})
}

func (c *Client) SetDefaultGateway(ctx context.Context, gatewayName string) (*pfsense.PfSenseAPIResponse, error) {
	if c.SetDefaultGatewayFunc != nil {
		return c.SetDefaultGatewayFunc(ctx, gatewayName)
	}
	return ok()
}

func (c *Client) GetRoutingConfig(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	if c.GetRoutingConfigFunc != nil {
		return c.GetRoutingConfigFunc(ctx)
	}
	return okData(map[string]any{"routes": []any{}})
}

func (c *Client) CreateRoute(ctx context.Context, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
	if c.CreateRouteFunc != nil {
		return c.CreateRouteFunc(ctx, routeData)
	}
	return okData(map[string]any{"id": "route-123"})
}

func (c *Client) DeleteRoute(ctx context.Context, routeID string) (*pfsense.PfSenseAPIResponse, error) {
	if c.DeleteRouteFunc != nil {
		return c.DeleteRouteFunc(ctx, routeID)
	}
	return ok()
}

func (c *Client) GetStaticRoutes(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	if c.GetStaticRoutesFunc != nil {
		return c.GetStaticRoutesFunc(ctx)
	}
	return okData([]any{})
}

func (c *Client) CreateStaticRoute(ctx context.Context, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
	if c.CreateStaticRouteFunc != nil {
		return c.CreateStaticRouteFunc(ctx, routeData)
	}
	return okData(map[string]any{"id": "static-route-123"})
}

func (c *Client) UpdateStaticRoute(ctx context.Context, routeID string, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
	if c.UpdateStaticRouteFunc != nil {
		return c.UpdateStaticRouteFunc(ctx, routeID, routeData)
	}
	return ok()
}

func (c *Client) DeleteStaticRoute(ctx context.Context, routeID string) (*pfsense.PfSenseAPIResponse, error) {
	if c.DeleteStaticRouteFunc != nil {
		return c.DeleteStaticRouteFunc(ctx, routeID)
	}
	return ok()
}

func (c *Client) ApplyStaticRoutes(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	if c.ApplyStaticRoutesFunc != nil {
		return c.ApplyStaticRoutesFunc(ctx)
	}
	return ok()
}

func (c *Client) UpdateFirewallRule(ctx context.Context, ruleID int, updateData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
	if c.UpdateFirewallRuleFunc != nil {
		return c.UpdateFirewallRuleFunc(ctx, ruleID, updateData)
	}
	return ok()
}

func (c *Client) ApplyFirewall(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	if c.ApplyFirewallFunc != nil {
		return c.ApplyFirewallFunc(ctx)
	}
	return ok()
}

func (c *Client) FlushStates(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	if c.FlushStatesFunc != nil {
		return c.FlushStatesFunc(ctx)
	}
	return ok()
}

func (c *Client) Get(ctx context.Context, path string) (*pfsense.PfSenseAPIResponse, error) {
	if c.GetFunc != nil {
		return c.GetFunc(ctx, path)
	}
	return okData([]any{})
}

func (c *Client) Post(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error) {
	if c.PostFunc != nil {
		return c.PostFunc(ctx, path, data)
	}
	return ok()
}

func (c *Client) Put(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error) {
	if c.PutFunc != nil {
		return c.PutFunc(ctx, path, data)
	}
	return ok()
}

func (c *Client) Patch(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error) {
	if c.PatchFunc != nil {
		return c.PatchFunc(ctx, path, data)
	}
	return ok()
}

func (c *Client) Delete(ctx context.Context, path string) (*pfsense.PfSenseAPIResponse, error) {
	if c.DeleteFunc != nil {
		return c.DeleteFunc(ctx, path)
	}
	return ok()
}

func (c *Client) DeleteWithBody(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error) {
	if c.DeleteWithBodyFunc != nil {
		return c.DeleteWithBodyFunc(ctx, path, data)
	}
	return ok()
}
