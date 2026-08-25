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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"aalyria.com/spacetime/agent/internal/pfsense"
)

// APIClientV2 implements the pfSense APIClientInterface.
type APIClientV2 struct {
	endpoint string
	username string
	apiKey   string
	timeout  time.Duration
	retries  int
	client   *http.Client
}

// NewAPIClientV2 creates a new pfSense API v2 client.
func NewAPIClientV2(config pfsense.Config) pfsense.APIClientInterface {
	// Ensure the endpoint has a proper protocol scheme
	endpoint := config.APIEndpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		// Default to https if no protocol is specified
		endpoint = "https://" + endpoint
	}

	// Configure TLS transport if needed
	var transport http.RoundTripper
	if strings.HasPrefix(endpoint, "https://") {
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS13,
				InsecureSkipVerify: config.InsecureSkipVerify,
			},
		}
	}

	client := &http.Client{
		Timeout: config.Timeout,
	}
	if transport != nil {
		client.Transport = transport
	}

	return &APIClientV2{
		endpoint: endpoint,
		username: config.Username,
		apiKey:   config.APIKey,
		timeout:  config.Timeout,
		retries:  config.Retries,
		client:   client,
	}
}

// TestConnection tests the connection to the pfSense API.
func (c *APIClientV2) TestConnection(ctx context.Context) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Info().Msg("testing connection to pfSense API")

	// Test connection by calling the system status endpoint
	resp, err := c.Get(ctx, "/api/v2/status/system")
	if err != nil {
		log.Error().Err(err).Msg("failed to connect to pfSense API")
		return fmt.Errorf("failed to connect to pfSense API: %w", err)
	}

	// Check if the response indicates success
	if resp.Status != "ok" || resp.Code != 200 {
		log.Error().
			Str("status", resp.Status).
			Int("code", resp.Code).
			Str("message", resp.Message).
			Msg("pfSense API returned error status")
		return fmt.Errorf("pfSense API returned error status: %s", resp.Message)
	}

	log.Info().Msg("successfully connected to pfSense API")
	return nil
}

// GetGateways retrieves gateways from pfSense and returns an IP-to-name mapping.
func (c *APIClientV2) GetGateways(ctx context.Context) (map[string]string, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Info().Msg("fetching gateways from pfSense")

	// Get gateways from pfSense using the correct endpoint
	resp, err := c.Get(ctx, "/api/v2/routing/gateways")
	if err != nil {
		log.Error().Err(err).Msg("failed to get gateways")
		return nil, fmt.Errorf("failed to get gateways: %w", err)
	}

	log.Debug().Msg("received gateways response")

	// Check if the response indicates success
	if resp.Status != "ok" || resp.Code != 200 {
		log.Error().
			Str("status", resp.Status).
			Int("code", resp.Code).
			Str("message", resp.Message).
			Msg("gateways API returned error status")
		return nil, fmt.Errorf("gateways API returned error status: %s", resp.Message)
	}

	log.Debug().Interface("response_data", resp.Data).Msg("gateways response data")

	// Parse the gateways data
	var gateways []any

	// Try different ways to access the gateways data
	if gatewaysArray, ok := resp.Data.([]any); ok {
		gateways = gatewaysArray
		log.Debug().Msg("found gateways as direct array")
	} else if dataMap, ok := resp.Data.(map[string]any); ok {
		if gatewaysArray, ok := dataMap["data"].([]any); ok {
			gateways = gatewaysArray
			log.Debug().Msg("found gateways in data field")
		}
	}

	if gateways == nil {
		log.Error().Interface("data_type", resp.Data).Msg("could not find gateways array in response")
		return nil, fmt.Errorf("could not find gateways array in response, data type: %T", resp.Data)
	}

	log.Debug().Int("gateway_count", len(gateways)).Msg("processing gateways")

	// Create IP-to-name mapping
	ipToGateway := make(map[string]string)

	for i, gateway := range gateways {
		gatewayMap, ok := gateway.(map[string]any)
		if !ok {
			log.Debug().Int("index", i).Interface("gateway", gateway).Msg("gateway is not a map")
			continue
		}

		name, nameExists := gatewayMap["name"]
		gatewayIP, ipExists := gatewayMap["gateway"]

		if !nameExists || !ipExists {
			log.Debug().Int("index", i).Msg("gateway missing name or gateway field")
			continue
		}

		nameStr, ok := name.(string)
		if !ok {
			log.Debug().Int("index", i).Interface("name", name).Msg("name is not a string")
			continue
		}

		ipStr, ok := gatewayIP.(string)
		if !ok {
			log.Debug().Int("index", i).Interface("gateway", gatewayIP).Msg("gateway is not a string")
			continue
		}

		ipToGateway[ipStr] = nameStr
		log.Debug().Str("ip", ipStr).Str("name", nameStr).Msg("added gateway mapping")
	}

	log.Info().Interface("gateway_mapping", ipToGateway).Msg("completed gateway mapping")
	return ipToGateway, nil
}

