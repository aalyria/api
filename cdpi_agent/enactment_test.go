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

package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	afpb "aalyria.com/spacetime/api/cdpi/v1alpha"
	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/enactment"
	"aalyria.com/spacetime/cdpi_agent/internal/channels"
	"aalyria.com/spacetime/cdpi_agent/internal/task"

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	startTime = time.Now()

	testCases = []testCase{
		{
			desc: "checking channel priorities are included in initial hello",
			nodes: []testNode{
				{id: "highpri", priority: 1},
				{id: "lowpri", priority: 0},
			},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHello(ctx, "highpri", 1)
				f.expectHello(ctx, "lowpri", 0)
				f.expectNodeStateUpdate(ctx, "highpri", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("highpri"),
					State:  &apipb.ControlPlaneState{},
				})
				f.expectNodeStateUpdate(ctx, "lowpri", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("lowpri"),
					State:  &apipb.ControlPlaneState{},
				})
			},
		},
		{
			desc:  "checking ping/pong works for multiple nodes",
			nodes: []testNode{{id: "one"}, {id: "two"}, {id: "three"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "one")
				f.expectHelloAndEmptyInitialState(ctx, "two")
				f.expectHelloAndEmptyInitialState(ctx, "three")

				f.sendPing(ctx, "one", 1, 20)
				f.sendPing(ctx, "two", 1, 30)
				f.sendPing(ctx, "three", 1, 40)
				f.expectPong(ctx, "one", 1, 20)
				f.expectPong(ctx, "two", 1, 30)
				f.expectPong(ctx, "three", 1, 40)

				f.sendPing(ctx, "one", 2, 21)
				f.sendPing(ctx, "two", 2, 31)
				f.sendPing(ctx, "three", 2, 41)
				f.expectPong(ctx, "one", 2, 21)
				f.expectPong(ctx, "two", 2, 31)
				f.expectPong(ctx, "three", 2, 41)
			},
		},
		{
			desc:  "checking a simple change request",
			nodes: []testNode{{id: "node-a"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "node-a")
				f.prepareEnactmentSuccessForUpdateID(ctx, "20", &apipb.ControlPlaneState{})
				f.sendScheduledUpdate(ctx, "node-a", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("node-a"),
					UpdateId:    proto.String("20"),
					TimeToEnact: timestamppb.New(startTime),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								InterfaceId: &apipb.NetworkInterfaceId{
									InterfaceId: proto.String("lo1"),
								},
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "node-a", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("20"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectNodeStateUpdate(ctx, "node-a", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("node-a"),
					State:  &apipb.ControlPlaneState{},
				})
				f.expectRequestStatusUpdate(ctx, "node-a", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("node-a"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("20"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: OK().Proto(),
						},
					},
				})
			},
		},
		{
			desc:  "checking a change request that failed with an unknown error",
			nodes: []testNode{{id: "node-a"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "node-a")
				f.prepareEnactmentFailureForUpdateID(ctx, "20", errors.New("something went wrong"))
				f.sendScheduledUpdate(ctx, "node-a", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("node-a"),
					UpdateId:    proto.String("20"),
					TimeToEnact: timestamppb.New(startTime),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								InterfaceId: &apipb.NetworkInterfaceId{
									InterfaceId: proto.String("lo1"),
								},
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "node-a", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("20"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectRequestStatusUpdate(ctx, "node-a", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("node-a"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("20"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: status.New(codes.Unknown, "something went wrong").Proto(),
						},
					},
				})
			},
		},
		{
			desc:  "immediate and future scheduled updates",
			nodes: []testNode{{id: "node-a"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "node-a")
				f.prepareEnactmentSuccessForUpdateID(ctx, "20", &apipb.ControlPlaneState{
					RadioStates: &apipb.RadioStates{
						Timestamp: &apipb.DateTime{
							UnixTimeUsec: proto.Int64(startTime.Add(5 * time.Minute).UnixMicro()),
						},
						RadioConfigIdByInterfaceId: map[string]string{
							"lo0": "some ID",
						},
					},
				})
				f.prepareEnactmentSuccessForUpdateID(ctx, "21", &apipb.ControlPlaneState{
					BeamStates: &apipb.BeamStates{
						Timestamp: &apipb.DateTime{
							UnixTimeUsec: proto.Int64(startTime.UnixMicro()),
						},
						BeamTaskIds: []string{"bt1", "bt2"},
					},
				})
				// future update
				f.sendScheduledUpdate(ctx, "node-a", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("node-a"),
					UpdateId:    proto.String("20"),
					TimeToEnact: timestamppb.New(startTime.Add(5 * time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								InterfaceId: &apipb.NetworkInterfaceId{
									InterfaceId: proto.String("lo1"),
								},
							},
						},
					},
				})
				// now update
				f.sendScheduledUpdate(ctx, "node-a", 2, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("node-a"),
					UpdateId:    proto.String("21"),
					TimeToEnact: timestamppb.New(startTime),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								InterfaceId: &apipb.NetworkInterfaceId{
									InterfaceId: proto.String("lo1"),
								},
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "node-a", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("20"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectStreamResponse(ctx, "node-a", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(2),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("21"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectRequestStatusUpdate(ctx, "node-a", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("node-a"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("21"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: OK().Proto(),
						},
					},
				})
				f.expectNodeStateUpdate(ctx, "node-a", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("node-a"),
					State: &apipb.ControlPlaneState{
						BeamStates: &apipb.BeamStates{
							Timestamp: &apipb.DateTime{
								UnixTimeUsec: proto.Int64(startTime.UnixMicro()),
							},
							BeamTaskIds: []string{"bt1", "bt2"},
						},
					},
				})
				f.advanceClock(ctx, 5*time.Minute)
				f.expectRequestStatusUpdate(ctx, "node-a", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("node-a"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("20"),
						Timestamp: timeToProto(startTime.Add(5 * time.Minute)),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: OK().Proto(),
						},
					},
				})
				f.expectNodeStateUpdate(ctx, "node-a", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("node-a"),
					State: &apipb.ControlPlaneState{
						RadioStates: &apipb.RadioStates{
							Timestamp: &apipb.DateTime{
								UnixTimeUsec: proto.Int64(startTime.Add(5 * time.Minute).UnixMicro()),
							},
							RadioConfigIdByInterfaceId: map[string]string{
								"lo0": "some ID",
							},
						},
					},
				})
			},
		},
		{
			desc:  "deleting a future scheduled update",
			nodes: []testNode{{id: "node-a"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "node-a")
				f.prepareEnactmentSuccessForUpdateID(ctx, "20", &apipb.ControlPlaneState{})
				// future update
				f.sendScheduledUpdate(ctx, "node-a", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("node-a"),
					UpdateId:    proto.String("20"),
					TimeToEnact: timestamppb.New(startTime.Add(5 * time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								InterfaceId: &apipb.NetworkInterfaceId{
									InterfaceId: proto.String("lo1"),
								},
							},
						},
					},
				})
				f.sendScheduledDeletion(ctx, "node-a", 2, &apipb.ScheduledControlDeletion{
					NodeId:    proto.String("node-a"),
					UpdateIds: []string{"20"},
				})
				f.expectStreamResponse(ctx, "node-a", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						Timestamp: timeToProto(startTime),
						UpdateId:  proto.String("20"),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectStreamResponse(ctx, "node-a", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(2),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("20"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Unscheduled{Unscheduled: OK().Proto()},
					}},
				})
			},
		},
		{
			desc:  "deleting an in-progress scheduled update",
			nodes: []testNode{{id: "node-a"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "node-a")
				f.prepareEnactmentSuccessForUpdateID(ctx, "20", &apipb.ControlPlaneState{})
				// future update
				f.sendScheduledUpdate(ctx, "node-a", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("node-a"),
					UpdateId:    proto.String("20"),
					TimeToEnact: timestamppb.New(startTime.Add(5 * time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								InterfaceId: &apipb.NetworkInterfaceId{
									InterfaceId: proto.String("lo1"),
								},
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "node-a", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						Timestamp: timeToProto(startTime),
						UpdateId:  proto.String("20"),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.advanceClock(ctx, 5*time.Minute)
				f.expectRequestStatusUpdate(ctx, "node-a", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("node-a"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("20"),
						Timestamp: timeToProto(f.clock.Now()),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: OK().Proto(),
						},
					},
				})
				f.expectNodeStateUpdate(ctx, "node-a", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("node-a"),
					State:  &apipb.ControlPlaneState{},
				})
				f.sendScheduledDeletion(ctx, "node-a", 2, &apipb.ScheduledControlDeletion{
					NodeId:    proto.String("node-a"),
					UpdateIds: []string{"20"},
				})
				f.expectStreamResponse(ctx, "node-a", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(2),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("20"),
						Timestamp: timeToProto(f.clock.Now()),
						State: &apipb.ScheduledControlUpdateStatus_Unscheduled{
							Unscheduled: status.New(codes.DeadlineExceeded, "scheduled update already attempted").Proto(),
						},
					}},
				})
			},
		},
		{
			desc:  "GRPCStatus errors from the enactment backend should get sent back to the server",
			nodes: []testNode{{id: "node-a"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "node-a")
				f.prepareEnactmentFailureForUpdateID(ctx, "42", status.New(codes.Unauthenticated, "some auth issue").Err())
				f.sendScheduledUpdate(ctx, "node-a", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("node-a"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								InterfaceId: &apipb.NetworkInterfaceId{
									InterfaceId: proto.String("lo1"),
								},
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "node-a", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectRequestStatusUpdate(ctx, "node-a", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("node-a"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(f.clock.Now()),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: status.New(codes.Unauthenticated, "some auth issue").Proto(),
						},
					},
				})
			},
		},
		{
			desc:  "stale change requests (ones that are too old) for any radio updates SHOULD be rejected",
			nodes: []testNode{{id: "radio-node"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "radio-node")
				f.sendScheduledUpdate(ctx, "radio-node", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("radio-node"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(-time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_RadioUpdate{
							RadioUpdate: &apipb.RadioUpdate{
								InterfaceId: proto.String("ro0"),
								TxState:     &apipb.TransmitterState{CenterFrequencyHz: proto.Uint64(1337)},
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "radio-node", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: status.New(codes.DeadlineExceeded, "Update received by node but is too old to enact").Proto(),
						},
					}},
				})
			},
		},
		{
			desc:  "stale change requests (ones that are too old) for beam updates with operation ADD SHOULD be rejected",
			nodes: []testNode{{id: "radio-node"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "radio-node")
				f.sendScheduledUpdate(ctx, "radio-node", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("radio-node"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(-time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								Operation: apipb.BeamUpdate_ADD.Enum(),
								InterfaceId: &apipb.NetworkInterfaceId{
									NodeId:      proto.String("radio-node"),
									InterfaceId: proto.String("ro1"),
								},
								RadioConfig: &apipb.RadioConfig{TxChannel: &apipb.RadioConfig_Channel{CenterFrequencyHz: proto.Uint64(1337)}},
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "radio-node", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: status.New(codes.DeadlineExceeded, "Update received by node but is too old to enact").Proto(),
						},
					}},
				})
			},
		},
		{
			desc:  "stale change requests (ones that are too old) for beam updates with operation DELETE SHOULD NOT be rejected",
			nodes: []testNode{{id: "beam-node"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "beam-node")
				f.prepareEnactmentSuccessForUpdateID(ctx, "42", &apipb.ControlPlaneState{})
				f.sendScheduledUpdate(ctx, "beam-node", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("beam-node"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(-time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								Operation: apipb.BeamUpdate_DELETE.Enum(),
								InterfaceId: &apipb.NetworkInterfaceId{
									NodeId:      proto.String("radio-node"),
									InterfaceId: proto.String("ro1"),
								},
								RadioConfig: &apipb.RadioConfig{
									TxChannel: &apipb.RadioConfig_Channel{
										CenterFrequencyHz: proto.Uint64(1337),
									},
								},
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "beam-node", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectNodeStateUpdate(ctx, "beam-node", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("beam-node"),
					State:  &apipb.ControlPlaneState{},
				})
				f.expectRequestStatusUpdate(ctx, "beam-node", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("beam-node"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: OK().Proto(),
						},
					},
				})
			},
		},
		{
			desc:  "stale change requests (ones that are too old) for flow updates with operation ADD SHOULD be rejected",
			nodes: []testNode{{id: "radio-node"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "radio-node")
				f.prepareEnactmentSuccessForUpdateID(ctx, "42", &apipb.ControlPlaneState{})
				f.sendScheduledUpdate(ctx, "radio-node", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("radio-node"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(-time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								FlowRuleId: new(string),
								Operation:  apipb.FlowUpdate_ADD.Enum(),
								Rule: &apipb.FlowRule{Classifier: &apipb.PacketClassifier{}, ActionBucket: []*apipb.FlowRule_ActionBucket{{
									Action: []*apipb.FlowRule_ActionBucket_Action{
										{
											ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
												Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
													OutInterfaceId: proto.String("flo0"),
												},
											},
										},
									},
								}}},
								SequenceNumber: new(int64),
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "radio-node", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: status.New(codes.DeadlineExceeded, "Update received by node but is too old to enact").Proto(),
						},
					}},
				})
			},
		},
		{
			desc:  "stale change requests (ones that are too old) for flow updates with operation DELETE SHOULD NOT be rejected",
			nodes: []testNode{{id: "flow-node"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "flow-node")
				f.prepareEnactmentSuccessForUpdateID(ctx, "42", &apipb.ControlPlaneState{})
				f.sendScheduledUpdate(ctx, "flow-node", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("flow-node"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(-time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								Operation: apipb.FlowUpdate_DELETE.Enum(),
								Rule:      &apipb.FlowRule{},
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "flow-node", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectRequestStatusUpdate(ctx, "flow-node", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("flow-node"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: OK().Proto(),
						},
					},
				})
				f.expectNodeStateUpdate(ctx, "flow-node", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("flow-node"),
					State:  &apipb.ControlPlaneState{},
				})
			},
		},
		{
			desc:  "stale change requests (ones that are too old) for tunnel updates with operation ADD SHOULD be rejected",
			nodes: []testNode{{id: "tunnel-node"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "tunnel-node")
				f.prepareEnactmentSuccessForUpdateID(ctx, "42", &apipb.ControlPlaneState{})
				f.sendScheduledUpdate(ctx, "tunnel-node", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("tunnel-node"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(-time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_TunnelUpdate{
							TunnelUpdate: &apipb.TunnelUpdate{
								Operation: apipb.TunnelUpdate_ADD.Enum(),
								Rule: &apipb.TunnelRule{
									EncapRule: &apipb.TunnelRule_EncapRule{
										Classifier: &apipb.PacketClassifier{},
									},
									DecapRule: &apipb.TunnelRule_DecapRule{},
								},
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "tunnel-node", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: status.New(codes.DeadlineExceeded, "Update received by node but is too old to enact").Proto(),
						},
					}},
				})
			},
		},
		{
			desc:  "stale change requests (ones that are too old) for tunnel updates with operation DELETE SHOULD NOT be rejected",
			nodes: []testNode{{id: "tunnel-node"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "tunnel-node")
				f.prepareEnactmentSuccessForUpdateID(ctx, "42", &apipb.ControlPlaneState{})
				f.sendScheduledUpdate(ctx, "tunnel-node", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("tunnel-node"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(-time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_TunnelUpdate{
							TunnelUpdate: &apipb.TunnelUpdate{
								Operation: apipb.TunnelUpdate_DELETE.Enum(),
								Rule:      &apipb.TunnelRule{},
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "tunnel-node", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectNodeStateUpdate(ctx, "tunnel-node", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("tunnel-node"),
					State:  &apipb.ControlPlaneState{},
				})
				f.expectRequestStatusUpdate(ctx, "tunnel-node", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("tunnel-node"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: OK().Proto(),
						},
					},
				})
			},
		},
		{
			desc:  "stale change requests (ones that are too old) for unknown update types SHOULD be rejected",
			nodes: []testNode{{id: "unknown-node"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "unknown-node")
				f.prepareEnactmentSuccessForUpdateID(ctx, "42", &apipb.ControlPlaneState{})
				f.sendScheduledUpdate(ctx, "unknown-node", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("unknown-node"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(-time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: nil,
					},
				})
				f.expectStreamResponse(ctx, "unknown-node", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: status.New(codes.DeadlineExceeded, "Update received by node but is too old to enact").Proto(),
						},
					}},
				})
			},
		},
		{
			desc:  "change requests for pending changes with lower sequence numbers SHOULD be rejected",
			nodes: []testNode{{id: "n1"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "n1")
				f.sendScheduledUpdate(ctx, "n1", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("n1"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								BeamTaskId:                 proto.String("b1"),
								PerInterfaceSequenceNumber: proto.Int64(1),
							},
						},
					},
				})
				f.sendScheduledUpdate(ctx, "n1", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("n1"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								BeamTaskId:                 proto.String("b1"),
								PerInterfaceSequenceNumber: proto.Int64(0),
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "n1", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectStreamResponse(ctx, "n1", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_Scheduled{
							Scheduled: status.New(codes.InvalidArgument, "more recent update already exists").Proto(),
						},
					}},
				})
			},
		},
		{
			desc:  "change requests for pending changes with higher sequence numbers SHOULD be accepted",
			nodes: []testNode{{id: "n1"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "n1")
				f.sendScheduledUpdate(ctx, "n1", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("n1"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								BeamTaskId:                 proto.String("b1"),
								PerInterfaceSequenceNumber: proto.Int64(1),
							},
						},
					},
				})
				f.sendScheduledUpdate(ctx, "n1", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("n1"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								BeamTaskId:                 proto.String("b1"),
								PerInterfaceSequenceNumber: proto.Int64(2),
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "n1", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectStreamResponse(ctx, "n1", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
			},
		},
		{
			desc:  "change requests for already enacted changes with higher sequence numbers SHOULD cause an error",
			nodes: []testNode{{id: "n1"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "n1")
				f.prepareEnactmentSuccessForUpdateID(ctx, "42", &apipb.ControlPlaneState{})
				// send an "enact now" update
				f.sendScheduledUpdate(ctx, "n1", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("n1"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								BeamTaskId:                 proto.String("b1"),
								PerInterfaceSequenceNumber: proto.Int64(1),
							},
						},
					},
				})
				// attempt to update that with an "enact in a minute" message
				f.sendScheduledUpdate(ctx, "n1", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("n1"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime.Add(time.Minute)),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								BeamTaskId:                 proto.String("b1"),
								PerInterfaceSequenceNumber: proto.Int64(2),
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "n1", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})
				f.expectStreamResponse(ctx, "n1", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: status.New(codes.FailedPrecondition, "update already enacted").Proto(),
						},
					}},
				})
				f.expectRequestStatusUpdate(ctx, "n1", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("n1"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: OK().Proto(),
						},
					},
				})
				f.expectNodeStateUpdate(ctx, "n1", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("n1"),
					State:  &apipb.ControlPlaneState{},
				})
			},
		},
		{
			desc:  "after a minute, already enacted change requests should be removed from memory",
			nodes: []testNode{{id: "n1"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectHelloAndEmptyInitialState(ctx, "n1")
				f.prepareEnactmentSuccessForUpdateID(ctx, "42", &apipb.ControlPlaneState{})
				// send an "enact now" update
				f.sendScheduledUpdate(ctx, "n1", 1, &apipb.ScheduledControlUpdate{
					NodeId:      proto.String("n1"),
					UpdateId:    proto.String("42"),
					TimeToEnact: timestamppb.New(startTime),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{
								BeamTaskId:                 proto.String("b1"),
								PerInterfaceSequenceNumber: proto.Int64(1),
							},
						},
					},
				})
				f.expectStreamResponse(ctx, "n1", &afpb.CdpiRequest_Response{
					RequestId: proto.Int64(1),
					Status:    OK().Proto(),
				}, &afpb.ControlStateNotification{
					Statuses: []*apipb.ScheduledControlUpdateStatus{{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
					}},
				})

				f.expectNodeStateUpdate(ctx, "n1", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("n1"),
					State:  &apipb.ControlPlaneState{},
				})
				f.expectRequestStatusUpdate(ctx, "n1", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("n1"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(startTime),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: OK().Proto(),
						},
					},
				})

				f.clock.Advance(1 * time.Minute)
				now := f.clock.Now()

				// Even though we've advanced the clock, we don't know for sure
				// that the cleanup function that removes the stale scheduled
				// update has finished doing its job. Since the stale entry +
				// cleanup behavior isn't a hard requirement of the protocol,
				// we check a couple of times to see that it's eventually
				// removed which is good enough.
				attempt := func() error {
					f.sendScheduledUpdate(ctx, "n1", 4, &apipb.ScheduledControlUpdate{
						NodeId:      proto.String("n1"),
						UpdateId:    proto.String("42"),
						TimeToEnact: timestamppb.New(now),
						Change: &apipb.ControlPlaneUpdate{
							UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
								BeamUpdate: &apipb.BeamUpdate{
									BeamTaskId:                 proto.String("b1"),
									PerInterfaceSequenceNumber: proto.Int64(1),
								},
							},
						},
					})

					return f.maybeExpectStreamResponse(ctx, "n1", &afpb.CdpiRequest_Response{
						RequestId: proto.Int64(4),
						Status:    OK().Proto(),
					}, &afpb.ControlStateNotification{
						Statuses: []*apipb.ScheduledControlUpdateStatus{{
							UpdateId:  proto.String("42"),
							Timestamp: timeToProto(now),
							State:     &apipb.ScheduledControlUpdateStatus_Scheduled{},
						}},
					})
				}

				var (
					err         error
					numAttempts = 1
				)
				for ; numAttempts <= 5; numAttempts++ {
					if err = attempt(); err == nil {
						break
					}
					time.Sleep(1 * time.Millisecond)
				}
				if err != nil {
					f.t.Errorf("failed after attempt #%d: %v", numAttempts, err)
					return
				}

				f.expectNodeStateUpdate(ctx, "n1", &afpb.CdpiNodeStateRequest{
					NodeId: proto.String("n1"),
					State:  &apipb.ControlPlaneState{},
				})
				f.expectRequestStatusUpdate(ctx, "n1", &afpb.CdpiRequestStatusRequest{
					NodeId: proto.String("n1"),
					Status: &apipb.ScheduledControlUpdateStatus{
						UpdateId:  proto.String("42"),
						Timestamp: timeToProto(now),
						State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
							EnactmentAttempted: OK().Proto(),
						},
					},
				})
			},
		},
	}
)

