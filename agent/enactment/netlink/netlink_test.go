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
		wantRoutes            [][]*installedRoute
		wantErrors            []error
	}

	_, dst, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatalf("failed net.ParseCIDR(dstString=%s): %v", "192.168.1.0/24", err)
	}
	gw, _, err := net.ParseCIDR("192.168.1.1/32")
	if err != nil {
		t.Fatalf("failed net.ParseCIDR(gwString=%s): %v", "192.168.1.1/32", err)
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
			routeAdd:        []error{nil, nil},
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
			routeAdd:        []error{nil, nil},
			routeDel:        []error{nil, nil},
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
			routeAdd:        []error{nil, nil},
			routeDel:        []error{nil, nil},
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
			routeAdd:        []error{nil, nil},
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
			wantRoutes: [][]*installedRoute{
				{},
			},
			wantErrors: []error{&UnknownRouteDeleteError{changeID: "rule1"}},
		},
		{
			name: "attempt to perform CreateEntryRequest with no Change",
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
			wantRoutes: [][]*installedRoute{
				{},
			},
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
					t.Fatalf("update %d unexpected message (-want +got):\n%s", i, diff)
				}

			}
		})
	}
}