// GetDefaultGateway retrieves the default IPv4 gateway from pfSense (v2).
func (c *APIClientV2) GetDefaultGateway(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	rlog := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	rlog.Debug().Msg("retrieving default gateway")

	return c.Get(ctx, "/api/v2/routing/gateway/default")
}

// SetDefaultGateway sets the default IPv4 gateway in pfSense (v2).
func (c *APIClientV2) SetDefaultGateway(ctx context.Context, gatewayName string) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Str("gateway_name", gatewayName).Msg("setting default gateway")

	// PATCH body must be {"defaultgw4": "<gateway-name>"}
	body := map[string]string{"defaultgw4": gatewayName, "apply": "true"}
	return c.Patch(ctx, "/api/v2/routing/gateway/default", body)
}

// GetRoutingConfig retrieves the current routing configuration from pfSense.
func (c *APIClientV2) GetRoutingConfig(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Msg("retrieving routing configuration")

	// pfSense routing configuration endpoint (v2)
	return c.Get(ctx, "/api/v2/routing/gateway/")
}

// CreateRoute creates a new route in pfSense.
func (c *APIClientV2) CreateRoute(ctx context.Context, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Interface("route_data", routeData).Msg("creating route")

	return c.Post(ctx, "/api/v2/routing/gateway/", routeData)
}

// DeleteRoute deletes a route from pfSense.
func (c *APIClientV2) DeleteRoute(ctx context.Context, routeID string) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Str("route_id", routeID).Msg("deleting route")

	return c.Delete(ctx, "/api/v2/routing/gateway/"+routeID+"/")
}

// UpdateFirewallRule updates a firewall rule in pfSense.
func (c *APIClientV2) UpdateFirewallRule(ctx context.Context, ruleID int, updateData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Int("rule_id", ruleID).Interface("update_data", updateData).Msg("updating firewall rule")

	path := fmt.Sprintf("/api/v2/firewall/rule?id=%d", ruleID)
	return c.Patch(ctx, path, updateData)
}

// ApplyFirewall applies firewall rule changes in pfSense.
func (c *APIClientV2) ApplyFirewall(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Msg("applying firewall rules")

	return c.Post(ctx, "/api/v2/firewall/apply/", nil)
}

// FlushStates flushes all pfSense firewall states using the command prompt API.
func (c *APIClientV2) FlushStates(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Msg("flushing firewall states")

	// Execute the pfctl command to flush all firewall states
	commandData := map[string]any{
		"command": "pfctl -F state",
	}

	return c.Post(ctx, "/api/v2/diagnostics/command_prompt", commandData)
}

// GetStaticRoutes retrieves all static routes from pfSense.
func (c *APIClientV2) GetStaticRoutes(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Msg("retrieving static routes")

	return c.Get(ctx, "/api/v2/routing/static_routes/")
}

// CreateStaticRoute creates a new static route in pfSense.
func (c *APIClientV2) CreateStaticRoute(ctx context.Context, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Interface("route_data", routeData).Msg("creating static route")

	return c.Post(ctx, "/api/v2/routing/static_route/", routeData)
}

// UpdateStaticRoute updates an existing static route in pfSense in place.
func (c *APIClientV2) UpdateStaticRoute(ctx context.Context, routeID string, routeData map[string]any) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Str("route_id", routeID).Interface("route_data", routeData).Msg("updating static route")

	// PATCH identifies the route to update by its id, sent in the body alongside
	// the fields being changed.
	body := make(map[string]any, len(routeData)+1)
	for k, v := range routeData {
		body[k] = v
	}
	body["id"] = routeID

	return c.Patch(ctx, "/api/v2/routing/static_route/", body)
}

// DeleteStaticRoute deletes a static route from pfSense.
func (c *APIClientV2) DeleteStaticRoute(ctx context.Context, routeID string) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Str("route_id", routeID).Msg("deleting static route")

	// Even though documentation says query parameter, the error suggests pfSense expects it in the body
	// Try sending the ID in the request body instead
	deleteData := map[string]any{
		"id": routeID,
	}

	// Use the original endpoint but send data in body
	// The error "Field 'id' is required" suggests pfSense expects the ID in the request body
	return c.DeleteWithBody(ctx, "/api/v2/routing/static_route/", deleteData)
}

// ApplyStaticRoutes applies static routes in pfSense.
func (c *APIClientV2) ApplyStaticRoutes(ctx context.Context) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Msg("applying static routes")

	return c.Post(ctx, "/api/v2/routing/apply/", nil)
}