func TestEnactments(t *testing.T) {
	t.Parallel()

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			tc.runTest(t)
		})
	}
}

func (tc *testCase) runTest(t *testing.T) {
	ctx, cancel := context.WithCancel(baseContext(t))
	g, ctx := errgroup.WithContext(ctx)
	defer func() {
		cancel()
		checkErrIsDueToCanceledContext(t, g.Wait())
	}()

	enact := newDelegatingBackend()
	defer enact.checkNoUnhandledUpdates(t)

	clock := clockwork.NewFakeClockAt(startTime)
	srv := newServer(
		zerolog.Ctx(ctx).With().Str("role", "test server").Logger().WithContext(ctx), tc.nodes)
	srvAddr := srv.start(ctx, t, g)
	defer srv.checkNoUnreadUnaryUpdates(t)

	opts := []AgentOption{
		WithClock(clock),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithServerEndpoint(srvAddr),
	}
	for _, n := range tc.nodes {
		opts = append(opts, WithNode(n.id,
			WithInitialState(&apipb.ControlPlaneState{}),
			WithChannelPriority(n.priority),
			WithEnactmentBackend(enact.toBackend())))
	}
	agent := newAgent(t, opts...)

	g.Go(func() error { return agent.Run(ctx) })
	tc.test(ctx, &testFixture{t: t, srv: srv, eb: enact, clock: clock})
}

