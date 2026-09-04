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

// Package pfsense provides an enactment.Driver implementation for pfSense routers.
package pfsense

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"aalyria.com/spacetime/agent/internal/pfsense"
	"aalyria.com/spacetime/agent/internal/pfsense/impl"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
)

// Sentinel errors
var (
	ErrNotInitialized              = errors.New("driver not initialized - call Init() first")
	ErrNoConfigurationSpecified    = errors.New("no configuration change specified")
	ErrUnsupportedConfigChangeType = errors.New("unsupported configuration change type")
)

// Driver implements enactment.Driver for pfSense routers.
type Driver struct {
	mu         sync.RWMutex
	config     Config
	client     pfsense.APIClientInterface
	stats      *Stats
	gatewayMap map[string]string // IP address -> Gateway name mapping
}

// Config holds the configuration for the pfSense enactment driver.
type Config struct {
	// API URL with protocol
	APIEndpoint string

	// Authentication credentials
	Username string
	APIKey   string

	// Clock for testing and timing operations
	Clock clockwork.Clock

	// Timeout for API requests
	Timeout time.Duration

	// Maximum number of retries for API requests
	Retries int

	// Whether to skip SSL verification
	InsecureSkipVerify bool
}

// Stats tracks driver statistics.
type Stats struct {
	InitCount           int
	DispatchCount       int
	SuccessCount        int
	FailureCount        int
	RecentErrors        []string
	LastDispatchTime    time.Time
	TotalDispatchTime   time.Duration
	AverageDispatchTime time.Duration
}

// addError adds an error to the recent errors list, maintaining only the last 5
func (s *Stats) addError(err string) {
	const maxErrors = 5

	// Add the new error to the beginning (most recent first)
	s.RecentErrors = append([]string{err}, s.RecentErrors...)

	// Keep only the last maxErrors
	if len(s.RecentErrors) > maxErrors {
		s.RecentErrors = s.RecentErrors[:maxErrors]
	}
}

// New creates a new pfSense enactment driver.
func New(config Config) *Driver {
	return NewWithClient(config, nil)
}

// NewWithClient creates a new pfSense enactment driver with a custom API client.
// This is primarily used for testing with mock clients.
func NewWithClient(config Config, client pfsense.APIClientInterface) *Driver {
	if config.Clock == nil {
		config.Clock = clockwork.NewRealClock()
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.Retries == 0 {
		config.Retries = 3
	}

	// Use provided client or create a new one
	if client == nil {
		// Create shared API client configuration
		apiConfig := pfsense.Config{
			APIEndpoint:        config.APIEndpoint,
			Username:           config.Username,
			APIKey:             config.APIKey,
			Timeout:            config.Timeout,
			Retries:            config.Retries,
			InsecureSkipVerify: config.InsecureSkipVerify,
		}

		client = impl.NewAPIClientV2(apiConfig)
	}

	return &Driver{
		config: config,
		client: client,
		stats:  &Stats{},
	}
}

// Init initializes the pfSense driver.
func (d *Driver) Init(ctx context.Context) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Info().Msg("initializing pfSense enactment driver")

	d.mu.Lock()
	d.stats.InitCount++
	d.mu.Unlock()

	// Test connection to pfSense API. The network round-trips below are made
	// without holding d.mu so that concurrent Dispatch/Stats callers are not
	// blocked for the (Timeout × Retries) duration of the API calls; the lock
	// is only taken to update stats and publish the gateway map.
	if err := d.client.TestConnection(ctx); err != nil {
		d.mu.Lock()
		d.stats.addError(err.Error())
		d.mu.Unlock()
		log.Error().Err(err).Msg("failed to connect to pfSense API")
		return fmt.Errorf("failed to connect to pfSense API: %w", err)
	}

	// Get gateway mapping
	gatewayMap, err := d.client.GetGateways(ctx)
	if err != nil {
		d.mu.Lock()
		d.stats.addError(err.Error())
		d.mu.Unlock()
		log.Error().Err(err).Msg("failed to get gateways")
		return fmt.Errorf("failed to get gateways: %w", err)
	}

	d.mu.Lock()
	d.gatewayMap = gatewayMap
	d.mu.Unlock()

	log.Info().
		Msg("pfSense enactment driver initialized successfully")

	return nil
}

