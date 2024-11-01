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

package netlink

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"slices"
	"sync"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	vnl "github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
)

const (
	defaultRtTableID             = 252   // a5a
	defaultRtTableLookupPriority = 25200 // a5a * 100
)

type routeToRuleID struct {
	route   vnl.Route
	ruleIDs map[string]*schedpb.SetRoute
}

type Driver struct {
	// mu protects the backend's map fields from concurrent mutation.
	mu *sync.Mutex

	// routesToRuleIDs is a list-formatted mapping of route keys to a set of
	// rule ID strings. For every controlled route, there will be an entry in
	// this list. It will have a non-empty list of ruleIDs associated with it.
	// The entire structure and all its contents are protected by the
	// [backend.mu].
	routesToRuleIDs []*routeToRuleID

	config Config

	stats *exportedStats
}

type exportedStats struct {
	InitCount                          int
	LastError                          string
	InstalledRoutes                    []string
	EnactmentFailureCount              int
	RoutesSetCount, RoutesDeletedCount int
}

// Config provides configuration and dependency injection parameters for backend
type Config struct {
	// Clock to support repeatable unit or container testing
	Clock clockwork.Clock

	// The route table number in which destination routes will be managed.
	RtTableID int

	// The Linux PBR priority to use for the lookup into |RtTableID|.
	RtTableLookupPriority int

	// GetLinkIDByName returns the ID of the provided interface.
	GetLinkIDByName func(interfaceID string) (int, error)

	// RouteList fetches a list of installed routes.
	// TODO: Evaluate whether this is still needed
	RouteList func() ([]vnl.Route, error)

	// RouteListFiltered fetches a list of installed routes matching a filter.
	RouteListFiltered func(int, *vnl.Route, uint64) ([]vnl.Route, error)

	// RouteAdd inserts new routes.
	RouteAdd func(*vnl.Route) error

	// RouteDel removes the provided route.
	RouteDel func(*vnl.Route) error

	// RuleAdd is called during the backend Init() process.
	RuleAdd func(*vnl.Rule) error
}

// DefaultConfig generates a nominal Config for New().
// Pass in a Netlink *Handle with the specified namespace, like so:
// nlHandle, err := vnl.NewHandle(vnl.FAMILY_V4)
// config := DefaultConfig(nlHandle)
func DefaultConfig(ctx context.Context, nlHandle *vnl.Handle, rtTableID int, rtTableLookupPriority int) Config {
	log := zerolog.Ctx(ctx).With().Str("backend", "netlink").Logger()

	if rtTableID <= 0 {
		rtTableID = defaultRtTableID
	}
	if rtTableLookupPriority <= 0 {
		rtTableLookupPriority = defaultRtTableLookupPriority
	}

	return Config{
		Clock: clockwork.NewRealClock(),

		RtTableID: rtTableID,

		RtTableLookupPriority: rtTableLookupPriority,

		GetLinkIDByName: func(interfaceID string) (n int, err error) {
			defer func() { log.Debug().Msgf("GetLinkIDByName(%s) returned (%d, %v)", interfaceID, n, err) }()

			link, err := nlHandle.LinkByName(interfaceID)
			if err != nil {
				return 0, fmt.Errorf("failed GetLinkIDByName(%s): %w", interfaceID, err)
			}
			return link.Attrs().Index, nil
		},

		// TODO: Evaluate whether this is still needed
		RouteList: func() (routes []vnl.Route, err error) {
			defer func() { log.Debug().Msgf("RouteList() returned (%v, %v)", routes, err) }()

			// TODO: FAMILY_ALL.
			routes, err = nlHandle.RouteList(nil, vnl.FAMILY_V4)
			if err != nil {
				return nil, fmt.Errorf("failed RouteList(): %w)", err)
			}
			return routes, nil
		},

		RouteListFiltered: func(family int, filter *vnl.Route, filterMask uint64) (routes []vnl.Route, err error) {
			defer func() { log.Debug().Msgf("RouteListFiltered() returned (%v, %v)", routes, err) }()

			// TODO: FAMILY_ALL.
			routes, err = nlHandle.RouteListFiltered(family, filter, filterMask)
			if err != nil {
				return nil, fmt.Errorf("failed RouteList(): %w)", err)
			}
			return routes, nil
		},

		RouteAdd: func(route *vnl.Route) (err error) {
			defer func() { log.Debug().Msgf("RouteAdd(%+v) returned %v", route, err) }()

			return nlHandle.RouteAdd(route)
		},

		RouteDel: func(route *vnl.Route) (err error) {
			defer func() { log.Debug().Msgf("RouteDel(%+v) returned %v", route, err) }()

			return nlHandle.RouteDel(route)
		},

		RuleAdd: func(rule *vnl.Rule) error {
			err := nlHandle.RuleAdd(rule)
			if err != nil {
				return fmt.Errorf("failed RuleAdd(%v): %w)", rule.String(), err)
			}
			return nil
		},
	}
}