type delegatingBackend struct {
	m    map[string]enactment.Backend
	errs []error
}

func newDelegatingBackend() *delegatingBackend {
	return &delegatingBackend{
		m:    map[string]enactment.Backend{},
		errs: []error{},
	}
}

func (d *delegatingBackend) setBackendForUpdateID(updateID string, b enactment.Backend) {
	d.m[updateID] = b
}

func (d *delegatingBackend) toBackend() enactment.Backend {
	return func(ctx context.Context, scu *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
		uid := scu.GetUpdateId()
		if b, ok := d.m[uid]; ok {
			return b(ctx, scu)
		}

		err := fmt.Errorf("no delegate for update with ID %s", uid)
		d.errs = append(d.errs, err)
		return nil, err
	}
}

func (d *delegatingBackend) checkNoUnhandledUpdates(t *testing.T) {
	if err := errors.Join(d.errs...); err != nil {
		t.Errorf("delegatingBackend encountered error(s): %s", err)
	}
}

type server struct {
	afpb.UnimplementedCdpiServer
	ctx            context.Context
	mu             *sync.Mutex
	updateCond     *sync.Cond
	streams        map[string]*serverStream
	stateUpdates   map[string][]*afpb.CdpiNodeStateRequest
	requestUpdates map[string][]*afpb.CdpiRequestStatusRequest
}

