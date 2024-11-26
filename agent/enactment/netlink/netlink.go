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
	"log/slog"
	"math"
	"slices"
	"sync"
	"syscall"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	vnl "github.com/vishvananda/netlink"

	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
)

const (
	defaultRtTableID             = 252   // a5a
	defaultRtTableLookupPriority = 25200 // a5a * 100
)

type Driver struct {
	// mu protects the backend's map fields from concurrent mutation.
	mu *sync.Mutex

	routes []*installedRoute
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
			defer func() { slog.Info("RouteList() returned", "routes", routes, "err", err) }()

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
		mu:     &sync.Mutex{},
		routes: []*installedRoute{},
		config: config,
		stats:  &exportedStats{},
	}
}

func (b *Driver) Close() error { return nil }

func (b *Driver) Init(ctx context.Context) error {
	log := zerolog.Ctx(ctx).With().Str("backend", "netlink").Logger()
	ctx = log.WithContext(ctx)

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

	changeIDs := make([]string, 0, len(b.routes))
	for _, r := range b.routes {
		changeIDs = append(changeIDs, r.ID)
	}
	slices.Sort(changeIDs)
	b.stats.InstalledRoutes = changeIDs

	return b.stats
}

func (b *Driver) addRoute(route *vnl.Route) error {
	if err := b.config.RouteAdd(route); err != nil {
		return fmt.Errorf("RouteAdd(%v): %w", route, err)
	}
	return nil
}

func (b *Driver) deleteRoute(route *vnl.Route) error {
	if err := b.config.RouteDel(route); err != nil {
		return fmt.Errorf("RouteDel(%v): %w", route, err)
	}
	return nil
}

func (d *Driver) wantedRoutes() []vnl.Route {
	wantRoutes := []vnl.Route{}
	for _, r := range d.routes {
		wantRoutes = append(wantRoutes, r.ToNetlinkRoutes(d.config)...)
	}
	return wantRoutes
}

func (b *Driver) reconcileRoutes(installedRoutes, wantRoutes []vnl.Route) error {
	toAdd, toRemove := diffRoutes(installedRoutes, wantRoutes)
	added, deleted := 0, 0

	defer func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		b.stats.RoutesSetCount += added
		b.stats.RoutesDeletedCount += deleted
	}()

	for _, route := range toAdd {
		routeWithPriority := route
		routeWithPriority.Priority = int(math.MaxUint32 - uint32(b.config.Clock.Now().Unix()))
		if err := b.addRoute(&routeWithPriority); err != nil && errors.Is(err, syscall.EEXIST) {
		} else if err != nil {
			return fmt.Errorf("SetRoute failed with RTNETLINK-sourced error: %w", err)
		} else {
			added++
		}
	}

	for _, route := range toRemove {
		// First attempt to remove from the system via Netlink
		// If this fails, we do not want to delete the stateful store of routes (which we do next)
		if err := b.deleteRoute(&route); err != nil {
			return fmt.Errorf("DeleteRoute failed with RTNETLINK-sourced error: %w", err)
		}
		deleted++
	}

	return nil
}

func (b *Driver) Dispatch(ctx context.Context, req *schedpb.CreateEntryRequest) error {
	log := zerolog.Ctx(ctx).With().Str("backend", "netlink").Logger()
	ctx = log.WithContext(ctx)

	if req.ConfigurationChange == nil {
		return &NoChangeSpecifiedError{req: req}
	}

	changeID := req.GetId()

	installedRoutes, err := b.routeListFilteredByTableID()
	if err != nil {
		return fmt.Errorf("listing installed routes: %w", err)
	}

	switch cc := req.ConfigurationChange.(type) {
	case *schedpb.CreateEntryRequest_SetRoute:
		ir, err := newInstalledRoute(b.config, changeID, cc.SetRoute)
		if err != nil {
			return fmt.Errorf("constructing route from SetRoute command: %w", err)
		}

		b.mu.Lock()
		replaced := false
		for idx, r := range b.routes {
			if r.ID == changeID {
				replaced = true
				b.routes[idx] = ir
				break
			}
		}
		if !replaced {
			b.routes = append(b.routes, ir)
		}

		wantedRoutes := b.wantedRoutes()
		b.mu.Unlock()

		if err := b.reconcileRoutes(installedRoutes, wantedRoutes); err != nil {
			return fmt.Errorf("installing change %s: %w", changeID, err)
		}

	case *schedpb.CreateEntryRequest_DeleteRoute:
		b.mu.Lock()
		oldLen := len(b.routes)
		b.routes = slices.DeleteFunc(b.routes, func(r *installedRoute) bool {
			slog.Info("DeleteRoute.deleteFunc called", "id", r.ID, "changeID", changeID, "result", r.ID == changeID)
			return r.ID == changeID
		})
		newLen := len(b.routes)
		wantedRoutes := b.wantedRoutes()
		b.mu.Unlock()

		if oldLen == newLen {
			return &UnknownRouteDeleteError{changeID: changeID}
		}

		if err := b.reconcileRoutes(installedRoutes, wantedRoutes); err != nil {
			return fmt.Errorf("installing change %s: %w", changeID, err)
		}

	default:
		log.Warn().Msgf("received CreateEntryRequest with unsupported update type: %v", err)
		return &UnsupportedUpdateError{req: req}
	}

	log.Info().Msgf("Successfully implemented CreateEntryRequest with ID %s and action: %T", req.Id, req.GetConfigurationChange())

	return nil
}