// retryBaseBackoff is the delay before the first retry; it doubles each attempt.
const retryBaseBackoff = 200 * time.Millisecond

// do executes an HTTP request against the pfSense API, attaching the shared
// authentication headers. Transport failures are retried (up to c.retries
// additional times, with exponential backoff) only for idempotent GET requests:
// a write (POST/PATCH/PUT/DELETE) may already have been applied by pfSense
// before the transport error surfaced, so retrying it could duplicate a route
// or re-run a command. A completed (non-2xx) HTTP response is returned to the
// caller for status inspection rather than retried.
func (c *APIClientV2) do(ctx context.Context, method, path string, body any) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Str("method", method).Str("path", path).Interface("data", body).Msg("performing request")

	url := c.endpoint + path

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			log.Error().Err(err).Interface("data", body).Msgf("failed to marshal %s body to JSON", method)
			return nil, err
		}
	}

	// Only idempotent GETs are safe to retry after a transport failure; all
	// other methods get a single attempt.
	attempts := 1
	if method == http.MethodGet {
		attempts = c.retries + 1
		if attempts < 1 {
			attempts = 1
		}
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			// Exponential backoff before retrying, aborting promptly if the
			// context is cancelled.
			backoff := retryBaseBackoff << uint(attempt-2)
			select {
			case <-ctx.Done():
				return nil, context.Cause(ctx)
			case <-time.After(backoff):
			}
		}

		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			log.Error().Err(err).Str("url", url).Msgf("failed to create %s request", method)
			return nil, err
		}

		req.Header.Set("X-API-Key", c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		if c.username != "" {
			req.Header.Set("X-Username", c.username)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			// Transport-level failures are transient; retry while attempts remain.
			lastErr = err
			log.Warn().Err(err).Str("url", url).Int("attempt", attempt).Int("attempts", attempts).Msgf("failed to execute %s request", method)
			if ctx.Err() != nil {
				return nil, context.Cause(ctx)
			}
			continue
		}

		parsed, parseErr := c.parseResponse(ctx, resp)
		resp.Body.Close()
		return parsed, parseErr
	}

	return nil, fmt.Errorf("%s %s failed after %d attempt(s): %w", method, path, attempts, lastErr)
}

// Get performs a GET request to the pfSense API.
func (c *APIClientV2) Get(ctx context.Context, path string) (*pfsense.PfSenseAPIResponse, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

// Post performs a POST request to the pfSense API.
func (c *APIClientV2) Post(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error) {
	return c.do(ctx, http.MethodPost, path, data)
}

// Put performs a PUT request to the pfSense API.
func (c *APIClientV2) Put(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error) {
	return c.do(ctx, http.MethodPut, path, data)
}

// Patch performs a PATCH request to the pfSense API.
func (c *APIClientV2) Patch(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error) {
	return c.do(ctx, http.MethodPatch, path, data)
}

// Delete performs a DELETE request to the pfSense API.
func (c *APIClientV2) Delete(ctx context.Context, path string) (*pfsense.PfSenseAPIResponse, error) {
	return c.do(ctx, http.MethodDelete, path, nil)
}

// DeleteWithBody performs a DELETE request to the pfSense API with a request body.
func (c *APIClientV2) DeleteWithBody(ctx context.Context, path string, data any) (*pfsense.PfSenseAPIResponse, error) {
	return c.do(ctx, http.MethodDelete, path, data)
}

// parseResponse parses the HTTP response into a PfSenseAPIResponse.
func (c *APIClientV2) parseResponse(ctx context.Context, resp *http.Response) (*pfsense.PfSenseAPIResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Int("status_code", resp.StatusCode).Msg("failed to read response body")
		return nil, err
	}

	log.Debug().Int("status_code", resp.StatusCode).Str("body", string(body)).Msg("received response")

	// Try to parse as JSON first
	var apiResp pfsense.PfSenseAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		log.Debug().Err(err).Msg("failed to parse response as JSON, treating as plain text")
		// If JSON parsing fails, fall back to the HTTP status. Status is left
		// empty here and derived from the status code below so a non-2xx
		// response with a non-JSON body is not misreported as a success.
		apiResp = pfsense.PfSenseAPIResponse{
			Code:    resp.StatusCode,
			Message: string(body),
		}
	}

	// Set the status code from HTTP response if not set in JSON
	if apiResp.Code == 0 {
		apiResp.Code = resp.StatusCode
	}

	// Set status based on HTTP status code if not set in JSON
	if apiResp.Status == "" {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			apiResp.Status = "ok"
		} else {
			apiResp.Status = "error"
		}
	}

	return &apiResp, nil
}