// Dispatch handles enactment requests for pfSense.
func (d *Driver) Dispatch(ctx context.Context, req *schedpb.CreateEntryRequest) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()

	d.mu.Lock()
	d.stats.DispatchCount++
	startTime := d.config.Clock.Now()
	d.stats.LastDispatchTime = startTime
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		duration := d.config.Clock.Now().Sub(startTime)
		d.stats.TotalDispatchTime += duration
		if d.stats.DispatchCount > 0 {
			d.stats.AverageDispatchTime = d.stats.TotalDispatchTime / time.Duration(d.stats.DispatchCount)
		}
	}()

	log.Debug().
		Interface("request", req).
		Msg("dispatching pfSense enactment request")

	// Check if driver is initialized
	if !d.isInitialized() {
		d.mu.Lock()
		d.stats.FailureCount++
		d.stats.addError(ErrNotInitialized.Error())
		d.mu.Unlock()
		log.Error().Err(ErrNotInitialized).Msg("dispatch failed")
		return status.Errorf(codes.FailedPrecondition, "%v", ErrNotInitialized)
	}

	// Check if ConfigurationChange is set
	if req.ConfigurationChange == nil {
		d.mu.Lock()
		d.stats.FailureCount++
		d.stats.addError(ErrNoConfigurationSpecified.Error())
		d.mu.Unlock()
		log.Error().Err(ErrNoConfigurationSpecified).Msg("dispatch failed")
		return status.Errorf(codes.InvalidArgument, "%v", ErrNoConfigurationSpecified)
	}

	// Handle different types of configuration changes. Only the route-oriented
	// changes are enactable on a pfSense router; everything else (beams, SR-TE
	// policies, ...) is reported as unimplemented rather than silently accepted
	// as a successful enactment.
	var err error
	switch change := req.ConfigurationChange.(type) {
	case *schedpb.CreateEntryRequest_SetRoute:
		err = d.handleSetRoute(ctx, change.SetRoute)
	case *schedpb.CreateEntryRequest_DeleteRoute:
		err = d.handleDeleteRoute(ctx, change.DeleteRoute)
	default:
		d.mu.Lock()
		d.stats.FailureCount++
		d.stats.addError(ErrUnsupportedConfigChangeType.Error())
		d.mu.Unlock()
		log.Error().Interface("change_type", change).Err(ErrUnsupportedConfigChangeType).Msg("dispatch failed")
		return status.Errorf(codes.Unimplemented, "%v", ErrUnsupportedConfigChangeType)
	}

	// Track success/failure based on the result
	d.mu.Lock()
	if err != nil {
		d.stats.FailureCount++
		d.stats.addError(err.Error())
	} else {
		d.stats.SuccessCount++
	}
	d.mu.Unlock()

	return err
}

// handleSetRoute handles SetRoute configuration changes.
func (d *Driver) handleSetRoute(ctx context.Context, setRoute *schedpb.SetRoute) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Info().
		Str("from", setRoute.From).
		Str("to", setRoute.To).
		Str("dev", setRoute.Dev).
		Str("via", setRoute.Via).
		Msg("handling SetRoute request")

	// Get the gateway name for the via IP.
	gatewayName, exists := d.GetGatewayName(setRoute.Via)
	if !exists {
		return status.Errorf(codes.NotFound, "Could not find gateway %s", setRoute.Via)
	}

	// Fetch the current static routes once and reuse them for the existence
	// check and the conflict cleanup below (avoids re-fetching the whole table
	// multiple times per dispatch).
	routes, err := d.getAllStaticRoutes(ctx)
	if err != nil {
		return err
	}

	// Only mutate the config when the exact route is not already present, but
	// always fall through to apply and flush below. A previous dispatch may have
	// written the route to the config while its apply (a POST, which is never
	// retried) failed, leaving the entry in the config but absent from the
	// running table; re-applying unconditionally recovers that case instead of
	// reporting success without ever installing the route.
	if routeExists(routes, setRoute.To, gatewayName) {
		log.Debug().
			Str("gateway", gatewayName).
			Str("network", setRoute.To).
			Msg("static route already exists; re-applying to ensure it is installed")
	} else {
		routeData := map[string]any{
			"network": setRoute.To,
			"gateway": gatewayName,
		}

		// If a route to this destination already exists (via a different
		// gateway), update it in place rather than deleting it first: a delete
		// followed by a failed create (writes are not retried) would leave the
		// destination with no route at all. Any additional duplicate routes to
		// the same destination are removed afterwards. Routes to other
		// destinations that happen to share this gateway are left untouched.
		existingRoutes := findRoutesByNetwork(ctx, routes, setRoute.To)
		if len(existingRoutes) == 0 {
			if err := d.createStaticRoute(ctx, routeData); err != nil {
				return err
			}
		} else {
			if err := d.updateStaticRoute(ctx, existingRoutes[0], routeData); err != nil {
				return err
			}
			for _, routeID := range existingRoutes[1:] {
				if err := d.deleteStaticRouteByID(ctx, routeID, setRoute.To); err != nil {
					return err
				}
			}
		}

		log.Info().
			Str("to", setRoute.To).
			Str("via", setRoute.Via).
			Str("gateway_name", gatewayName).
			Msg("successfully created new static route")
	}

	// Apply routing changes.
	if err := d.applyStaticRoutes(ctx); err != nil {
		return err
	}

	// Flush pfSense firewall states after applying the route so existing
	// stateful connections stop using the previous route.
	return d.flushFirewallStates(ctx)
}