type serverStream struct {
	stream *afpb.Cdpi_CdpiServer
	reqCh  chan *afpb.CdpiRequest
	respCh chan *afpb.CdpiResponse
}

func newServer(ctx context.Context, nodes []testNode) *server {
	streams := map[string]*serverStream{}
	for _, n := range nodes {
		streams[n.id] = &serverStream{
			reqCh:  make(chan *afpb.CdpiRequest),
			respCh: make(chan *afpb.CdpiResponse),
		}
	}

	mu := &sync.Mutex{}
	return &server{
		mu:             mu,
		updateCond:     sync.NewCond(mu),
		ctx:            ctx,
		streams:        streams,
		stateUpdates:   map[string][]*afpb.CdpiNodeStateRequest{},
		requestUpdates: map[string][]*afpb.CdpiRequestStatusRequest{},
	}
}

func (s *server) Cdpi(stream afpb.Cdpi_CdpiServer) error {
	hello, err := stream.Recv()
	if err != nil {
		return err
	}

	nid := hello.GetHello().GetNodeId()

	s.mu.Lock()
	ss, ok := s.streams[nid]
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("unknown node connecting: %q", nid)
	}
	defer close(ss.respCh)

	select {
	case <-s.ctx.Done():
		return context.Canceled
	case ss.reqCh <- hello:
	}

	return task.Group(
		channels.NewSink(ss.reqCh).FillFrom(stream.Recv).
			WithStartingStoppingLogs("server_stream", zerolog.TraceLevel).
			WithLogField("nodeID", nid).
			WithLogField("direction", "server.Recv"),
		channels.NewSource(ss.respCh).ForwardTo(stream.Send).
			WithStartingStoppingLogs("server_stream", zerolog.TraceLevel).
			WithLogField("nodeID", nid).
			WithLogField("direction", "server.Send"),
	)(s.ctx)
}

