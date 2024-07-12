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
	"net"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	vnl "github.com/vishvananda/netlink"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	apipb "aalyria.com/spacetime/api/common"
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
		name                    string
		preInstalledFlowRules   map[string]*apipb.FlowRule
		routeList               []routesError
		routeAdd                []error
		routeDel                []error
		getLinkIDByName         []linkIdxError
		scheduledControlUpdates []*apipb.ScheduledControlUpdate
		wantStates              []*apipb.ControlPlaneState
		wantErrors              []error
	}

	_, dst, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatalf("failed net.ParseIP(dstString=%s)", "192.168.1.0/24")
	}

	updateId := "64988bc4-d401-428a-b7be-926bebb5f832"

	testCases := []testCase{
		{
			name: "add route",
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{
									Classifier: &apipb.PacketClassifier{
										IpHeader: &apipb.PacketClassifier_IpHeader{
											SrcIpRange: proto.String("192.168.2.2"),
											DstIpRange: proto.String("192.168.1.0/24"),
										},
									},
									ActionBucket: []*apipb.FlowRule_ActionBucket{
										{
											Action: []*apipb.FlowRule_ActionBucket_Action{
												{
													ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
														Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
															OutInterfaceId: proto.String("foo"),
															NextHopIp:      proto.String("192.168.1.1/32"),
														},
													},
												},
											},
										},
									},
								},
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{
						FlowRuleIds: []string{"rule1"},
					},
				},
			},
			wantErrors: []error{nil},
		},
		{
			name: "add and remove route",
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
					routes: []vnl.Route{},
					err:    nil,
				},
			},
			routeAdd:        []error{nil},
			routeDel:        []error{nil},
			getLinkIDByName: []linkIdxError{{idx: 7, err: nil}, {idx: 7, err: nil}},
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{
									Classifier: &apipb.PacketClassifier{
										IpHeader: &apipb.PacketClassifier_IpHeader{
											SrcIpRange: proto.String("192.168.2.2"),
											DstIpRange: proto.String("192.168.1.0/24"),
										},
									},
									ActionBucket: []*apipb.FlowRule_ActionBucket{
										{
											Action: []*apipb.FlowRule_ActionBucket_Action{
												{
													ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
														Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
															OutInterfaceId: proto.String("foo"),
															NextHopIp:      proto.String("192.168.1.1/32"),
														},
													},
												},
											},
										},
									},
								},
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId:     proto.String("rule1"),
								Operation:      apipb.FlowUpdate_DELETE.Enum(),
								SequenceNumber: proto.Int64(2),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{
						FlowRuleIds: []string{"rule1"},
					},
				},
				{
					ForwardingState: &apipb.FlowState{},
				},
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{
									Classifier: &apipb.PacketClassifier{
										IpHeader: &apipb.PacketClassifier_IpHeader{
											SrcIpRange: proto.String("192.168.2.2"),
											DstIpRange: proto.String("192.168.1.0/24"),
										},
									},
									ActionBucket: []*apipb.FlowRule_ActionBucket{
										{
											Action: []*apipb.FlowRule_ActionBucket_Action{
												{
													ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
														Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
															OutInterfaceId: proto.String("foo"),
															NextHopIp:      proto.String("192.168.1.1/32"),
														},
													},
												},
											},
										},
									},
								},
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId:     proto.String("rule1"),
								Operation:      apipb.FlowUpdate_DELETE.Enum(),
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{FlowRuleIds: []string{"rule1"}},
				},
				{
					ForwardingState: &apipb.FlowState{},
				},
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId:     proto.String("rule1"),
								Operation:      apipb.FlowUpdate_DELETE.Enum(),
								SequenceNumber: proto.Int64(2),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{},
				},
			},
			wantErrors: []error{&UnknownFlowRuleDeleteError{"rule1"}},
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					UpdateId: &updateId,
					Change:   nil, // test break
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{},
				},
			},
			wantErrors: []error{&NoChangeSpecifiedError{}},
		},
		{
			name: "attempt to perform BeamUpdate",
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					UpdateId: &updateId,
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{ // test break
							BeamUpdate: &apipb.BeamUpdate{},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{},
				},
			},
			wantErrors: []error{
				&UnsupportedUpdateError{
					req: &apipb.ScheduledControlUpdate{
						UpdateId: &updateId,
						Change: &apipb.ControlPlaneUpdate{
							UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{ // test break
								BeamUpdate: &apipb.BeamUpdate{},
							},
						},
					},
				},
			},
		},
		{
			name: "attempt unsupported FlowUpdate operation",
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_UNKNOWN.Enum(), // test break
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{},
				},
			},
			wantErrors: []error{fmt.Errorf("FlowUpdate with flowRuleID (%s) failed with error: %w", "rule1", &UnrecognizedFlowUpdateOperationError{apipb.FlowUpdate_UNKNOWN})},
		},
		{
			name: "attempt unsupported ActionType",
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{
									Classifier: &apipb.PacketClassifier{
										IpHeader: &apipb.PacketClassifier_IpHeader{
											SrcIpRange: proto.String("192.168.2.2"),
											DstIpRange: proto.String("192.168.1.0/24"),
										},
									},
									ActionBucket: []*apipb.FlowRule_ActionBucket{
										{
											Action: []*apipb.FlowRule_ActionBucket_Action{
												{
													ActionType: &apipb.FlowRule_ActionBucket_Action_PopHeader_{ // test break
														PopHeader: &apipb.FlowRule_ActionBucket_Action_PopHeader{},
													},
												},
											},
										},
									},
								},
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{},
				},
			},
			wantErrors: []error{fmt.Errorf("FlowUpdate with flowRuleID (%s) failed with error: %w", "rule1", &UnsupportedActionTypeError{})},
		},
		{
			name: "attempt supported ActionType with wrong nextHopIp IPv4 formatting",
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{
									Classifier: &apipb.PacketClassifier{
										IpHeader: &apipb.PacketClassifier_IpHeader{
											SrcIpRange: proto.String("192.168.2.2"),
											DstIpRange: proto.String("192.168.1.0/24"),
										},
									},
									ActionBucket: []*apipb.FlowRule_ActionBucket{
										{
											Action: []*apipb.FlowRule_ActionBucket_Action{
												{
													ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
														Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
															OutInterfaceId: proto.String("foo"),
															NextHopIp:      proto.String("192.168.11."), // test break
														},
													},
												},
											},
										},
									},
								},
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{
						FlowRuleIds: []string{},
					},
				},
			},
			wantErrors: []error{fmt.Errorf("FlowUpdate with flowRuleID (%v) failed with error: %w", "rule1", &net.ParseError{Type: "CIDR address", Text: "192.168.11."})},
		},
		{
			name: "attempt supported ActionType with wrong OutInterfaceId",
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
			getLinkIDByName: []linkIdxError{{idx: 7, err: errors.New("failed GetLinkIDByName(dumb_foo): network is unreachable")}},
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{
									Classifier: &apipb.PacketClassifier{
										IpHeader: &apipb.PacketClassifier_IpHeader{
											SrcIpRange: proto.String("192.168.2.2"),
											DstIpRange: proto.String("192.168.1.0/24"),
										},
									},
									ActionBucket: []*apipb.FlowRule_ActionBucket{
										{
											Action: []*apipb.FlowRule_ActionBucket_Action{
												{
													ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
														Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
															OutInterfaceId: proto.String("dumb_foo"), // test break
															NextHopIp:      proto.String("192.168.1.1/32"),
														},
													},
												},
											},
										},
									},
								},
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{},
				},
			},
			wantErrors: []error{fmt.Errorf("FlowUpdate with flowRuleID (%s) failed with error: %w", "rule1",
				&OutInterfaceIdxError{
					wrongIface:  "dumb_foo",
					sourceError: errors.New("failed GetLinkIDByName(dumb_foo): network is unreachable"),
				})},
		},
		{
			name: "attempt without supplied Classifier.IpHeader",
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{
									Classifier: &apipb.PacketClassifier{
										IpHeader: nil, // test break
									},
									ActionBucket: []*apipb.FlowRule_ActionBucket{
										{
											Action: []*apipb.FlowRule_ActionBucket_Action{
												{
													ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
														Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
															OutInterfaceId: proto.String("foo"),
															NextHopIp:      proto.String("192.168.1.1/32"),
														},
													},
												},
											},
										},
									},
								},
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{},
				},
			},
			wantErrors: []error{
				fmt.Errorf("FlowUpdate with flowRuleID (%s) failed with error: %w", "rule1",
					&ClassifierError{
						missingField: IpHeader_Field,
					}),
			},
		},
		{
			name: "attempt without supplied Classifier.IpHeader.SrcIpRange",
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{
									Classifier: &apipb.PacketClassifier{
										IpHeader: &apipb.PacketClassifier_IpHeader{
											// SrcIpRange: proto.String("192.168.2.2"), // test break
											DstIpRange: proto.String("192.168.1.0/24"),
										},
									},
									ActionBucket: []*apipb.FlowRule_ActionBucket{
										{
											Action: []*apipb.FlowRule_ActionBucket_Action{
												{
													ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
														Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
															OutInterfaceId: proto.String("foo"),
															NextHopIp:      proto.String("192.168.1.1/32"),
														},
													},
												},
											},
										},
									},
								},
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{
						FlowRuleIds: []string{"rule1"},
					},
				},
			},
			wantErrors: []error{nil},
		},
		{
			name: "attempt without supplied Classifier.IpHeader.DstIpRange",
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{
									Classifier: &apipb.PacketClassifier{
										IpHeader: &apipb.PacketClassifier_IpHeader{
											SrcIpRange: proto.String("192.168.2.2"),
											// DstIpRange: proto.String("192.168.1.0/24"), // test break
										},
									},
									ActionBucket: []*apipb.FlowRule_ActionBucket{
										{
											Action: []*apipb.FlowRule_ActionBucket_Action{
												{
													ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
														Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
															OutInterfaceId: proto.String("foo"),
															NextHopIp:      proto.String("192.168.1.1/32"),
														},
													},
												},
											},
										},
									},
								},
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{},
				},
			},
			wantErrors: []error{
				fmt.Errorf("FlowUpdate with flowRuleID (%s) failed with error: %w", "rule1",
					&ClassifierError{
						missingField: DstIpRange_Field,
					}),
			},
		},
		{
			name: "attempt with wrongly formatted Classifier.IpHeader.SrcIpRange",
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{
									Classifier: &apipb.PacketClassifier{
										IpHeader: &apipb.PacketClassifier_IpHeader{
											SrcIpRange: proto.String("192.168.2.222222"),
											DstIpRange: proto.String("192.168.1.0/24"),
										},
									},
									ActionBucket: []*apipb.FlowRule_ActionBucket{
										{
											Action: []*apipb.FlowRule_ActionBucket_Action{
												{
													ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
														Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
															OutInterfaceId: proto.String("foo"),
															NextHopIp:      proto.String("192.168.1.1/32"),
														},
													},
												},
											},
										},
									},
								},
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{
						FlowRuleIds: []string{"rule1"},
					},
				},
			},
			wantErrors: []error{nil},
		},
		{
			name: "attempt with wrongly formatted Classifier.IpHeader.DstIpRange",
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
			scheduledControlUpdates: []*apipb.ScheduledControlUpdate{
				{
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: proto.String("rule1"),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{
									Classifier: &apipb.PacketClassifier{
										IpHeader: &apipb.PacketClassifier_IpHeader{
											SrcIpRange: proto.String("192.168.2.2"),
											DstIpRange: proto.String("192.168.1112.0/24"), // test break
										},
									},
									ActionBucket: []*apipb.FlowRule_ActionBucket{
										{
											Action: []*apipb.FlowRule_ActionBucket_Action{
												{
													ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
														Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
															OutInterfaceId: proto.String("foo"),
															NextHopIp:      proto.String("192.168.1.1/32"),
														},
													},
												},
											},
										},
									},
								},
								SequenceNumber: proto.Int64(1),
							},
						},
					},
				},
			},
			wantStates: []*apipb.ControlPlaneState{
				{
					ForwardingState: &apipb.FlowState{},
				},
			},
			wantErrors: []error{
				fmt.Errorf("FlowUpdate with flowRuleID (%s) failed with error: %w", "rule1",
					&IPv4FormattingError{
						ipv4:        "192.168.1112.0/24",
						sourceField: DstIpRange_Ip,
					}),
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

			for i, cpu := range tc.scheduledControlUpdates {
				got, err := backend.Apply(ctx, cpu)
				switch wantErr := tc.wantErrors[i]; {
				case err == nil && wantErr != nil:
					t.Fatalf("test %q update #%d expected error (%v), got nil", tc.name, i, wantErr)
				case err != nil && err.Error() != wantErr.Error():
					t.Fatalf("test %q update #%d error mismatch; wanted %v, got %v", tc.name, i, wantErr, err)
				}

				if diff := cmp.Diff(tc.wantStates[i], got,
					protocmp.Transform(),
					protocmp.IgnoreFields(&apipb.FlowState{}, "timestamp"),
				); diff != "" {
					t.Fatalf("test %q update %d unexpected message (-want +got):\n%s", tc.name, i, diff)
				}

			}
		})
	}
}