// flushFirewallStates flushes pfSense firewall states so that existing stateful
// connections stop forwarding on routes that were just changed.
func (d *Driver) flushFirewallStates(ctx context.Context) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Msg("flushing pfSense firewall states")

	flushResp, err := d.client.FlushStates(ctx)
	if err != nil {
		log.Error().
			Err(err).
			Msg("failed to flush firewall states")
		return status.Errorf(codes.Unavailable, "failed to flush firewall states: %v", err)
	}

	if flushResp.Status != "ok" || flushResp.Code != 200 {
		log.Error().
			Str("status", flushResp.Status).
			Int("code", flushResp.Code).
			Str("message", flushResp.Message).
			Msg("failed to flush firewall states")
		return status.Errorf(codes.Internal, "failed to flush firewall states: status=%s, code=%d, message=%s", flushResp.Status, flushResp.Code, flushResp.Message)
	}

	log.Info().Msg("successfully flushed firewall states")

	return nil
}

// handleDeleteRoute handles DeleteRoute configuration changes.
func (d *Driver) handleDeleteRoute(ctx context.Context, deleteRoute *schedpb.DeleteRoute) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Info().
		Str("from", deleteRoute.From).
		Str("to", deleteRoute.To).
		Msg("handling DeleteRoute request")

	// Get all static routes
	routes, err := d.getAllStaticRoutes(ctx)
	if err != nil {
		return err
	}

	if routes == nil {
		log.Warn().Msg("no static routes found")
		return nil
	}

	// Find routes that match the destination network
	matchingRoutes := findRoutesByNetwork(ctx, routes, deleteRoute.To)

	if len(matchingRoutes) == 0 {
		log.Warn().Str("destination", deleteRoute.To).Msg("no matching static routes found for deletion")
		return nil
	}

	// Delete all matching routes
	for _, routeID := range matchingRoutes {
		if err := d.deleteStaticRouteByID(ctx, routeID, deleteRoute.To); err != nil {
			return err
		}
	}

	// Apply the routing change so the router stops forwarding on the deleted
	// route, then flush stateful connections that were using it. This mirrors
	// the SetRoute path, which applies and flushes after mutating the table.
	if err := d.applyStaticRoutes(ctx); err != nil {
		return err
	}

	return d.flushFirewallStates(ctx)
}

// isInitialized reports whether Init has populated the gateway map. The read is
// guarded by d.mu because Init may publish the map concurrently with Dispatch.
func (d *Driver) isInitialized() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.gatewayMap != nil
}

// GetGatewayName returns the gateway name for a given IP address
func (d *Driver) GetGatewayName(ip string) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.gatewayMap == nil {
		return "", false
	}

	gatewayName, exists := d.gatewayMap[ip]
	return gatewayName, exists
}