// routeListFilteredByTableID is a helper function to return all routes from table <RtTableID>
func (b *Driver) routeListFilteredByTableID() ([]vnl.Route, error) {
	ret, err := b.config.RouteListFiltered(vnl.FAMILY_V4, &vnl.Route{Table: b.config.RtTableID}, vnl.RT_FILTER_TABLE)
	return ret, err
}

// flushExistingRoutesInSpacetimeTable deletes all routes located in the Spacetime route table
// TODO: Should we returns errors here?
func (b *Driver) flushExistingRoutesInSpacetimeTable() error {
	implRoutes, err := b.routeListFilteredByTableID()
	if err != nil {
		return err
	}

	errs := []error{}
	for _, route := range implRoutes {
		err := b.config.RouteDel(&route)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// New is a constructor function which allows you to supply the Config as well
// as a map of any already implemented routes. Before it starts managing
// routes, it flushes the route table <rtTableID> which is dedicated to
// Spacetime activities
func New(config Config) *Driver {
	return &Driver{
		mu:              &sync.Mutex{},
		routesToRuleIDs: []*routeToRuleID{},
		config:          config,
		stats:           &exportedStats{},
	}
}

func (b *Driver) Close() error { return nil }

func (b *Driver) Init(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.stats.InitCount++

	if err := b.flushExistingRoutesInSpacetimeTable(); err != nil {
		return fmt.Errorf("flushExistingRoutesInSpacetimeTable: %w", err)
	}

	for _, family := range []int{vnl.FAMILY_V4, vnl.FAMILY_V6} {
		rule := vnl.NewRule()
		rule.Priority = b.config.RtTableLookupPriority
		rule.Family = family
		rule.Table = b.config.RtTableID
		// TODO: EEXISTS is okay, if the existing rule is the same.
		if err := b.config.RuleAdd(rule); err != nil {
			zerolog.Ctx(ctx).Warn().Err(err).Msgf("RuleAdd failed (do not be [too] alarmed by EEXIST)")
		}
	}

	b.stats.LastError = ""
	b.stats.InstalledRoutes = []string{}
	b.stats.EnactmentFailureCount = 0
	b.stats.RoutesSetCount = 0
	b.stats.RoutesDeletedCount = 0

	return nil
}

func (b *Driver) Stats() interface{} {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.stats
}

// isRouteUpdate validates that the CreateEntryRequest.ConfigurationChange is
// of the supported type (SetRoute | DeleteRoute).
func isRouteUpdate(req *schedpb.CreateEntryRequest) error {
	switch t := req.GetConfigurationChange().(type) {
	case *schedpb.CreateEntryRequest_DeleteRoute, *schedpb.CreateEntryRequest_SetRoute:
		return nil

	default:
		// The update is not for routes
		return fmt.Errorf("unimplemented CreateEntryRequest.ConfigurationChange variant: %T", t)
	}
}

func (b *Driver) buildNetlinkRoute(cc *schedpb.SetRoute) (*vnl.Route, error) {
	var protocol uint32 = 0

	outIfaceIdx, err := b.config.GetLinkIDByName(cc.Dev)
	if err != nil {
		return nil, err
	}
	_, dst, err := net.ParseCIDR(cc.To)
	if err != nil || dst.IP.To4() == nil {
		return nil, &IPv4FormattingError{ipv4: cc.To, sourceField: DstIpRange_Ip}
	}
	nextHopIP := net.ParseIP(cc.Via)
	if nextHopIP == nil {
		return nil, &IPv4FormattingError{ipv4: cc.Via, sourceField: DstIpRange_Ip}
	}

	return &vnl.Route{
		LinkIndex: outIfaceIdx,
		// ILinkIndex: Unused,
		Scope: vnl.SCOPE_UNIVERSE,
		Dst:   dst,
		// Src:   src,
		Gw: nextHopIP,
		// Multipath: Unused,
		Protocol: vnl.RouteProtocol(protocol),
		// Priority:  Unused,
		Family: unix.AF_INET, // IPv4
		Table:  b.config.RtTableID,
		Type:   unix.RTN_UNICAST,
		// Tos:     Unused,
		// Flags:   Unused,
		// MPLSDst: Unused,
		// NewDst:  Unused,
		// Encap:   Unused,
		// Via:     Unused,
		// Realm:   Unused,
		// MTU: 	Unused,
		// Window: 	Unused,
		// Rtt:    	Unused,
		// Remaining are TCP related and will be omitted for brevity
	}, nil
}

// syncCachedRoutes is invoked at the beginning of every operation in order to align the in-memory cache with the real
// state of the Spacetime routing table on the host. In cases where a Spacetime-managed route has been deleted by a party
// or process other than Spacetime, this is caught here and the in-memory cache is updated to reflect it. This updated
// representation of the underlying host is eventually returned to Spacetime, which reprovisions the missing routes if
// neccessary
func (b *Driver) syncCachedRoutes(log zerolog.Logger) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	const filterMask = vnl.RT_FILTER_TABLE + vnl.RT_FILTER_DST + vnl.RT_FILTER_GW + vnl.RT_FILTER_OIF

	errs := []error{}
	b.routesToRuleIDs = slices.DeleteFunc(b.routesToRuleIDs, func(rt *routeToRuleID) bool {
		matchingRoutes, err := b.config.RouteListFiltered(vnl.FAMILY_V4, &rt.route, filterMask)
		if err != nil {
			errs = append(errs, fmt.Errorf("syncCachedRoutes() failed fetching installed routes: %w", err))
			return false
		}

		switch len(matchingRoutes) {
		case 0:
			log.Warn().Msgf("syncCachedRoutes() found a cached route which isn't implemented in Host: %v", rt.route)
			return true
		case 1:
			// Expectation is that there will be at most one route with this
			// signature. If we get here we're good
			return false
		default:
			errs = append(errs, fmt.Errorf("syncCachedRoutes() returned an unexpected number of routes (%d) matching route (%v)", len(matchingRoutes), rt.route))
			return false
		}
	})
	return errors.Join(errs...)
}

func (b *Driver) addRoute(route *vnl.Route) error {
	if err := b.config.RouteAdd(route); err != nil {
		return fmt.Errorf("RouteAdd(%v): %w", route, err)
	}
	return nil
}

func routeEqual(routeA, routeB *vnl.Route) bool {
	return (routeA.Dst == routeB.Dst && routeA.Gw.Equal(routeB.Gw) && routeA.LinkIndex == routeB.LinkIndex)
}

func (b *Driver) deleteRoute(route *vnl.Route) error {
	if err := b.config.RouteDel(route); err != nil {
		return fmt.Errorf("RouteDel(%v): %w", route, err)
	}
	return nil
}

func (b *Driver) Dispatch(ctx context.Context, req *schedpb.CreateEntryRequest) error {
	log := zerolog.Ctx(ctx).With().Str("backend", "netlink").Logger()

	err := b.syncCachedRoutes(log)
	if err != nil {
		return fmt.Errorf("Failed to sync cached routes with implemented routes, with err: %w\n", err)
	}

	if req.ConfigurationChange == nil {
		return &NoChangeSpecifiedError{req}
	}

	changeID := req.GetId()

	switch cc := req.ConfigurationChange.(type) {
	case *schedpb.CreateEntryRequest_SetRoute:
		route, needsAdd, err := func() (*vnl.Route, bool, error) {
			b.mu.Lock()
			defer b.mu.Unlock()

			b.stats.RoutesSetCount++

			route, err := b.buildNetlinkRoute(cc.SetRoute)
			if err != nil {
				return nil, false, err
			}

			needsAdd := true
			for _, r := range b.routesToRuleIDs {
				if r.route.Equal(*route) {
					log.Debug().Msgf("skipping adding route %q because route is already installed", changeID)
					needsAdd = false
					r.ruleIDs[changeID] = cc.SetRoute
				}
			}
			return route, needsAdd, nil
		}()

		if err != nil {
			return fmt.Errorf("SetRoute with ID %q failed: %w", changeID, err)
		}

		if needsAdd {
			// Then add to the system via Netlink
			routeWithPriority := *route
			routeWithPriority.Priority = int(math.MaxUint32 - uint32(b.config.Clock.Now().Unix()))
			if err := b.addRoute(&routeWithPriority); err != nil {
				return fmt.Errorf("SetRoute with ID %q failed with RTNETLINK-sourced error: %w", changeID, err)
			}
		}

		b.mu.Lock()
		if needsAdd {
			b.routesToRuleIDs = append(b.routesToRuleIDs, &routeToRuleID{
				route:   *route,
				ruleIDs: map[string]*schedpb.SetRoute{changeID: cc.SetRoute},
			})
		}
		b.stats.InstalledRoutes = append(b.stats.InstalledRoutes, changeID)
		slices.Sort(b.stats.InstalledRoutes)
		b.stats.InstalledRoutes = slices.Compact(b.stats.InstalledRoutes)

		b.mu.Unlock()

	case *schedpb.CreateEntryRequest_DeleteRoute:
		route, needsDelete, err := func() (route *vnl.Route, needsDelete bool, err error) {
			b.mu.Lock()
			defer b.mu.Unlock()

			b.stats.RoutesDeletedCount++

			// Sanity check delete operation
			var routeToDelete *schedpb.SetRoute
		findRuleProtoLoop:
			for _, rt := range b.routesToRuleIDs {
				for rID, pb := range rt.ruleIDs {
					if rID == changeID {
						routeToDelete = pb
						break findRuleProtoLoop
					}
				}
			}

			if routeToDelete == nil {
				return nil, false, &UnknownRouteDeleteError{changeID}
			}

			route, err = b.buildNetlinkRoute(routeToDelete)
			if err != nil {
				return nil, false, fmt.Errorf("building netlink route (route=%v): %w", routeToDelete, err)
			}

			for _, rt := range b.routesToRuleIDs {
				if _, ok := rt.ruleIDs[changeID]; !ok {
					continue
				}
				if !rt.route.Equal(*route) {
					continue
				}
				needsDelete = needsDelete || len(rt.ruleIDs) == 1
			}

			return route, needsDelete, nil
		}()
		if err != nil {
			return err
		}

		if needsDelete {
			// First attempt to remove from the system via Netlink
			// If this fails, we do not want to delete the stateful store of routes (which we do next)
			if err := b.deleteRoute(route); err != nil {
				return fmt.Errorf("DeleteRoute with ID %q failed with RTNETLINK-sourced error: %w", changeID, err)
			}
		}

		b.mu.Lock()
		b.routesToRuleIDs = slices.DeleteFunc(b.routesToRuleIDs, func(rt *routeToRuleID) bool {
			delete(rt.ruleIDs, changeID)
			if len(rt.ruleIDs) > 0 {
				return false
			}

			log.Debug().Msgf("deleting route %v because the last change that referenced it (%q) has been deleted", route, changeID)
			return true
		})
		b.stats.InstalledRoutes = slices.DeleteFunc(b.stats.InstalledRoutes, func(s string) bool { return s == changeID })
		b.mu.Unlock()

	default:
		log.Warn().Msgf("received CreateEntryRequest with unsupported update type: %v", err)
		return &UnsupportedUpdateError{req}
	}

	log.Info().Msgf("Successfully implemented CreateEntryRequest with ID %s and action: %T", req.Id, req.GetConfigurationChange())

	return nil
}
