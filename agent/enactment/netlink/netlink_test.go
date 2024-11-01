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
	"maps"
	"net"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	vnl "github.com/vishvananda/netlink"
	"google.golang.org/protobuf/testing/protocmp"

	apipb "aalyria.com/spacetime/api/common"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
)

func TestNetlink(t *testing.T) {
	// t.Parallel()

	type routesError struct {
		routes []vnl.Route
		err    error
	}

	type linkIdxError struct {
		idx int
		err error
	}

	type testCase struct {
		name                  string
		preInstalledFlowRules map[string]*apipb.FlowRule
		routeList             []routesError
		routeAdd              []error
		routeDel              []error
		getLinkIDByName       []linkIdxError
		configChanges         []*schedpb.CreateEntryRequest
		wantIDs               []map[string]*schedpb.SetRoute
		wantErrors            []error
	}

	_, dst, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatalf("failed net.ParseIP(dstString=%s)", "192.168.1.0/24")
	}

	testCases := []testCase{
		{
			name: "add route",
			routeList: []routesError{{
				routes: []vnl.Route{{
					Src:       net.ParseIP("192.168.2.2"),
					Dst:       dst,
					Gw:        net.ParseIP("192.168.1.1"),
					LinkIndex: 7,
				}},
				err: nil,
			}},
			routeAdd:        []error{nil},
			routeDel:        []error{nil},
			getLinkIDByName: []linkIdxError{{idx: 7, err: nil}},
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
			wantIDs: []map[string]*schedpb.SetRoute{
				{
					"rule1": &schedpb.SetRoute{
						From: "192.168.2.2",
						To:   "192.168.1.0/24",
						Via:  "192.168.1.1",
						Dev:  "foo",
					},
				},
			},
			wantErrors: []error{nil},
		},
		{
			name: "add and remove route",
			routeList: []routesError{
				{
					routes: []vnl.Route{{
						Src:       net.ParseIP("192.168.2.2"),
						Dst:       dst,
						Gw:        net.ParseIP("192.168.1.1"),
						LinkIndex: 7,
					}},
					err: nil,
				},
				{
					routes: []vnl.Route{},
					err:    nil,
				},
			},
			routeAdd:        []error{nil},
			routeDel:        []error{nil},
			getLinkIDByName: []linkIdxError{{idx: 7, err: nil}, {idx: 7, err: nil}},
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
			wantIDs: []map[string]*schedpb.SetRoute{
				{
					"rule1": &schedpb.SetRoute{
						From: "192.168.2.2",
						To:   "192.168.1.0/24",
						Dev:  "foo",
						Via:  "192.168.1.1",
					},
				},
				{},
			},
			wantErrors: []error{nil, nil},
		},
		{
			name: "remove existing route",
			routeList: []routesError{
				{
					routes: []vnl.Route{
						{
							Src:       net.ParseIP("192.168.2.2"),
							Dst:       dst,
							Gw:        net.ParseIP("192.168.1.1"),
							LinkIndex: 7,
						},
					},
					err: nil,
				},
				{
					err: nil,
				},
			},
			routeAdd:        []error{nil},
			routeDel:        []error{nil},
			getLinkIDByName: []linkIdxError{{idx: 7, err: nil}, {idx: 7, err: nil}},
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
			wantIDs: []map[string]*schedpb.SetRoute{
				{
					"rule1": &schedpb.SetRoute{
						From: "192.168.2.2",
						To:   "192.168.1.0/24",
						Dev:  "foo",
						Via:  "192.168.1.1",
					},
				},
				{},
			},
			wantErrors: []error{nil, nil},
		},
		{
			name: "remove nonexisting route",
			routeList: []routesError{
				{
					routes: []vnl.Route{
						{
							Src:       net.ParseIP("192.168.2.2"),
							Dst:       dst,
							Gw:        net.ParseIP("192.168.1.1"),
							LinkIndex: 7,
						},
					},
					err: nil,
				},
			},
			routeAdd:        []error{nil},
			routeDel:        []error{nil},
			getLinkIDByName: []linkIdxError{{idx: 7, err: nil}},
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
			wantIDs:    []map[string]*schedpb.SetRoute{{}},
			wantErrors: []error{&UnknownRouteDeleteError{"rule1"}},
		},
		{
			name: "attempt to perform ScheduledControlUpdate with no Change",
			routeList: []routesError{
				{
					routes: []vnl.Route{
						{
							Src:       net.ParseIP("192.168.2.2"),
							Dst:       dst,
							Gw:        net.ParseIP("192.168.1.1"),
							LinkIndex: 7,
						},
					},
					err: nil,
				},
			},
			routeAdd:        []error{nil},
			routeDel:        []error{nil},
			getLinkIDByName: []linkIdxError{{idx: 7, err: nil}},
			configChanges: []*schedpb.CreateEntryRequest{
				{
					Id:                  "rule1",
					Seqno:               1,
					ConfigurationChange: nil,
				},
			},
			wantIDs:    []map[string]*schedpb.SetRoute{{}},
			wantErrors: []error{&NoChangeSpecifiedError{&schedpb.CreateEntryRequest{Id: "rule1", Seqno: 1}}},
		},
		{
			name: "attempt to perform unsupported UpdateBeam",
			routeList: []routesError{
				{
					routes: []vnl.Route{
						{
							Src:       net.ParseIP("192.168.2.2"),
							Dst:       dst,
							Gw:        net.ParseIP("192.168.1.1"),
							LinkIndex: 7,
						},
					},
					err: nil,
				},
			},
			routeAdd:        []error{nil},
			routeDel:        []error{nil},
			getLinkIDByName: []linkIdxError{{idx: 7, err: nil}},
			configChanges: []*schedpb.CreateEntryRequest{
				{
					Id:    "beam_update_1",
					Seqno: 1,
					ConfigurationChange: &schedpb.CreateEntryRequest_UpdateBeam{
						UpdateBeam: &schedpb.UpdateBeam{},
					},
				},
			},
			wantIDs: []map[string]*schedpb.SetRoute{{}},
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
				{
					routes: []vnl.Route{
						{
							Src:       net.ParseIP("192.168.2.2"),
							Dst:       dst,
							Gw:        net.ParseIP("192.168.1.1"),
							LinkIndex: 7,
						},
					},
					err: nil,
				},
			},
			routeAdd:        []error{nil},
			routeDel:        []error{nil},
			getLinkIDByName: []linkIdxError{{idx: 7, err: nil}},
			configChanges: []*schedpb.CreateEntryRequest{
				{
					Id:    "beam_update_1",
					Seqno: 1,
					ConfigurationChange: &schedpb.CreateEntryRequest_DeleteBeam{
						DeleteBeam: &schedpb.DeleteBeam{},
					},
				},
			},
			wantIDs: []map[string]*schedpb.SetRoute{{}},
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

	// Execute table driven test cases
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// t.Parallel()

			t.Logf("==== Unit test: %s", tc.name)

			routeListIdx := 0
			routeAddIdx := 0
			routeDelIdx := 0
			getLinkIdxByNameIdx := 0

			config := Config{
				Clock:                 clockwork.NewFakeClock(),
				RtTableID:             252,
				RtTableLookupPriority: 25200,
				GetLinkIDByName: func(interface_id string) (int, error) {
					rv := tc.getLinkIDByName[getLinkIdxByNameIdx/2]
					getLinkIdxByNameIdx += 1
					return rv.idx, rv.err
				},
				RouteList: func() ([]vnl.Route, error) {
					rv := tc.routeList[routeListIdx]
					routeListIdx += 1
					return rv.routes, rv.err
				},
				RouteListFiltered: func(int, *vnl.Route, uint64) (routes []vnl.Route, err error) {
					rv := tc.routeList[routeListIdx/2] // Because now this function is invoked twice for every operation (due to syncCachedRoutes)
					routeListIdx += 1
					return rv.routes, rv.err
				},
				RouteAdd: func(*vnl.Route) error {
					rv := tc.routeAdd[routeAddIdx]
					routeAddIdx += 1
					return rv
				},
				RouteDel: func(*vnl.Route) error {
					rv := tc.routeDel[routeDelIdx]
					routeDelIdx += 1
					return rv
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
					t.Fatalf("test %q update #%d expected error (%v), got nil", tc.name, i, wantErr)
				case err != nil && err.Error() != wantErr.Error():
					t.Fatalf("test %q update #%d error mismatch; wanted %v, got %v", tc.name, i, wantErr, err)
				}

				gotIDs := map[string]*schedpb.SetRoute{}
				backend.mu.Lock()
				for _, rtr := range backend.routesToRuleIDs {
					maps.Insert(gotIDs, maps.All(rtr.ruleIDs))
				}
				backend.mu.Unlock()

				if diff := cmp.Diff(tc.wantIDs[i], gotIDs, protocmp.Transform()); diff != "" {
					t.Fatalf("test %q update %d unexpected message (-want +got):\n%s", tc.name, i, diff)
				}

			}
		})
	}
}
