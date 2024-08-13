// Copyright 2024 Aalyria Technologies, Inc., and its affiliates.
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
	"testing"
	"time"

	apipb "aalyria.com/spacetime/api/common"
	"github.com/google/go-cmp/cmp"
	vnl "github.com/vishvananda/netlink"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/jonboulle/clockwork"
	"google.golang.org/protobuf/proto"
)

func linkByNameFromMap(links map[string]vnl.Link) func(string) (vnl.Link, error) {
	return func(name string) (vnl.Link, error) {
		if link, ok := links[name]; !ok {
			return nil, fmt.Errorf("link for %s not found", name)
		} else {
			return link, nil
		}
	}
}

func TestNetlink(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()

	type testCase struct {
		name         string
		interfaceIDs []string
		linkByName   func(string) (vnl.Link, error)
		nodeID       string
		wantStats    *apipb.NetworkStatsReport
		wantErr      error
	}

	testCases := []testCase{
		{
			name:         "stats for single link",
			interfaceIDs: []string{"cargo-bay-door"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"cargo-bay-door": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						Statistics: &vnl.LinkStatistics{
							TxPackets: 1,
							RxPackets: 2,
							TxBytes:   3,
							RxBytes:   4,
							TxDropped: 5,
							RxDropped: 6,
							TxErrors:  7,
							RxErrors:  8,
						},
					},
				},
			}),
			nodeID: "serenity",
			wantStats: &apipb.NetworkStatsReport{
				NodeId: proto.String("serenity"),
				Timestamp: &apipb.DateTime{
					UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
				},
				InterfaceStatsById: map[string]*apipb.InterfaceStats{
					"cargo-bay-door": {
						Timestamp: &apipb.DateTime{
							UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
						},
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					},
				},
			},
			wantErr: nil,
		},
		{
			name:         "stats for multiple links",
			interfaceIDs: []string{"gemini", "apollo", "IDSS"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"gemini": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						Statistics: &vnl.LinkStatistics{
							TxPackets: 1,
							RxPackets: 2,
							TxBytes:   3,
							RxBytes:   4,
							TxDropped: 5,
							RxDropped: 6,
							TxErrors:  7,
							RxErrors:  8,
						},
					},
				},
				"apollo": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						Statistics: &vnl.LinkStatistics{
							TxPackets: 10,
							RxPackets: 20,
							TxBytes:   30,
							RxBytes:   40,
							TxDropped: 50,
							RxDropped: 60,
							TxErrors:  70,
							RxErrors:  80,
						},
					},
				},
				"IDSS": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						Statistics: &vnl.LinkStatistics{
							TxPackets: 100,
							RxPackets: 200,
							TxBytes:   300,
							RxBytes:   400,
							TxDropped: 500,
							RxDropped: 600,
							TxErrors:  700,
							RxErrors:  800,
						},
					},
				},
			}),
			nodeID: "ISS",
			wantStats: &apipb.NetworkStatsReport{
				NodeId: proto.String("ISS"),
				Timestamp: &apipb.DateTime{
					UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
				},
				InterfaceStatsById: map[string]*apipb.InterfaceStats{
					"gemini": {
						Timestamp: &apipb.DateTime{
							UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
						},
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					},
					"apollo": {
						Timestamp: &apipb.DateTime{
							UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
						},
						TxPackets: proto.Int64(10),
						RxPackets: proto.Int64(20),
						TxBytes:   proto.Int64(30),
						RxBytes:   proto.Int64(40),
						TxDropped: proto.Int64(50),
						RxDropped: proto.Int64(60),
						TxErrors:  proto.Int64(70),
						RxErrors:  proto.Int64(80),
					},
					"IDSS": {
						Timestamp: &apipb.DateTime{
							UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
						},
						TxPackets: proto.Int64(100),
						RxPackets: proto.Int64(200),
						TxBytes:   proto.Int64(300),
						RxBytes:   proto.Int64(400),
						TxDropped: proto.Int64(500),
						RxDropped: proto.Int64(600),
						TxErrors:  proto.Int64(700),
						RxErrors:  proto.Int64(800),
					},
				},
			},
			wantErr: nil,
		},
		{
			name:         "one link missing",
			interfaceIDs: []string{"dry-dock", "transporter-room"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"dry-dock": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						Statistics: &vnl.LinkStatistics{
							TxPackets: 1,
							RxPackets: 2,
							TxBytes:   3,
							RxBytes:   4,
							TxDropped: 5,
							RxDropped: 6,
							TxErrors:  7,
							RxErrors:  8,
						},
					},
				},
			}),
			nodeID: "enterprise",
			wantStats: &apipb.NetworkStatsReport{
				NodeId: proto.String("enterprise"),
				Timestamp: &apipb.DateTime{
					UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
				},
				InterfaceStatsById: map[string]*apipb.InterfaceStats{
					"dry-dock": {
						Timestamp: &apipb.DateTime{
							UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
						},
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					},
				},
			},
			wantErr: nil,
		},
		{
			name:         "one link missing attrs",
			interfaceIDs: []string{"dry-dock", "transporter-room"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"dry-dock": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						Statistics: &vnl.LinkStatistics{
							TxPackets: 1,
							RxPackets: 2,
							TxBytes:   3,
							RxBytes:   4,
							TxDropped: 5,
							RxDropped: 6,
							TxErrors:  7,
							RxErrors:  8,
						},
					},
				},
				"transporter-room": &vnl.Dummy{},
			}),
			nodeID: "enterprise",
			wantStats: &apipb.NetworkStatsReport{
				NodeId: proto.String("enterprise"),
				Timestamp: &apipb.DateTime{
					UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
				},
				InterfaceStatsById: map[string]*apipb.InterfaceStats{
					"dry-dock": {
						Timestamp: &apipb.DateTime{
							UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
						},
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					},
				},
			},
			wantErr: nil,
		},
		{
			name:         "one link missing stats",
			interfaceIDs: []string{"dry-dock", "transporter-room"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"dry-dock": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						Statistics: &vnl.LinkStatistics{
							TxPackets: 1,
							RxPackets: 2,
							TxBytes:   3,
							RxBytes:   4,
							TxDropped: 5,
							RxDropped: 6,
							TxErrors:  7,
							RxErrors:  8,
						},
					},
				},
				"transporter-room": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{},
				},
			}),
			nodeID: "enterprise",
			wantStats: &apipb.NetworkStatsReport{
				NodeId: proto.String("enterprise"),
				Timestamp: &apipb.DateTime{
					UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
				},
				InterfaceStatsById: map[string]*apipb.InterfaceStats{
					"dry-dock": {
						Timestamp: &apipb.DateTime{
							UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
						},
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					},
				},
			},
			wantErr: nil,
		},
		{
			name:         "all links missing stats",
			interfaceIDs: []string{"dry-dock", "transporter-room"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"dry-dock":         &vnl.Dummy{LinkAttrs: vnl.LinkAttrs{}},
				"transporter-room": &vnl.Dummy{LinkAttrs: vnl.LinkAttrs{}},
			}),
			nodeID:    "enterprise",
			wantStats: nil,
			wantErr:   errNoStats,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			driver := New(clock, tc.interfaceIDs, tc.linkByName)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			gotStats, gotErr := driver.GenerateReport(ctx, tc.nodeID)

			if gotErr != nil && !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf(
					"test %q unexpected error: want(%s), got(%s)\n",
					tc.name,
					tc.wantErr,
					gotErr,
				)
			}

			if diff := cmp.Diff(tc.wantStats, gotStats, protocmp.Transform()); diff != "" {
				t.Fatalf("test %q unexpected stats (-want +got):\n%s", tc.name, diff)
			}
		})
	}
}