// getAllStaticRoutes retrieves all static routes from pfSense
func (d *Driver) getAllStaticRoutes(ctx context.Context) ([]any, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()

	// Get all static routes
	resp, err := d.client.GetStaticRoutes(ctx)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to get static routes: %w", err)
		log.Error().Err(wrappedErr).Msg("get static routes failed")
		return nil, status.Errorf(codes.Unavailable, "%v", wrappedErr)
	}

	// Check if the get was successful
	if resp.Status != "ok" || resp.Code != 200 {
		wrappedErr := fmt.Errorf("failed to get static routes: status=%s, code=%d, message=%s", resp.Status, resp.Code, resp.Message)
		log.Error().Err(wrappedErr).Msg("get static routes failed")
		return nil, status.Errorf(codes.Internal, "%v", wrappedErr)
	}

	// Parse the routes from the response. A successful response must carry the
	// route table either as a top-level array or under a "data" key; any other
	// shape means the payload could not be parsed. Reporting that as an error
	// (rather than an empty table) keeps DeleteRoute from claiming success while
	// the route is still installed, and SetRoute from creating a duplicate.
	var routes []any
	switch data := resp.Data.(type) {
	case []any:
		routes = data
	case map[string]any:
		routesArray, ok := data["data"].([]any)
		if !ok {
			wrappedErr := fmt.Errorf("failed to parse static routes: no route array under \"data\" key, got %T", data["data"])
			log.Error().Err(wrappedErr).Msg("get static routes failed")
			return nil, status.Errorf(codes.Internal, "%v", wrappedErr)
		}
		routes = routesArray
	default:
		wrappedErr := fmt.Errorf("failed to parse static routes: unexpected response payload type %T", resp.Data)
		log.Error().Err(wrappedErr).Msg("get static routes failed")
		return nil, status.Errorf(codes.Internal, "%v", wrappedErr)
	}

	log.Debug().Interface("parsed_routes", routes).Msg("final parsed routes")
	return routes, nil
}

// extractRouteID returns a static route's "id" field as a string, handling the
// string and numeric (int/float64) encodings the pfSense JSON API may use.
// encoding/json decodes JSON numbers into any as float64, so numeric
// IDs must be handled explicitly.
func extractRouteID(routeMap map[string]any) (string, bool) {
	id, exists := routeMap["id"]
	if !exists {
		return "", false
	}

	switch v := id.(type) {
	case string:
		return v, true
	case int:
		return fmt.Sprintf("%d", v), true
	case float64:
		return fmt.Sprintf("%.0f", v), true
	default:
		return "", false
	}
}

// findRoutesByNetwork finds route IDs that match the specified network (destination).
func findRoutesByNetwork(ctx context.Context, routes []any, network string) []string {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	var matchingRoutes []string

	for i, route := range routes {
		routeMap, ok := route.(map[string]any)
		if !ok {
			continue
		}

		networkStr, ok := routeMap["network"].(string)
		if !ok || networkStr != network {
			continue
		}

		id, ok := extractRouteID(routeMap)
		if !ok {
			log.Debug().Int("route_index", i).Interface("id", routeMap["id"]).Msg("skipping route with unexpected id type")
			continue
		}
		matchingRoutes = append(matchingRoutes, id)
	}

	// Callers delete these IDs sequentially from a single snapshot of the route
	// table. pfSense's REST API v2 exposes each object's id as its index in the
	// config array, so deleting an entry re-indexes every entry after it.
	// Returning the IDs in descending index order means each deletion only
	// shifts entries that have already been processed, keeping the remaining IDs
	// valid.
	slices.SortFunc(matchingRoutes, func(a, b string) int {
		return routeIDIndex(b) - routeIDIndex(a)
	})

	return matchingRoutes
}

// routeIDIndex parses a pfSense static-route id (its index in the config array)
// into an int for ordering. Unparseable ids sort last (deleted last), where they
// cannot re-index a numeric id that still needs deleting.
func routeIDIndex(id string) int {
	n, err := strconv.Atoi(id)
	if err != nil {
		return -1
	}
	return n
}