func (s *server) start(ctx context.Context, t *testing.T, g *errgroup.Group) string {
	nl, err := (&net.ListenConfig{}).Listen(ctx, "tcp", ":0")
	if err != nil {
		t.Fatalf("error starting tcp listener: %s", err)
	}

	grpcSrv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	afpb.RegisterCdpiServer(grpcSrv, s)

	g.Go(task.Task(func(ctx context.Context) error {
		return grpcSrv.Serve(nl)
	}).WithCtx(ctx))

	g.Go(task.Task(func(ctx context.Context) error {
		<-ctx.Done()
		grpcSrv.GracefulStop()
		return nil
	}).WithCtx(ctx))

	return nl.Addr().String()
}

func (s *server) WaitForHello(ctx context.Context, nodeID string) (*afpb.CdpiRequest_Hello, error) {
	res, err := s.Recv(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("WaitForHello(%q): %s", nodeID, err)
	}
	return res.GetHello(), nil
}

func (s *server) Recv(ctx context.Context, nodeID string) (*afpb.CdpiRequest, error) {
	ss, ok := s.streams[nodeID]
	if !ok {
		return nil, fmt.Errorf("Recv(%q): unknown node provided", nodeID)
	}

	select {
	case req := <-ss.reqCh:
		return req, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

func (s *server) Send(ctx context.Context, nodeID string, resp *afpb.CdpiResponse) error {
	ss, ok := s.streams[nodeID]
	if !ok {
		return fmt.Errorf("Send(%q): unknown node provided", nodeID)
	}

	select {
	case ss.respCh <- resp:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (s *server) waitForNodeStateUpdate(ctx context.Context, nodeID string) (*afpb.CdpiNodeStateRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	updates := s.stateUpdates[nodeID]
	for len(updates) == 0 && ctx.Err() == nil {
		s.updateCond.Wait()
		updates = s.stateUpdates[nodeID]
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("WaitForUnaryUpdate(%q): timed out waiting for update: %w", nodeID, ctx.Err())
	} else if ctx.Err() != nil {
		return nil, fmt.Errorf("WaitForUnaryUpdate(%q): unknown error waiting for update: %w", nodeID, ctx.Err())
	} else if len(updates) == 0 {
		return nil, fmt.Errorf("WaitForUnaryUpdate(%q): empty list somehow (shouldn't happen)", nodeID)
	}

	update := updates[0]
	s.stateUpdates[nodeID] = updates[1:]
	return update, ctx.Err()
}

func (s *server) waitForRequestUpdate(ctx context.Context, nodeID string) (*afpb.CdpiRequestStatusRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	updates := s.requestUpdates[nodeID]
	for len(updates) == 0 && ctx.Err() == nil {
		s.updateCond.Wait()
		updates = s.requestUpdates[nodeID]
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("WaitForUnaryUpdate(%q): timed out waiting for update: %w", nodeID, ctx.Err())
	} else if ctx.Err() != nil {
		return nil, fmt.Errorf("WaitForUnaryUpdate(%q): unknown error waiting for update: %w", nodeID, ctx.Err())
	} else if len(updates) == 0 {
		return nil, fmt.Errorf("WaitForUnaryUpdate(%q): empty list somehow (shouldn't happen)", nodeID)
	}

	update := updates[0]
	s.requestUpdates[nodeID] = updates[1:]
	return update, ctx.Err()
}

func (s *server) UpdateNodeState(ctx context.Context, newState *afpb.CdpiNodeStateRequest) (*emptypb.Empty, error) {
	nid := newState.GetNodeId()
	s.mu.Lock()
	defer s.mu.Unlock()

	list := s.stateUpdates[nid]
	list = append(list, newState)
	s.stateUpdates[nid] = list
	s.updateCond.Broadcast()

	return &emptypb.Empty{}, nil
}

func (s *server) UpdateRequestStatus(ctx context.Context, reqStatus *afpb.CdpiRequestStatusRequest) (*emptypb.Empty, error) {
	nid := reqStatus.GetNodeId()
	s.mu.Lock()
	defer s.mu.Unlock()

	list := s.requestUpdates[nid]
	list = append(list, reqStatus)
	s.requestUpdates[nid] = list
	s.updateCond.Broadcast()

	return &emptypb.Empty{}, nil
}

func (s *server) checkNoUnreadUnaryUpdates(t *testing.T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for nodeID, updateList := range s.requestUpdates {
		if len(updateList) != 0 {
			t.Errorf("node %q has %d unread request updates", nodeID, len(updateList))
			for _, upd := range updateList {
				t.Logf("missed update: %v", upd.ProtoReflect().Interface())
			}
		}
	}
	for nodeID, updateList := range s.stateUpdates {
		if len(updateList) != 0 {
			t.Errorf("node %q has %d unread state updates", nodeID, len(updateList))
			for _, upd := range updateList {
				t.Logf("missed update: %v", upd.ProtoReflect().Interface())
			}
		}
	}
}

func mustMarshal(t *testing.T, m proto.Message) []byte {
	t.Helper()

	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("mustMarshal(%#v): %s", m, err)
	}
	return b
}

// testFixture holds references to the various moving parts involved in an
// agent test and provides a high-level vocabulary for expressing test steps.
type testFixture struct {
	t     *testing.T
	srv   *server
	eb    *delegatingBackend
	clock clockwork.FakeClock
}

type testNode struct {
	id       string
	priority uint32
}

type testCase struct {
	test  func(context.Context, *testFixture)
	desc  string
	nodes []testNode
}

func (f *testFixture) sendRequest(ctx context.Context, nodeID string, msg *afpb.CdpiResponse) {
	if err := f.srv.Send(ctx, nodeID, msg); err != nil {
		f.t.Errorf("sendRequest(%q, %#v): %s", nodeID, msg.ProtoReflect().Interface(), err)
	}
}

func (f *testFixture) sendRequestWithPayload(ctx context.Context, nodeID string, reqID int64, payload proto.Message) {
	f.t.Helper()

	b, err := proto.Marshal(payload)
	if err != nil {
		f.t.Errorf(
			"sendRequestWithPayload(%q, %#v): error marshalling payload: %s",
			nodeID, payload.ProtoReflect().Interface(), err)
		return
	}

	f.sendRequest(ctx, nodeID, &afpb.CdpiResponse{
		RequestId:      &reqID,
		RequestPayload: b,
	})
}

func (f *testFixture) sendChangeRequest(ctx context.Context, nodeID string, reqID int64, req *afpb.ControlStateChangeRequest) {
	f.sendRequestWithPayload(ctx, nodeID, reqID, req)
}

func (f *testFixture) sendPing(ctx context.Context, nodeID string, reqID, pingID int64) {
	f.sendRequestWithPayload(ctx, nodeID, reqID, &afpb.ControlStateChangeRequest{
		Type: &afpb.ControlStateChangeRequest_ControlPlanePingRequest{
			ControlPlanePingRequest: &afpb.ControlPlanePingRequest{Id: &pingID},
		},
	})
}

func (f *testFixture) sendScheduledDeletion(ctx context.Context, nodeID string, reqID int64, deletion *apipb.ScheduledControlDeletion) {
	f.sendRequestWithPayload(ctx, nodeID, reqID, &afpb.ControlStateChangeRequest{
		Type: &afpb.ControlStateChangeRequest_ScheduledDeletion{
			ScheduledDeletion: deletion,
		},
	})
}

func (f *testFixture) sendScheduledUpdate(ctx context.Context, nodeID string, reqID int64, update *apipb.ScheduledControlUpdate) {
	f.sendRequestWithPayload(ctx, nodeID, reqID, &afpb.ControlStateChangeRequest{
		Type: &afpb.ControlStateChangeRequest_ScheduledUpdate{
			ScheduledUpdate: update,
		},
	})
}

func (f *testFixture) expectNodeStateUpdate(ctx context.Context, nodeID string, want *afpb.CdpiNodeStateRequest) {
	got, err := f.srv.waitForNodeStateUpdate(ctx, nodeID)
	if err != nil {
		f.t.Errorf("expectNodeStateUpdate(%q): failed with error: %s", nodeID, err)
		return
	}

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		f.t.Errorf("expectNodeStateUpdate(%q): proto mismatch: (-want +got):\n%s", nodeID, diff)
		f.t.FailNow()
		return
	}

	zerolog.Ctx(ctx).Debug().Str("nodeID", nodeID).Msg("expectNodeStateUpdate() succeeded")
}

func (f *testFixture) expectRequestStatusUpdate(ctx context.Context, nodeID string, want *afpb.CdpiRequestStatusRequest) {
	got, err := f.srv.waitForRequestUpdate(ctx, nodeID)
	if err != nil {
		f.t.Errorf("expectRequestStatusUpdate(%q): failed with error: %s", nodeID, err)
		return
	}

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		f.t.Errorf("expectRequestStatusUpdate(%q): proto mismatch: (-want +got):\n%s", nodeID, diff)
		f.t.FailNow()
		return
	}

	zerolog.Ctx(ctx).Debug().Str("nodeID", nodeID).Msg("expectRequestStatusUpdate() succeeded")
}

func (f *testFixture) expectHello(ctx context.Context, nodeID string, channelPriority uint32) {
	want := &afpb.CdpiRequest_Hello{
		NodeId:          proto.String(nodeID),
		ChannelPriority: proto.Uint32(channelPriority),
	}

	got, err := f.srv.WaitForHello(ctx, nodeID)
	if err != nil {
		f.t.Errorf("waitForHello(%#v): %s", want.ProtoReflect().Interface(), err)
		f.t.FailNow()
		return
	}

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		f.t.Errorf("waitForHello(%q): proto mismatch: (-want +got):\n%s", nodeID, diff)
		f.t.FailNow()
		return
	}

	zerolog.Ctx(ctx).Debug().Str("nodeID", nodeID).Msg("waitForHello() succeeded")
}

