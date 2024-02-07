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
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"

	apipb "aalyria.com/spacetime/api/common"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	vnl "github.com/vishvananda/netlink"
)

const (
	defaultRtTableID             = 252   // a5a
	defaultRtTableLookupPriority = 25200 // a5a * 100
)

type routeToRuleID struct {
	route   vnl.Route
	ruleIDs map[string]*apipb.FlowRule
}

type Backend struct {
	// mu protects the backend's map fields from concurrent mutation.
	mu *sync.Mutex

	// routesToRuleIDs is a list-formatted mapping of route keys to a set of
	// rule ID strings. For every controlled route, there will be an entry in
	// this list. It will have a non-empty list of ruleIDs associated with it.
	// The entire structure and all its contents, like the cdpiFlowRules map,
	// are protected by the [backend.mu].
	routesToRuleIDs []*routeToRuleID

	config Config

	stats *exportedStats
}

type exportedStats struct {
	InitCount                                  int
	LastError                                  string
	InstalledFlowRules                         []string
	InstalledUniqueRoutes                      []string
	EnactmentFailureCount                      int
	FlowRulesAddedCount, FlowRulesDeletedCount int
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
func (b *Backend) routeListFilteredByTableID() ([]vnl.Route, error) {
	ret, err := b.config.RouteListFiltered(vnl.FAMILY_V4, &vnl.Route{Table: b.config.RtTableID}, vnl.RT_FILTER_TABLE)
	return ret, err
}

// flushExistingRoutesInSpacetimeTable deletes all routes located in the Spacetime route table
// TODO: Should we returns errors here?
func (b *Backend) flushExistingRoutesInSpacetimeTable() error {
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

// New is a constructor function which allows you to supply the Config as well as a map of any already implemented FlowRules\
// Before it starts managing routes, it flushes the route table <rtTableID> which is dedicated to Spacetime activities
func New(config Config) *Backend {
	return &Backend{
		mu:              &sync.Mutex{},
		routesToRuleIDs: []*routeToRuleID{},
		config:          config,
		stats:           &exportedStats{},
	}
}

func (b *Backend) Close() error { return nil }

func (b *Backend) Init(ctx context.Context) error {
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
	b.stats.InstalledFlowRules = []string{}
	b.stats.InstalledUniqueRoutes = []string{}
	b.stats.EnactmentFailureCount = 0
	b.stats.FlowRulesAddedCount = 0
	b.stats.FlowRulesDeletedCount = 0

	return nil
}

func (b *Backend) Stats() interface{} {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.stats
}

// isFlowUpdate validates that the ScheduledControlUpdate's Change is of the supported type (FlowUpdate).
func isFlowUpdate(req *apipb.ScheduledControlUpdate) error {
	switch t := req.GetChange().UpdateType.(type) {
	case *apipb.ControlPlaneUpdate_FlowUpdate:
		return nil
	default:
		// The update is not of type FlowUpdate
		return fmt.Errorf("unimplemented ScheduledControlUpdate.Change.UpdateType variant: %T", t)
	}
}

// flowRuleMatchesInstalledRoute does an equality check between classification parameters for a FlowRule, and a netlink-fetched route
// DO NOT COMMIT: What are a sufficient set of fields for equality check?
func (b *Backend) flowRuleMatchesInstalledRoute(flowRule *apipb.FlowRule, route vnl.Route) bool {
	_, dstSubnet, _ := net.ParseCIDR(flowRule.GetClassifier().GetIpHeader().GetDstIpRange())
	// src := net.ParseIP(flowRule.GetClassifier().GetIpHeader().GetSrcIpRange())
	if dstSubnet.String() != route.Dst.String() {
		return false
	}

	var oifname, nextHopIP string

	// We expect there to only ever be a single action inside of a single
	// action bucket. If that changes, this assumption will break!!!!!
outerLoop:
	for _, b := range flowRule.GetActionBucket() {
		for _, act := range b.GetAction() {
			oifname = act.GetForward().GetOutInterfaceId()
			nextHopIP = act.GetForward().GetNextHopIp()
			break outerLoop
		}
	}

	// TODO: we don't enforce CIDR elsewhere and this won't work
	// without a slash suffix, so this is fragile if the inputs don't specify
	// the CIDR length
	ip, _, err := net.ParseCIDR(nextHopIP)
	if err != nil || !route.Gw.Equal(ip) {
		return false
	}

	idx, err := b.config.GetLinkIDByName(oifname)
	if err != nil || route.LinkIndex != idx {
		return false
	}

	return true
}

// buildStateProtoLocked scans through the b.routesToRuleIDs and compares
// against netlink installed routes. For any matches, the ruleIDs are
// accumulated in the return value ControlPlaneState.
//
// This function acquires the [backend.mu], hence the "Unlocked" suffix.
func (b *Backend) buildStateProtoUnlocked(installedRouteProvider func() ([]vnl.Route, error)) (*apipb.ControlPlaneState, error) {
	installedRoutes, err := installedRouteProvider()
	if err != nil {
		return nil, err
	}

	// Will accumulate installed ruleIDs when installedRouteProvider is matched to routesToRuleIDs.
	ruleIDs := []string{}
	routes := []string{}

	b.mu.Lock()
	defer b.mu.Unlock()

	// TODO: this is simply a brute force matching. If and when we have
	// `cdpi_agent`s with large volumes of routes, we may want to use a more
	// intelligent matching algorithm.
	for _, rt := range b.routesToRuleIDs {
		for ruleID, flowRulePB := range rt.ruleIDs {
			for _, installedRoute := range installedRoutes {
				if b.flowRuleMatchesInstalledRoute(flowRulePB, installedRoute) {
					ruleIDs = append(ruleIDs, ruleID)
				}
				routes = append(routes, installedRoute.String())
			}
		}
	}

	b.stats.InstalledFlowRules = ruleIDs
	b.stats.InstalledUniqueRoutes = routes

	return &apipb.ControlPlaneState{
		ForwardingState: &apipb.FlowState{
			Timestamp:   timeToProto(b.config.Clock.Now()),
			FlowRuleIds: ruleIDs,
		},
	}, nil
}

func timeToProto(t time.Time) *apipb.DateTime {
	return &apipb.DateTime{
		UnixTimeUsec: proto.Int64(t.UnixMicro()),
	}
}

// buildReturnUnlocked executes the common pattern of "collect state and return any errors" for multiple exits from handleRequest
func (b *Backend) buildReturnUnlocked(log zerolog.Logger, installedRouteProvider func() ([]vnl.Route, error), andErr error) (*apipb.ControlPlaneState, error) {
	errs := []error{andErr}
	cpState, err := b.buildStateProtoUnlocked(installedRouteProvider)
	if err != nil {
		errs = append(errs, err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.stats.InstalledFlowRules = cpState.GetForwardingState().GetFlowRuleIds()

	if combinedErr := errors.Join(errs...); combinedErr != nil {
		b.stats.LastError = combinedErr.Error()
		b.stats.EnactmentFailureCount++

		log.Err(combinedErr)
		return cpState, combinedErr
	}
	return cpState, nil
}

// forwardActionFromFlowRule takes the FlowRules and retrieves the OutInterfaceId and/or NextHopIp
// assuming that the FLowRule contains an Action of ActionType Forward
func (b *Backend) forwardActionFromFlowRule(flowRule *apipb.FlowRule) (int, net.IP, error) {
	// DO NOT COMMIT - Form a plan for enforcing only one ActionBucket and one Action for now.
	for _, actionBucket := range flowRule.GetActionBucket() {
		for _, action := range actionBucket.GetAction() {
			switch action.ActionType.(type) {
			case *apipb.FlowRule_ActionBucket_Action_Forward_:
				nextHopIpString := action.GetForward().GetNextHopIp()

				// Make sure that IPs both with and without prefixes are parsed
				// e.g. both 192.168.200.1 and 192.168.200.1/32
				ip := net.ParseIP(nextHopIpString)
				if ip == nil {
					ip2, _, err := net.ParseCIDR(nextHopIpString)
					if err != nil {
						return 0, nil, err
					}
					ip = ip2
				}

				// if nextHopIp == nil || nextHopIp.To4() == nil {
				// 	return 0, nil, &IPv4FormattingError{ipv4: nextHopIpString, sourceField: NextHopIp_Ip}
				//

				outIfaceIdx, err := b.config.GetLinkIDByName(action.GetForward().GetOutInterfaceId())
				if err != nil {
					return 0, nil, &OutInterfaceIdxError{wrongIface: action.GetForward().GetOutInterfaceId(), sourceError: err}
				}

				if ip == nil && outIfaceIdx < 0 {
					return 0, nil, fmt.Errorf("Supplied neither nextHopIP nor outInterfaceId")
				}

				return outIfaceIdx, ip, nil
			}
		}
	}
	return 0, nil, &UnsupportedActionTypeError{}
}

// classifierFromFlowRule retrieves the PacketClassifier details from the FlowRule, while making sure
// that the fiels which are necessary are present, or otherwise throwing the appropriate error
func classifierFromFlowRule(flowRule *apipb.FlowRule) (uint32, net.IP, *net.IPNet, error) {
	if flowRule.GetClassifier().IpHeader == nil {
		return 0, nil, nil, &ClassifierError{missingField: IpHeader_Field}
	}

	var protocol uint32 = 0
	if flowRule.GetClassifier().GetIpHeader().Protocol != nil {
		protocol = flowRule.GetClassifier().GetIpHeader().GetProtocol()
	}

	if flowRule.GetClassifier().GetIpHeader().DstIpRange == nil {
		return 0, nil, nil, &ClassifierError{missingField: DstIpRange_Field}
	}
	dstString := flowRule.GetClassifier().GetIpHeader().GetDstIpRange()
	_, dst, err := net.ParseCIDR(dstString)
	if err != nil || dst.IP.To4() == nil {
		return 0, nil, nil, &IPv4FormattingError{ipv4: dstString, sourceField: DstIpRange_Ip}
	}

	// if flowRule.GetClassifier().GetIpHeader().SrcIpRange == nil {
	// 	return 0, nil, nil, &ClassifierError{missingField: SrcIpRange_Field}
	// }
	// srcString := flowRule.GetClassifier().GetIpHeader().GetSrcIpRange()
	// src := net.ParseIP(srcString)
	// if src == nil || src.To4() == nil {
	// 	return 0, nil, nil, &IPv4FormattingError{ipv4: srcString, sourceField: SrcIpRange_Ip}
	// }

	return protocol, nil, dst, nil
}

func (b *Backend) buildNetlinkRoute(flowRule *apipb.FlowRule) (*vnl.Route, error) {
	outIfaceIdx, nextHopIp, err := b.forwardActionFromFlowRule(flowRule)
	if err != nil {
		return nil, err
	}

	protocol, _, dst, err := classifierFromFlowRule(flowRule)
	if err != nil {
		return nil, err
	}

	route := &vnl.Route{
		LinkIndex: outIfaceIdx,
		// ILinkIndex: Unused,
		Scope: vnl.SCOPE_UNIVERSE,
		Dst:   dst,
		// Src:   src,
		Gw: nextHopIp,
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
	}

	return route, nil
}

// syncCachedRoutes is invoked at the beginning of every operation in order to align the in-memory cache with the real
// state of the Spacetime routing table on the host. In cases where a Spacetime-managed route has been deleted by a party
// or process other than Spacetime, this is caught here and the in-memory cache is updated to reflect it. This updated
// representation of the underlying host is eventually returned to Spacetime, which reprovisions the missing routes if
// neccessary
func (b *Backend) syncCachedRoutes(log zerolog.Logger) error {
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
			errs = append(errs, fmt.Errorf("syncCachedRoutes() returned an unexpected number of routes (%v) matching route (%v)", len(matchingRoutes), rt.route))
			return false
		}
	})
	return errors.Join(errs...)
}

func (b *Backend) flowUpdateAdd(route *vnl.Route) error {
	if err := b.config.RouteAdd(route); err != nil {
		return fmt.Errorf("RouteAdd(%v): %w", route, err)
	}
	return nil
}

func routeEqual(routeA, routeB *vnl.Route) bool {
	return (routeA.Dst == routeB.Dst && routeA.Gw.Equal(routeB.Gw) && routeA.LinkIndex == routeB.LinkIndex)
}

func (b *Backend) flowUpdateDelete(route *vnl.Route) error {
	if err := b.config.RouteDel(route); err != nil {
		return fmt.Errorf("RouteDel(%v): %w", route, err)
	}
	return nil
}

func (b *Backend) Apply(ctx context.Context, req *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
	// DO NOT COMMIT: Questions
	// Do we need to validate sequence_number in any way, or is that done previous to handleRequest?
	// log.Debug().Msgf("UPDATE,%s,%s,%s,%s,%s\n",
	// 	req.GetChange().GetFlowUpdate().GetOperation().Enum(),
	// 	req.GetChange().GetFlowUpdate().GetFlowRuleId(),
	// 	req.GetChange().GetFlowUpdate().GetRule().GetClassifier().GetIpHeader().GetDstIpRange(),
	// 	req.GetChange().GetFlowUpdate().GetRule().GetActionBucket()[0].GetAction()[0].GetForward().GetOutInterfaceId(),
	// 	req.GetChange().GetFlowUpdate().GetRule().GetActionBucket()[0].GetAction()[0].GetForward().GetNextHopIp(),
	// )
	log := zerolog.Ctx(ctx).With().Str("backend", "netlink").Logger()

	err := b.syncCachedRoutes(log)
	if err != nil {
		return b.buildReturnUnlocked(log, b.routeListFilteredByTableID,
			fmt.Errorf("Failed to sync cached routes with implemented routes, with err: %w\n", err))
	}

	if req.Change == nil {
		return b.buildReturnUnlocked(log, b.routeListFilteredByTableID, &NoChangeSpecifiedError{})
	}

	// Current implementation only applies FlowUpdates - warn accordingly
	if err := isFlowUpdate(req); err != nil {
		log.Warn().Msgf("received ScheduledControlUpdate with unsupported update type: %v", err)
		return b.buildReturnUnlocked(log, b.routeListFilteredByTableID, &UnsupportedUpdateError{req})
	}

	// Update the state of our cdpiFlowRules to reflect new actions, if any
	flowRuleID := req.GetChange().GetFlowUpdate().GetFlowRuleId()
	operation := req.GetChange().GetFlowUpdate().GetOperation()
	flowRule := req.GetChange().GetFlowUpdate().GetRule()

	log.Trace().Msgf("got new request with flow rule ID %q | op %v | flowRule %v", flowRuleID, operation, flowRule)
	switch operation {
	case apipb.FlowUpdate_ADD:
		route, needsAdd, err := func() (*vnl.Route, bool, error) {
			b.mu.Lock()
			defer b.mu.Unlock()

			b.stats.FlowRulesAddedCount++

			route, err := b.buildNetlinkRoute(flowRule)
			if err != nil {
				return nil, false, err
			}

			needsAdd := true
			for _, r := range b.routesToRuleIDs {
				if r.route.Equal(*route) {
					log.Debug().Msgf("skipping adding flow rule %q because route is already installed", flowRuleID)
					fmt.Printf("skipping adding flow rule %q because route is already installed\n", flowRuleID)
					needsAdd = false
					r.ruleIDs[flowRuleID] = flowRule
				}
			}
			return route, needsAdd, nil
		}()

		if err != nil {
			return b.buildReturnUnlocked(log, b.routeListFilteredByTableID,
				fmt.Errorf("FlowUpdate with flowRuleID (%v) failed with error: %w", flowRuleID, err))
		}

		if needsAdd {
			// Then add to the system via Netlink
			routeWithPriority := *route
			routeWithPriority.Priority = int(math.MaxUint32 - uint32(b.config.Clock.Now().Unix()))
			if err := b.flowUpdateAdd(&routeWithPriority); err != nil {
				wrappedErr := fmt.Errorf("FlowUpdate(ADD) with flowRuleID (%s) failed with RTNETLINK-sourced error: %w", flowRuleID, err)
				return b.buildReturnUnlocked(log, b.routeListFilteredByTableID, wrappedErr)
			}
		}

		b.mu.Lock()
		if needsAdd {
			b.routesToRuleIDs = append(b.routesToRuleIDs, &routeToRuleID{
				route: *route,
				ruleIDs: map[string]*apipb.FlowRule{
					flowRuleID: flowRule,
				},
			})
		}
		b.mu.Unlock()

	case apipb.FlowUpdate_DELETE:
		route, needsDelete, err := func() (route *vnl.Route, needsDelete bool, err error) {
			b.mu.Lock()
			defer b.mu.Unlock()

			b.stats.FlowRulesDeletedCount++

			// Sanity check delete operation
			flowRule := (*apipb.FlowRule)(nil)
		findRuleProtoLoop:
			for _, rt := range b.routesToRuleIDs {
				for rID, pb := range rt.ruleIDs {
					if rID == flowRuleID {
						flowRule = pb
						break findRuleProtoLoop
					}
				}
			}

			if flowRule == nil {
				return nil, false, &UnknownFlowRuleDeleteError{flowRuleID}
			}

			route, err = b.buildNetlinkRoute(flowRule)
			if err != nil {
				return nil, false, fmt.Errorf("building netlink route (flowRule=%v): %w", flowRule, err)
			}

			for _, rt := range b.routesToRuleIDs {
				if _, ok := rt.ruleIDs[flowRuleID]; !ok {
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
			return b.buildReturnUnlocked(log, b.routeListFilteredByTableID, err)
		}

		if needsDelete {
			// First attempt to remove from the system via Netlink
			// If this fails, we do not want to delete the stateful store of CDPI FlowUpdates (which we do next)
			if err := b.flowUpdateDelete(route); err != nil {
				wrappedErr := fmt.Errorf("FlowUpdate(DELETE) with flowRuleID (%s) failed with RTNETLINK-sourced error: %w", flowRuleID, err)

				return b.buildReturnUnlocked(log, b.routeListFilteredByTableID, wrappedErr)
			}
		}

		b.mu.Lock()
		b.routesToRuleIDs = slices.DeleteFunc(b.routesToRuleIDs, func(rt *routeToRuleID) bool {
			delete(rt.ruleIDs, flowRuleID)
			if len(rt.ruleIDs) > 0 {
				return false
			}

			log.Debug().Msgf("deleting route %v because the last flow rule that referenced it (%q) has been deleted", route, flowRuleID)
			return true
		})
		b.mu.Unlock()

	default:
		wrappedErr := fmt.Errorf("FlowUpdate with flowRuleID (%s) failed with error: %w", flowRuleID, &UnrecognizedFlowUpdateOperationError{operation})
		return b.buildReturnUnlocked(log, b.routeListFilteredByTableID, wrappedErr)
	}

	// Collect and return final state
	log.Info().Msgf("Successfully implemented route with flowRule: %s, action: %s", flowRuleID, operation.String())

	return b.buildReturnUnlocked(log, b.routeListFilteredByTableID, nil)
}