// deleteStaticRouteByID deletes a single static route by ID.
func (d *Driver) deleteStaticRouteByID(ctx context.Context, routeID string, destination string) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Str("route_id", routeID).Msg("deleting static route")

	deleteResp, err := d.client.DeleteStaticRoute(ctx, routeID)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to delete static route %s: %w", routeID, err)
		log.Error().Err(wrappedErr).Str("route_id", routeID).Msg("delete static route failed")
		return status.Errorf(codes.Unavailable, "%v", wrappedErr)
	}

	// Check if the deletion was successful
	if deleteResp.Status != "ok" || deleteResp.Code != 200 {
		// Handle 404 errors gracefully - the route doesn't exist, which is the desired outcome
		if deleteResp.Code == 404 {
			log.Info().
				Str("status", deleteResp.Status).
				Int("code", deleteResp.Code).
				Str("message", deleteResp.Message).
				Str("route_id", routeID).
				Msg("static route already deleted or doesn't exist - treating as success")
			return nil
		}

		// For other errors, log and return error
		wrappedErr := fmt.Errorf("static route deletion failed: status=%s, code=%d, message=%s", deleteResp.Status, deleteResp.Code, deleteResp.Message)
		log.Error().Err(wrappedErr).
			Str("route_id", routeID).
			Msg("delete static route failed")
		return status.Errorf(codes.Internal, "%v", wrappedErr)
	}

	log.Info().
		Str("route_id", routeID).
		Str("destination", destination).
		Msg("successfully deleted static route")

	return nil
}

// routeExists returns true if a static route to targetNetwork via targetGateway
// already exists among routes.
func routeExists(routes []any, targetNetwork string, targetGateway string) bool {
	for _, route := range routes {
		routeMap, ok := route.(map[string]any)
		if !ok {
			continue
		}

		gatewayStr, ok := routeMap["gateway"].(string)
		if !ok {
			continue
		}

		networkStr, ok := routeMap["network"].(string)
		if !ok {
			continue
		}

		if targetGateway == gatewayStr && targetNetwork == networkStr {
			return true
		}
	}

	return false
}

// createStaticRoute creates a new static route
func (d *Driver) createStaticRoute(ctx context.Context, routeData map[string]any) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Interface("route_data", routeData).Msg("creating new static route")

	resp, err := d.client.CreateStaticRoute(ctx, routeData)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to create static route: %w", err)
		log.Error().Err(wrappedErr).Interface("route_data", routeData).Msg("create static route failed")
		return status.Errorf(codes.Unavailable, "%v", wrappedErr)
	}

	// Check if the creation was successful
	if resp.Status != "ok" || resp.Code != 200 {
		wrappedErr := fmt.Errorf("static route creation failed: status=%s, code=%d, message=%s", resp.Status, resp.Code, resp.Message)
		log.Error().Err(wrappedErr).
			Interface("route_data", routeData).
			Msg("create static route failed")
		return status.Errorf(codes.Internal, "%v", wrappedErr)
	}

	return nil
}

// updateStaticRoute updates an existing static route in place.
func (d *Driver) updateStaticRoute(ctx context.Context, routeID string, routeData map[string]any) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Str("route_id", routeID).Interface("route_data", routeData).Msg("updating static route")

	resp, err := d.client.UpdateStaticRoute(ctx, routeID, routeData)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to update static route: %w", err)
		log.Error().Err(wrappedErr).Str("route_id", routeID).Interface("route_data", routeData).Msg("update static route failed")
		return status.Errorf(codes.Unavailable, "%v", wrappedErr)
	}

	// Check if the update was successful
	if resp.Status != "ok" || resp.Code != 200 {
		wrappedErr := fmt.Errorf("static route update failed: status=%s, code=%d, message=%s", resp.Status, resp.Code, resp.Message)
		log.Error().Err(wrappedErr).
			Str("route_id", routeID).
			Interface("route_data", routeData).
			Msg("update static route failed")
		return status.Errorf(codes.Internal, "%v", wrappedErr)
	}

	return nil
}

// applyStaticRoutes applies the routing change
func (d *Driver) applyStaticRoutes(ctx context.Context) error {
	log := zerolog.Ctx(ctx).With().Str("driver", "pfsense").Logger()
	log.Debug().Msg("applying static routes")

	resp, err := d.client.ApplyStaticRoutes(ctx)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to apply routing changes: %w", err)
		log.Error().Err(wrappedErr).Msg("apply static routes failed")
		return status.Errorf(codes.Unavailable, "%v", wrappedErr)
	}

	if resp.Status != "ok" || resp.Code != 200 {
		wrappedErr := fmt.Errorf("failed to apply routing changes: status=%s, code=%d, message=%s", resp.Status, resp.Code, resp.Message)
		log.Error().Err(wrappedErr).Msg("apply static routes failed")
		return status.Errorf(codes.Internal, "%v", wrappedErr)
	}

	return nil
}

// Stats returns driver statistics.
func (d *Driver) Stats() any {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return *d.stats
}

// Close closes the pfSense driver.
func (d *Driver) Close() error {
	return nil
}