func (f *testFixture) expectHelloAndEmptyInitialState(ctx context.Context, nodeID string) {
	f.expectHello(ctx, nodeID, 0)
	f.expectNodeStateUpdate(ctx, nodeID, &afpb.CdpiNodeStateRequest{
		NodeId: &nodeID,
		State:  &apipb.ControlPlaneState{},
	})
}

func (f *testFixture) expectStreamResponse(ctx context.Context, nodeID string, wantWrapper *afpb.CdpiRequest_Response, wantPayload proto.Message) {
	f.t.Helper()

	if err := f.maybeExpectStreamResponse(ctx, nodeID, wantWrapper, wantPayload); err != nil {
		f.t.Error(err)
		f.t.FailNow()
		return
	}

	zerolog.Ctx(ctx).Debug().Str("nodeID", nodeID).Msg("expectStreamResponse() succeeded")
}

func (f *testFixture) maybeExpectStreamResponse(ctx context.Context, nodeID string, wantWrapper *afpb.CdpiRequest_Response, wantPayload proto.Message) error {
	f.t.Helper()

	wantWrapper.Payload = mustMarshal(f.t, wantPayload)

	got, err := f.srv.Recv(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("maybeExpectStreamResponse(%#v): %s", wantWrapper.ProtoReflect().Interface(), err)
	}

	// check non-payload fields
	if diff := cmp.Diff(wantWrapper, got.GetResponse(), protocmp.Transform(), protocmp.FilterField(wantWrapper, "payload", cmp.Ignore())); diff != "" {
		return fmt.Errorf("maybeExpectStreamResponse(%q): proto mismatch: (-want +got):\n%s", nodeID, diff)
	}

	gotPayload := &afpb.ControlStateNotification{}
	if err := proto.Unmarshal(got.GetResponse().GetPayload(), gotPayload); err != nil {
		return fmt.Errorf("maybeExpectStreamResponse(%q): error unmarshalling payload as ControlStateNotification: %s", nodeID, err)
	}

	if diff := cmp.Diff(wantPayload, gotPayload, protocmp.Transform()); diff != "" {
		return fmt.Errorf("maybeExpectStreamResponse(%q): payload proto mismatch: (-want +got):\n%s", nodeID, diff)
	}

	zerolog.Ctx(ctx).Debug().Str("nodeID", nodeID).Msg("maybeExpectStreamResponse() succeeded")
	return nil
}

