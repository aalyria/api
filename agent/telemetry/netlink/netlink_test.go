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

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	vnl "github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "aalyria.com/spacetime/api/common"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
)

func textNetworkIfaceID(id *commonpb.NetworkInterfaceId) string {
	text, _ := prototext.Marshal(id)
	return string(text)
}

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
		wantMetrics  *telemetrypb.ExportMetricsRequest
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
						OperState: vnl.OperUp,
					},
				},
			}),
			nodeID: "serenity",
			wantMetrics: &telemetrypb.ExportMetricsRequest{
				InterfaceMetrics: []*telemetrypb.InterfaceMetrics{{
					InterfaceId: proto.String(textNetworkIfaceID(&commonpb.NetworkInterfaceId{
						NodeId:      proto.String("serenity"),
						InterfaceId: proto.String("cargo-bay-door"),
					})),
					OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{{
						Time:  timestamppb.New(clock.Now()),
						Value: telemetrypb.IfOperStatus_IF_OPER_STATUS_UP.Enum(),
					}},
					StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{{
						Time:      timestamppb.New(clock.Now()),
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					}},
				}},
			},
		},
		{
			name:         "stats for multiple links",
			interfaceIDs: []string{"gemini", "apollo", "IDSS"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"gemini": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						OperState: vnl.OperDown,
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
						OperState: vnl.OperLowerLayerDown,
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
						OperState: vnl.OperDormant,
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
			wantMetrics: &telemetrypb.ExportMetricsRequest{
				InterfaceMetrics: []*telemetrypb.InterfaceMetrics{{
					InterfaceId: proto.String(textNetworkIfaceID(&commonpb.NetworkInterfaceId{
						NodeId:      proto.String("ISS"),
						InterfaceId: proto.String("gemini"),
					})),
					OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{{
						Time:  timestamppb.New(clock.Now()),
						Value: telemetrypb.IfOperStatus_IF_OPER_STATUS_DOWN.Enum(),
					}},
					StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{{
						Time:      timestamppb.New(clock.Now()),
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					}},
				}, {
					InterfaceId: proto.String(textNetworkIfaceID(&commonpb.NetworkInterfaceId{
						NodeId:      proto.String("ISS"),
						InterfaceId: proto.String("apollo"),
					})),
					OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{{
						Time:  timestamppb.New(clock.Now()),
						Value: telemetrypb.IfOperStatus_IF_OPER_STATUS_LOWER_LAYER_DOWN.Enum(),
					}},
					StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{
						{
							Time:      timestamppb.New(clock.Now()),
							TxPackets: proto.Int64(10),
							RxPackets: proto.Int64(20),
							TxBytes:   proto.Int64(30),
							RxBytes:   proto.Int64(40),
							TxDropped: proto.Int64(50),
							RxDropped: proto.Int64(60),
							TxErrors:  proto.Int64(70),
							RxErrors:  proto.Int64(80),
						},
					},
				}, {
					InterfaceId: proto.String(textNetworkIfaceID(&commonpb.NetworkInterfaceId{
						NodeId:      proto.String("ISS"),
						InterfaceId: proto.String("IDSS"),
					})),
					OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{{
						Time:  timestamppb.New(clock.Now()),
						Value: telemetrypb.IfOperStatus_IF_OPER_STATUS_DORMANT.Enum(),
					}},
					StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{
						{
							Time:      timestamppb.New(clock.Now()),
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
				}},
			},
		},
		{
			name:         "one link missing",
			interfaceIDs: []string{"dry-dock", "transporter-room"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"dry-dock": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						OperState: vnl.OperTesting,
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
			wantMetrics: &telemetrypb.ExportMetricsRequest{
				InterfaceMetrics: []*telemetrypb.InterfaceMetrics{{
					InterfaceId: proto.String(textNetworkIfaceID(&commonpb.NetworkInterfaceId{
						NodeId:      proto.String("enterprise"),
						InterfaceId: proto.String("dry-dock"),
					})),
					OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{{
						Time:  timestamppb.New(clock.Now()),
						Value: telemetrypb.IfOperStatus_IF_OPER_STATUS_TESTING.Enum(),
					}},
					StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{{
						Time:      timestamppb.New(clock.Now()),
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					}},
				}},
			},
		},
		{
			name:         "one link missing attrs with IFF_RUNNING set",
			interfaceIDs: []string{"dry-dock", "transporter-room"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"dry-dock": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						OperState: vnl.OperUnknown,
						RawFlags:  unix.IFF_RUNNING,
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
			wantMetrics: &telemetrypb.ExportMetricsRequest{
				InterfaceMetrics: []*telemetrypb.InterfaceMetrics{{
					InterfaceId: proto.String(textNetworkIfaceID(&commonpb.NetworkInterfaceId{
						NodeId:      proto.String("enterprise"),
						InterfaceId: proto.String("dry-dock"),
					})),
					OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{{
						Time:  timestamppb.New(clock.Now()),
						Value: telemetrypb.IfOperStatus_IF_OPER_STATUS_UP.Enum(),
					}},
					StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{{
						Time:      timestamppb.New(clock.Now()),
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					}},
				}},
			},
		},
		{
			name:         "one link missing attrs with IFF_DORMANT set",
			interfaceIDs: []string{"dry-dock", "transporter-room"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"dry-dock": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						OperState: vnl.OperUnknown,
						RawFlags:  unix.IFF_DORMANT,
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
			wantMetrics: &telemetrypb.ExportMetricsRequest{
				InterfaceMetrics: []*telemetrypb.InterfaceMetrics{{
					InterfaceId: proto.String(textNetworkIfaceID(&commonpb.NetworkInterfaceId{
						NodeId:      proto.String("enterprise"),
						InterfaceId: proto.String("dry-dock"),
					})),
					OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{{
						Time:  timestamppb.New(clock.Now()),
						Value: telemetrypb.IfOperStatus_IF_OPER_STATUS_DORMANT.Enum(),
					}},
					StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{{
						Time:      timestamppb.New(clock.Now()),
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					}},
				}},
			},
		},
		{
			name:         "one link missing attrs with IFF_RUNNING *not* set",
			interfaceIDs: []string{"dry-dock", "transporter-room"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"dry-dock": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						OperState: vnl.OperUnknown,
						Flags:     0,
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
			wantMetrics: &telemetrypb.ExportMetricsRequest{
				InterfaceMetrics: []*telemetrypb.InterfaceMetrics{{
					InterfaceId: proto.String(textNetworkIfaceID(&commonpb.NetworkInterfaceId{
						NodeId:      proto.String("enterprise"),
						InterfaceId: proto.String("dry-dock"),
					})),
					OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{{
						Time:  timestamppb.New(clock.Now()),
						Value: telemetrypb.IfOperStatus_IF_OPER_STATUS_DOWN.Enum(),
					}},
					StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{{
						Time:      timestamppb.New(clock.Now()),
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					}},
				}},
			},
		},
		{
			name:         "one link missing stats",
			interfaceIDs: []string{"dry-dock", "transporter-room"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"dry-dock": &vnl.Dummy{
					LinkAttrs: vnl.LinkAttrs{
						OperState: vnl.OperNotPresent,
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
			wantMetrics: &telemetrypb.ExportMetricsRequest{
				InterfaceMetrics: []*telemetrypb.InterfaceMetrics{{
					InterfaceId: proto.String(textNetworkIfaceID(&commonpb.NetworkInterfaceId{
						NodeId:      proto.String("enterprise"),
						InterfaceId: proto.String("dry-dock"),
					})),
					OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{{
						Time:  timestamppb.New(clock.Now()),
						Value: telemetrypb.IfOperStatus_IF_OPER_STATUS_NOT_PRESENT.Enum(),
					}},
					StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{{
						Time:      timestamppb.New(clock.Now()),
						TxPackets: proto.Int64(1),
						RxPackets: proto.Int64(2),
						TxBytes:   proto.Int64(3),
						RxBytes:   proto.Int64(4),
						TxDropped: proto.Int64(5),
						RxDropped: proto.Int64(6),
						TxErrors:  proto.Int64(7),
						RxErrors:  proto.Int64(8),
					}},
				}},
			},
		},
		{
			name:         "all links missing stats",
			interfaceIDs: []string{"dry-dock", "transporter-room"},
			linkByName: linkByNameFromMap(map[string]vnl.Link{
				"dry-dock":         &vnl.Dummy{LinkAttrs: vnl.LinkAttrs{}},
				"transporter-room": &vnl.Dummy{LinkAttrs: vnl.LinkAttrs{}},
			}),
			nodeID:  "enterprise",
			wantErr: errNoStats,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			generator := reportGenerator{
				clock:        clock,
				interfaceIDs: tc.interfaceIDs,
				linkByName:   tc.linkByName,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			gotStats, gotErr := generator.GenerateReport(ctx, tc.nodeID)

			if gotErr != nil && !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf(
					"test %q unexpected error: want(%s), got(%s)\n",
					tc.name,
					tc.wantErr,
					gotErr,
				)
			}

			if diff := cmp.Diff(tc.wantMetrics, gotStats, protocmp.Transform()); diff != "" {
				t.Fatalf("test %q unexpected stats (-want +got):\n%s", tc.name, diff)
			}
		})
	}
}
