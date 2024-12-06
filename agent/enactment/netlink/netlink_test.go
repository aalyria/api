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
	"net"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	vnl "github.com/vishvananda/netlink"
	"google.golang.org/protobuf/testing/protocmp"

	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
)

const AGENT_TABLE_ID = 252

func mustParseCIDR(cidr string) (net.IP, *net.IPNet) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return ip, ipNet
}

func ipNetOf(_ net.IP, ipNet *net.IPNet) *net.IPNet {
	return ipNet
}

func TestNetlink(t *testing.T) {
	// t.Parallel()

	type routesError struct {
		routes []vnl.Route
		err    error
	}

	var EMPTY_ROUTE_TABLE = routesError{routes: []vnl.Route{}, err: nil}

	type routeError struct {
		route vnl.Route
		err   error
	}

	type linkIdxError struct {
		idx int
		err error
	}

	type testCase struct {
		name            string
		routeList       []routesError
		routeAdd        []routeError
		routeDel        []routeError
		getLinkIDByName map[string]linkIdxError
		configChanges   []*schedpb.CreateEntryRequest
		wantRoutes      [][]*installedRoute
		wantErrors      []error
	}

	_, dst := mustParseCIDR("192.168.1.0/24")
	gw, gwNet := mustParseCIDR("192.168.1.1/32")

	testCases := []testCase{
		{
			name: "add IPv4 unicast route",
			routeList: []routesError{
				EMPTY_ROUTE_TABLE, // AF_INET routes at start of Dispatch
				EMPTY_ROUTE_TABLE, // AF_INET6 routes at start of Dispatch
			},
			routeAdd: []routeError{
				{
					vnl.Route{
						Dst:       gwNet,
						LinkIndex: 7,
						Table:     AGENT_TABLE_ID,
						Scope:     vnl.SCOPE_LINK,
					},
					nil,
				},
				{
					vnl.Route{
						Src:       nil, // no support for src-dst routing yet
						Dst:       dst,
						Gw:        gw,
						LinkIndex: 7,
						Table:     AGENT_TABLE_ID,
						Scope:     vnl.SCOPE_UNIVERSE,
					},
					nil,
				},
			},
			routeDel: []routeError{},
			getLinkIDByName: map[string]linkIdxError{
				"foo": {idx: 7, err: nil},
			},
			configChanges: []*schedpb.CreateEntryRequest{
				{
					Id:    "rule1",
					Seqno: 1,
					ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
						SetRoute: &schedpb.SetRoute{
							From: "192.168.2.2",
							To:   "192.168.1.0/24",
							Via:  "192.168.1.1",
							Dev:  "foo",
						},
					},
				},
			},
			wantRoutes: [][]*installedRoute{{
				{
					ID:      "rule1",
					DevName: "foo",
					DevID:   7,
					To:      dst,
					Via:     gw,
				},
			}},
			wantErrors: []error{nil},
		},
		{
			name: "add and remove IPv4 unicast route",
			routeList: []routesError{
				EMPTY_ROUTE_TABLE, // AF_INET routes at start of Dispatch
				EMPTY_ROUTE_TABLE, // AF_INET6 routes at start of Dispatch
				{
					// AF_INET routes at end of first Dispatch
					routes: []vnl.Route{{
						Src:       nil, // no support for src-dst routing yet
						Dst:       dst,
						Gw:        net.ParseIP("192.168.1.1"),
						Table:     AGENT_TABLE_ID,
						Scope:     vnl.SCOPE_UNIVERSE,
						LinkIndex: 7,
					}},
					err: nil,
				},
				EMPTY_ROUTE_TABLE, // AF_INET6 routes at end of first Dispatch
			},
			routeAdd: []routeError{
				{
					vnl.Route{
						Dst:       gwNet,
						LinkIndex: 7,
						Table:     AGENT_TABLE_ID,
						Scope:     vnl.SCOPE_LINK,
					},
					nil,
				},
				{
					vnl.Route{
						Src:       nil, // no support for src-dst routing yet
						Dst:       dst,
						Gw:        gw,
						LinkIndex: 7,
						Table:     AGENT_TABLE_ID,
						Scope:     vnl.SCOPE_UNIVERSE,
					},
					nil,
				},
			},
			routeDel: []routeError{
				{
					vnl.Route{
						Dst:       dst,
						Gw:        gw,
						LinkIndex: 7,
						Table:     AGENT_TABLE_ID,
						Scope:     vnl.SCOPE_UNIVERSE,
					},
					nil,
				},
				// TODO: should delete the on-link gateway route as well.
				// {vnl.Route{}, nil},
			},
			getLinkIDByName: map[string]linkIdxError{
				"foo": {idx: 7, err: nil},
			},
			configChanges: []*schedpb.CreateEntryRequest{
				{
					Id:    "rule1",
					Seqno: 1,
					ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
						SetRoute: &schedpb.SetRoute{
							From: "192.168.2.2",
							To:   "192.168.1.0/24",
							Dev:  "foo",
							Via:  "192.168.1.1",
						},
					},
				},
				{
					Id:    "rule1",
					Seqno: 2,
					ConfigurationChange: &schedpb.CreateEntryRequest_DeleteRoute{
						DeleteRoute: &schedpb.DeleteRoute{
							From: "192.168.2.2",
							To:   "192.168.1.0/24",
						},
					},
				},
			},
			wantRoutes: [][]*installedRoute{
				{
					{
						ID:      "rule1",
						DevName: "foo",
						DevID:   7,
						To:      dst,
						Via:     gw,
					},
				},
				{},
			},
			wantErrors: []error{nil, nil},
		},
		{
			name: "remove nonexisting IPv4 unicast route",
			routeList: []routesError{
				EMPTY_ROUTE_TABLE, // AF_INET routes at start of Dispatch
				EMPTY_ROUTE_TABLE, // AF_INET6 routes at start of Dispatch
			},
			routeAdd: []routeError{},
			routeDel: []routeError{},

			getLinkIDByName: map[string]linkIdxError{
				"foo": {idx: 7, err: nil},
			},
			configChanges: []*schedpb.CreateEntryRequest{
				{
					Id:    "rule1",
					Seqno: 2,
					ConfigurationChange: &schedpb.CreateEntryRequest_DeleteRoute{
						DeleteRoute: &schedpb.DeleteRoute{
							From: "192.168.2.2",
							To:   "192.168.1.0/24",
						},
					},
				},
			},
			wantRoutes: [][]*installedRoute{
				{},
			},
			wantErrors: []error{&UnknownRouteDeleteError{changeID: "rule1"}},
		},
		{
			name:            "attempt to perform CreateEntryRequest with no Change",
			routeList:       []routesError{},
			routeAdd:        []routeError{},
			routeDel:        []routeError{},
			getLinkIDByName: map[string]linkIdxError{},
			configChanges: []*schedpb.CreateEntryRequest{
				{
					Id:                  "rule1",
					Seqno:               1,
					ConfigurationChange: nil,
				},
			},
			wantRoutes: [][]*installedRoute{
				{},
			},
			wantErrors: []error{&NoChangeSpecifiedError{&schedpb.CreateEntryRequest{Id: "rule1", Seqno: 1}}},
		},
		{
			name: "add IPv6 unicast route (GUA next hop)",
			routeList: []routesError{
				EMPTY_ROUTE_TABLE, // AF_INET routes at start of Dispatch
				EMPTY_ROUTE_TABLE, // AF_INET6 routes at start of Dispatch
			},
			routeAdd: []routeError{
				{
					vnl.Route{
						Dst:       ipNetOf(mustParseCIDR("2001:db8:0:2::1/128")),
						LinkIndex: 7,
						Table:     AGENT_TABLE_ID,
						Scope:     vnl.SCOPE_LINK,
					},
					nil,
				},
				{
					vnl.Route{
						Src:       nil, // no support for src-dst routing yet
						Dst:       ipNetOf(mustParseCIDR("2001:db8:0:3::/64")),
						Gw:        net.ParseIP("2001:db8:0:2::1"),
						LinkIndex: 7,
						Table:     AGENT_TABLE_ID,
						Scope:     vnl.SCOPE_UNIVERSE,
					},
					nil,
				},
			},
			routeDel: []routeError{},
			getLinkIDByName: map[string]linkIdxError{
				"foo": {idx: 7, err: nil},
			},
			configChanges: []*schedpb.CreateEntryRequest{
				{
					Id:    "rule1",
					Seqno: 1,
					ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
						SetRoute: &schedpb.SetRoute{
							From: "2001:db8:0:1::/64",
							To:   "2001:db8:0:3::/64",
							Via:  "2001:db8:0:2::1",
							Dev:  "foo",
						},
					},
				},
			},
			wantRoutes: [][]*installedRoute{{
				{
					ID:      "rule1",
					DevName: "foo",
					DevID:   7,
					To:      ipNetOf(mustParseCIDR("2001:db8:0:3::/64")),
					Via:     net.ParseIP("2001:db8:0:2::1"),
				},
			}},
			wantErrors: []error{nil},
		},
		{
			name: "add IPv6 unicast route (LLUA next hop)",
			routeList: []routesError{
				EMPTY_ROUTE_TABLE, // AF_INET routes at start of Dispatch
				EMPTY_ROUTE_TABLE, // AF_INET6 routes at start of Dispatch
			},
			routeAdd: []routeError{
				{
					vnl.Route{
						Src:       nil, // no support for src-dst routing yet
						Dst:       ipNetOf(mustParseCIDR("2001:db8:0:3::/64")),
						Gw:        net.ParseIP("fe80::1"),
						LinkIndex: 7,
						Table:     AGENT_TABLE_ID,
						Scope:     vnl.SCOPE_UNIVERSE,
					},
					nil,
				},
			},
			routeDel: []routeError{},
			getLinkIDByName: map[string]linkIdxError{
				"foo": {idx: 7, err: nil},
			},
			configChanges: []*schedpb.CreateEntryRequest{
				{
					Id:    "rule1",
					Seqno: 1,
					ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
						SetRoute: &schedpb.SetRoute{
							From: "2001:db8:0:1::/64",
							To:   "2001:db8:0:3::/64",
							Via:  "fe80::1",
							Dev:  "foo",
						},
					},
				},
			},
			wantRoutes: [][]*installedRoute{{
				{
					ID:      "rule1",
					DevName: "foo",
					DevID:   7,
					To:      ipNetOf(mustParseCIDR("2001:db8:0:3::/64")),
					Via:     net.ParseIP("fe80::1"),
				},
			}},
			wantErrors: []error{nil},
		},
		{
			name: "attempt to perform unsupported UpdateBeam",
			routeList: []routesError{
				EMPTY_ROUTE_TABLE, // AF_INET routes at start of Dispatch
				EMPTY_ROUTE_TABLE, // AF_INET6 routes at start of Dispatch
			},
			getLinkIDByName: map[string]linkIdxError{},
			configChanges: []*schedpb.CreateEntryRequest{
				{
					Id:    "beam_update_1",
					Seqno: 1,
					ConfigurationChange: &schedpb.CreateEntryRequest_UpdateBeam{
						UpdateBeam: &schedpb.UpdateBeam{},
					},
				},
			},
			wantRoutes: [][]*installedRoute{
				{},
			},
			wantErrors: []error{
				&UnsupportedUpdateError{
					req: &schedpb.CreateEntryRequest{
						Id:    "beam_update_1",
						Seqno: 1,
						ConfigurationChange: &schedpb.CreateEntryRequest_UpdateBeam{
							UpdateBeam: &schedpb.UpdateBeam{},
						},
					},
				},
			},
		},
		{
			name: "attempt to perform unsupported DeleteBeam",
			routeList: []routesError{
				EMPTY_ROUTE_TABLE, // AF_INET routes at start of Dispatch
				EMPTY_ROUTE_TABLE, // AF_INET6 routes at start of Dispatch
			},
			getLinkIDByName: map[string]linkIdxError{},
			configChanges: []*schedpb.CreateEntryRequest{
				{
					Id:    "beam_update_1",
					Seqno: 1,
					ConfigurationChange: &schedpb.CreateEntryRequest_DeleteBeam{
						DeleteBeam: &schedpb.DeleteBeam{},
					},
				},
			},
			wantRoutes: [][]*installedRoute{
				{},
			},
			wantErrors: []error{
				&UnsupportedUpdateError{
					req: &schedpb.CreateEntryRequest{
						Id:    "beam_update_1",
						Seqno: 1,
						ConfigurationChange: &schedpb.CreateEntryRequest_DeleteBeam{
							DeleteBeam: &schedpb.DeleteBeam{},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// t.Parallel()

			t.Logf("==== Unit test: %s", tc.name)

			routeListIdx := 0
			routeAddIdx := 0
			routeDelIdx := 0

			config := Config{
				Clock:                 clockwork.NewFakeClock(),
				RtTableID:             AGENT_TABLE_ID,
				RtTableLookupPriority: 25200,
				GetLinkIDByName: func(interface_id string) (int, error) {
					rv, ok := tc.getLinkIDByName[interface_id]
					if !ok {
						t.Fatalf("inteface %q not found", interface_id)
					}
					return rv.idx, rv.err
				},
				RouteListFiltered: func(int, *vnl.Route, uint64) (routes []vnl.Route, err error) {
					rv := tc.routeList[routeListIdx]
					routeListIdx += 1
					return rv.routes, rv.err
				},
				RouteAdd: func(toAdd *vnl.Route) error {
					rv := tc.routeAdd[routeAddIdx]
					if !routesMostlyEqual(rv.route, *toAdd) {
						t.Errorf("RouteAdd(%q) does not match expected %q", toAdd, rv.route)
					}
					routeAddIdx += 1
					return rv.err
				},
				RouteDel: func(toDelete *vnl.Route) error {
					rv := tc.routeDel[routeDelIdx]
					if !routesMostlyEqual(rv.route, *toDelete) {
						t.Errorf("RouteDel(%q) does not match expected %q", toDelete, rv.route)
					}
					routeDelIdx += 1
					return rv.err
				},
				RuleAdd: func(*vnl.Rule) error {
					return nil
				},
			}
			backend := New(config)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			for i, cc := range tc.configChanges {
				err := backend.Dispatch(ctx, cc)
				switch wantErr := tc.wantErrors[i]; {
				case err == nil && wantErr != nil:
					t.Fatalf("update #%d expected error (%v), got nil", i, wantErr)
				case err != nil && wantErr == nil:
					t.Fatalf("update #%d had unexpected error: %v", i, err)
				case err != nil && err.Error() != wantErr.Error():
					t.Fatalf("update #%d error mismatch; wanted %v, got %v", i, wantErr, err)
				}

				backend.mu.Lock()
				gotRoutes := slices.Clone(backend.routes)
				backend.mu.Unlock()

				if diff := cmp.Diff(tc.wantRoutes[i], gotRoutes, protocmp.Transform()); diff != "" {
					t.Errorf("update %d unexpected message (-want +got):\n%s", i, diff)
				}
			}

			if routeListIdx != len(tc.routeList) {
				t.Errorf("expected %d RouteListFiltered calls but got %d", len(tc.routeList), routeListIdx)
			}
			if routeAddIdx != len(tc.routeAdd) {
				t.Errorf("expected %d RouteAdd calls but got %d", len(tc.routeAdd), routeAddIdx)
			}
			if routeDelIdx != len(tc.routeDel) {
				t.Errorf("expected %d RouteDel calls but got %d", len(tc.routeDel), routeDelIdx)
			}
		})
	}
}