func (f *testFixture) expectPong(ctx context.Context, nodeID string, reqID, pingID int64) {
	wantPayload := &afpb.ControlStateNotification{
		ControlPlanePingResponse: &afpb.ControlPlanePingResponse{
			Status:        OK().Proto(),
			Id:            &pingID,
			TimeOfReceipt: timestamppb.New(f.clock.Now()),
		},
	}

	want := &afpb.CdpiRequest_Response{
		RequestId: &reqID,
		Status:    OK().Proto(),
	}

	f.expectStreamResponse(ctx, nodeID, want, wantPayload)
}

func (f *testFixture) advanceClock(ctx context.Context, dur time.Duration) {
	zerolog.Ctx(ctx).Debug().Dur("duration", dur).Msg("advancing clock")
	f.clock.Advance(dur)
}

func (f *testFixture) prepareEnactmentBackendForUpdateID(ctx context.Context, updateID string, eb enactment.Backend) {
	zerolog.Ctx(ctx).Debug().Str("updateID", updateID).Msg("preparing response for update")
	f.eb.setBackendForUpdateID(updateID, eb)
}

func (f *testFixture) prepareEnactmentSuccessForUpdateID(ctx context.Context, updateID string, state *apipb.ControlPlaneState) {
	f.prepareEnactmentBackendForUpdateID(ctx, updateID, func(_ context.Context, _ *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
		return state, nil
	})
}

func (f *testFixture) prepareEnactmentFailureForUpdateID(ctx context.Context, updateID string, err error) {
	f.prepareEnactmentBackendForUpdateID(ctx, updateID, func(_ context.Context, _ *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
		return nil, err
	})
}
