// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
// Confidential and Proprietary. All rights reserved.
package agent_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"aalyria.com/minkowski/airflow/agent"
	afpb "aalyria.com/minkowski/api/airflow"
	apipb "aalyria.com/minkowski/api/common"

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// enactmentBackendFunc is a wrapper around a function that implements the
// EnactmentBackend interface, suitable for simple enactment backends such as
// those used in testing.
type enactmentBackendFunc struct {
	fn func(ctx context.Context, req *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error)
}

func (e enactmentBackendFunc) HandleRequest(ctx context.Context, req *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
	return e.fn(ctx, req)
}

// cdpiConversation is a request / response pair.
type cdpiConversation struct {
	req  *afpb.ControlStateChangeRequest
	resp *afpb.ControlStateNotification
}

// testCase is a sample conversation to step through and verify.
type testCase struct {
	name         string
	initState    *afpb.ControlStateNotification
	conversation []cdpiConversation
}

func generateTestCases(clock clockwork.Clock) []testCase {
	return []testCase{
		{
			name: "adding a tunnel",
			initState: &afpb.ControlStateNotification{
				NodeId: proto.String("node1"),
				State: &apipb.ControlPlaneState{
					TunnelStates: &apipb.TunnelStates{
						TunnelRuleIds: []string{},
					},
				},
			},
			conversation: []cdpiConversation{
				{
					req: &afpb.ControlStateChangeRequest{
						Type: &afpb.ControlStateChangeRequest_ScheduledUpdate{
							ScheduledUpdate: &apipb.ScheduledControlUpdate{
								TimeToEnact: timestamppb.New(clock.Now()),
								UpdateId:    proto.String("1"),
								Change: &apipb.ControlPlaneUpdate{
									UpdateType: &apipb.ControlPlaneUpdate_TunnelUpdate{
										TunnelUpdate: &apipb.TunnelUpdate{
											TunnelRuleId: proto.String("tun1"),
											Operation:    apipb.TunnelUpdate_ADD.Enum(),
											Rule: &apipb.TunnelRule{
												EncapRule: &apipb.TunnelRule_EncapRule{
													Classifier: &apipb.PacketClassifier{
														IpHeader: &apipb.PacketClassifier_IpHeader{
															Protocol:   proto.Uint32(6), // 6 is TCP
															DstIpRange: proto.String("10.0.0.1"),
														},
													},
													EncapsulatedSrcIp:   proto.String("192.168.1.1"),
													EncapsulatedDstIp:   proto.String("10.0.0.1"),
													EncapsulatedSrcPort: proto.Int32(443),
													EncapsulatedDstPort: proto.Int32(443),
												},
											},
											SequenceNumber: proto.Int64(1),
										},
									},
								},
							},
						},
					},
					resp: &afpb.ControlStateNotification{
						NodeId: proto.String("node1"),
						Statuses: []*apipb.ScheduledControlUpdateStatus{
							{
								UpdateId: proto.String("1"),
								State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
									EnactmentAttempted: status.New(codes.OK, "").Proto(),
								},
							},
						},
						State: &apipb.ControlPlaneState{
							TunnelStates: &apipb.TunnelStates{
								TunnelRuleIds: []string{"tun1"},
							},
						},
					},
				},
			},
		},
		{
			name: "changing beam settings",
			initState: &afpb.ControlStateNotification{
				NodeId: proto.String("high beam node"),
				State: &apipb.ControlPlaneState{
					BeamStates: &apipb.BeamStates{
						BeamTaskIds: []string{},
					},
				},
			},
			conversation: []cdpiConversation{
				{
					req: &afpb.ControlStateChangeRequest{
						Type: &afpb.ControlStateChangeRequest_ScheduledUpdate{
							ScheduledUpdate: &apipb.ScheduledControlUpdate{
								TimeToEnact: timestamppb.New(clock.Now()),
								UpdateId:    proto.String("1"),
								Change: &apipb.ControlPlaneUpdate{
									UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
										BeamUpdate: &apipb.BeamUpdate{
											BeamTaskId: proto.String("bt1"),
											Operation:  apipb.BeamUpdate_ADD.Enum(),
											InterfaceId: &apipb.NetworkInterfaceId{
												NodeId:      proto.String("high beam node"),
												InterfaceId: proto.String("be0"),
											},
										},
									},
								},
							},
						},
					},
					resp: &afpb.ControlStateNotification{
						NodeId: proto.String("high beam node"),
						Statuses: []*apipb.ScheduledControlUpdateStatus{
							{
								UpdateId: proto.String("1"),
								State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
									EnactmentAttempted: status.New(codes.OK, "").Proto(),
								},
							},
						},
						State: &apipb.ControlPlaneState{},
					},
				},
			},
		},
	}
}

func TestAgent(t *testing.T) {
	clock := clockwork.NewFakeClock()

	for _, tc := range generateTestCases(clock) {
		t.Run(tc.name, func(t *testing.T) {
			runTest(t, tc, clock)
		})
	}
}

func runTest(t *testing.T, tc testCase, clock clockwork.Clock) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := startSingleChannelServer(ctx)

	ep, stateCh := channelingBackend()
	agent := agent.NewAgent(ep, clock)
	agent.Start(ctx, fmt.Sprintf("localhost:%d", s.port), grpc.WithTransportCredentials(insecure.NewCredentials()))

	agent.RegisterFor(ctx, tc.initState)
	gotState := <-s.respCh
	assertProtosEqual(t, tc.initState, gotState)

	for _, conv := range tc.conversation {
		// feed the inputs
		s.reqCh <- conv.req
		stateCh <- conv.resp.State

		// get the response from the server
		gotResp := <-s.respCh
		// verify they match
		assertProtosEqual(t, conv.resp, gotResp)
	}
}

func TestFutureEnactment(t *testing.T) {
	ctx, done := context.WithCancel(context.Background())
	defer done()

	s := startSingleChannelServer(ctx)
	clock := clockwork.NewFakeClock()
	ep, stateCh := channelingBackend()

	initState := &afpb.ControlStateNotification{
		NodeId: proto.String("node1"),
	}
	futureChangeReq := &afpb.ControlStateChangeRequest{
		Type: &afpb.ControlStateChangeRequest_ScheduledUpdate{
			ScheduledUpdate: &apipb.ScheduledControlUpdate{
				TimeToEnact: timestamppb.New(clock.Now().Add(5 * time.Second)),
				UpdateId:    proto.String("1"),
				Change: &apipb.ControlPlaneUpdate{
					UpdateType: &apipb.ControlPlaneUpdate_TunnelUpdate{
						TunnelUpdate: &apipb.TunnelUpdate{
							TunnelRuleId: proto.String("tun1"),
							Operation:    apipb.TunnelUpdate_ADD.Enum(),
						},
					},
				},
			},
		},
	}
	wantScheduleAck := &afpb.ControlStateNotification{
		NodeId: proto.String("node1"),
		Statuses: []*apipb.ScheduledControlUpdateStatus{
			{
				UpdateId: proto.String("1"),
				State:    &apipb.ScheduledControlUpdateStatus_Scheduled{},
			},
		},
	}
	wantEnactAck := &afpb.ControlStateNotification{
		NodeId: proto.String("node1"),
		Statuses: []*apipb.ScheduledControlUpdateStatus{
			{
				UpdateId: proto.String("1"),
				State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
					EnactmentAttempted: status.New(codes.OK, "").Proto(),
				},
			},
		},
	}

	agent := agent.NewAgent(ep, clock)
	agent.Start(ctx, fmt.Sprintf("localhost:%d", s.port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	agent.RegisterFor(ctx, initState)

	gotState := <-s.respCh
	assertProtosEqual(t, initState, gotState)

	s.reqCh <- futureChangeReq
	scheduleAck := <-s.respCh
	assertProtosEqual(t, wantScheduleAck, scheduleAck)

	go func() { stateCh <- nil }()

	clock.Advance(5 * time.Second)

	enactAck := <-s.respCh
	assertProtosEqual(t, wantEnactAck, enactAck)
}

func TestDeletedUpdateDoesntGetInvoked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := startSingleChannelServer(ctx)
	clock := clockwork.NewFakeClock()
	initState := &afpb.ControlStateNotification{NodeId: proto.String("cool-node-1")}
	ep := failingBackend(errors.New("shouldn't have been invoked"))
	agent := agent.NewAgent(ep, clock)
	agent.Start(ctx, fmt.Sprintf("localhost:%d", s.port), grpc.WithTransportCredentials(insecure.NewCredentials()))

	scheduleReq := &afpb.ControlStateChangeRequest{
		Type: &afpb.ControlStateChangeRequest_ScheduledUpdate{
			ScheduledUpdate: &apipb.ScheduledControlUpdate{
				TimeToEnact: timestamppb.New(clock.Now().Add(5 * time.Second)),
				UpdateId:    proto.String("101"),
				Change: &apipb.ControlPlaneUpdate{
					UpdateType: &apipb.ControlPlaneUpdate_TunnelUpdate{
						TunnelUpdate: &apipb.TunnelUpdate{
							TunnelRuleId: proto.String("tun1"),
							Operation:    apipb.TunnelUpdate_ADD.Enum(),
						},
					},
				},
			},
		},
	}
	scheduleAck := &afpb.ControlStateNotification{
		NodeId: proto.String("cool-node-1"),
		Statuses: []*apipb.ScheduledControlUpdateStatus{
			{
				UpdateId: proto.String("101"),
				State:    &apipb.ScheduledControlUpdateStatus_Scheduled{},
			},
		},
	}
	deletionReq := &afpb.ControlStateChangeRequest{
		Type: &afpb.ControlStateChangeRequest_ScheduledDeletion{
			ScheduledDeletion: &apipb.ScheduledControlDeletion{
				NodeId:    proto.String("cool-node-1"),
				UpdateIds: []string{"101"},
			},
		},
	}
	delectionAck := &afpb.ControlStateNotification{
		NodeId: proto.String("cool-node-1"),
		Statuses: []*apipb.ScheduledControlUpdateStatus{
			{
				UpdateId: proto.String("101"),
				Timestamp: &apipb.DateTime{
					UnixTimeUsec: proto.Int64(clock.Now().UnixMicro()),
				},
				State: &apipb.ScheduledControlUpdateStatus_Unscheduled{
					Unscheduled: status.New(codes.OK, "").Proto(),
				},
			},
		},
	}

	agent.RegisterFor(ctx, initState)
	gotState := <-s.respCh
	assertProtosEqual(t, initState, gotState)

	s.reqCh <- scheduleReq
	gotState = <-s.respCh
	assertProtosEqual(t, scheduleAck, gotState)

	s.reqCh <- deletionReq
	gotState = <-s.respCh
	assertProtosEqual(t, delectionAck, gotState)
}

func TestCanRegisterMultipleNodes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := startSingleChannelServer(ctx)
	clock := clockwork.NewFakeClock()
	nodeAstate := &afpb.ControlStateNotification{NodeId: proto.String("node-a")}
	nodeBstate := &afpb.ControlStateNotification{NodeId: proto.String("node-b")}
	ep := cannedStateBackend(nil)
	agent := agent.NewAgent(ep, clock)
	agent.Start(ctx, fmt.Sprintf("localhost:%d", s.port), grpc.WithTransportCredentials(insecure.NewCredentials()))

	agent.RegisterFor(ctx, nodeAstate)
	gotAstate := <-s.respCh
	agent.RegisterFor(ctx, nodeBstate)
	gotBstate := <-s.respCh

	assertProtosEqual(t, nodeAstate, gotAstate)
	assertProtosEqual(t, nodeBstate, gotBstate)
}

func TestRejectsStaleChanges(t *testing.T) {
	clock := clockwork.NewFakeClock()

	for name, change := range map[string]*afpb.ControlStateChangeRequest{
		"beam update": {
			Type: &afpb.ControlStateChangeRequest_ScheduledUpdate{
				ScheduledUpdate: &apipb.ScheduledControlUpdate{
					TimeToEnact: timestamppb.New(clock.Now().Add(-5 * time.Second)),
					UpdateId:    proto.String("101"),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_BeamUpdate{
							BeamUpdate: &apipb.BeamUpdate{Operation: apipb.BeamUpdate_ADD.Enum()},
						},
					},
				},
			},
		},
		"radio update": {
			Type: &afpb.ControlStateChangeRequest_ScheduledUpdate{
				ScheduledUpdate: &apipb.ScheduledControlUpdate{
					TimeToEnact: timestamppb.New(clock.Now().Add(-5 * time.Second)),
					UpdateId:    proto.String("101"),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_RadioUpdate{
							RadioUpdate: &apipb.RadioUpdate{},
						},
					},
				},
			},
		},
		"flow update": {
			Type: &afpb.ControlStateChangeRequest_ScheduledUpdate{
				ScheduledUpdate: &apipb.ScheduledControlUpdate{
					TimeToEnact: timestamppb.New(clock.Now().Add(-5 * time.Second)),
					UpdateId:    proto.String("101"),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
							FlowUpdate: &apipb.FlowUpdate{
								Operation: apipb.FlowUpdate_ADD.Enum()},
						},
					},
				},
			},
		},
		"tunnel update": {
			Type: &afpb.ControlStateChangeRequest_ScheduledUpdate{
				ScheduledUpdate: &apipb.ScheduledControlUpdate{
					TimeToEnact: timestamppb.New(clock.Now().Add(-5 * time.Second)),
					UpdateId:    proto.String("101"),
					Change: &apipb.ControlPlaneUpdate{
						UpdateType: &apipb.ControlPlaneUpdate_TunnelUpdate{
							TunnelUpdate: &apipb.TunnelUpdate{
								Operation: apipb.TunnelUpdate_ADD.Enum()},
						},
					},
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			checkStaleChangeIsRejected(t, clock, change)
		})
	}
}

func checkStaleChangeIsRejected(t *testing.T, clock clockwork.Clock, change *afpb.ControlStateChangeRequest) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := startSingleChannelServer(ctx)
	nodeAstate := &afpb.ControlStateNotification{NodeId: proto.String("node-a")}
	ep := cannedStateBackend(nil)
	agent := agent.NewAgent(ep, clock)
	agent.Start(ctx, fmt.Sprintf("localhost:%d", s.port), grpc.WithTransportCredentials(insecure.NewCredentials()))

	agent.RegisterFor(ctx, nodeAstate)
	<-s.respCh

	// send a stale request
	s.reqCh <- change
	nackResp := <-s.respCh

	assertProtosEqual(t, &afpb.ControlStateNotification{
		NodeId: proto.String("node-a"),
		Statuses: []*apipb.ScheduledControlUpdateStatus{
			{
				UpdateId: change.GetScheduledUpdate().UpdateId,
				State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
					EnactmentAttempted: status.New(codes.DeadlineExceeded, "Update received by node but is too old to enact").Proto(),
				},
			},
		},
	}, nackResp)
}

func assertProtosEqual(t *testing.T, want, got interface{}) {
	t.Helper()
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("proto mismatch: (-want +got):\n%s", diff)
		t.FailNow()
	}
}

// SingleChannelServer provides an implementation of the ControlPlaneInterface
// that passes messages via channels to aid in testing.
// TODO: put a done channel in here too
type SingleChannelServer struct {
	reqCh  chan *afpb.ControlStateChangeRequest
	respCh chan *afpb.ControlStateNotification
	port   int

	afpb.UnimplementedNetworkControllerStreamingServer
}

func (c *SingleChannelServer) ControlPlaneInterface(stream afpb.NetworkControllerStreaming_ControlPlaneInterfaceServer) error {
	g, ctx := errgroup.WithContext(context.Background())

	// reader routine
	g.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
				resp, err := stream.Recv()
				if err != nil {
					return err
				}
				c.respCh <- resp
			}
		}
	})
	// writer routine
	g.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case req := <-c.reqCh:
				if err := stream.Send(req); err != nil {
					return err
				}
			}
		}
	})

	return g.Wait()
}

func startSingleChannelServer(ctx context.Context) *SingleChannelServer {
	wg := sync.WaitGroup{}
	wg.Add(1)

	nl, err := net.Listen("tcp", ":0")
	if err != nil {
		panic("startSingleChannelServer: couldn't Listen")
	}

	s := &SingleChannelServer{
		reqCh:  make(chan *afpb.ControlStateChangeRequest),
		respCh: make(chan *afpb.ControlStateNotification),
		port:   nl.Addr().(*net.TCPAddr).Port,
	}

	go func() {
		defer nl.Close()

		grpcSrv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
		afpb.RegisterNetworkControllerStreamingServer(grpcSrv, s)

		go grpcSrv.Serve(nl)
		// Signal that we've begun serving
		wg.Done()

		<-ctx.Done()
		grpcSrv.Stop()
	}()

	// Wait until the server has started
	wg.Wait()
	return s
}

// failingBackend returns an EnactmentBackend that always fails with the
// provided error.
func failingBackend(err error) agent.EnactmentBackend {
	return enactmentBackendFunc{
		func(_ context.Context, _ *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
			return nil, err
		},
	}
}

// cannedStateBackend returns an EnactmentBackend that always returns the
// provided ControlPlaneState.
func cannedStateBackend(s *apipb.ControlPlaneState) agent.EnactmentBackend {
	return enactmentBackendFunc{
		func(_ context.Context, _ *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
			return s, nil
		},
	}
}

// channelingBackend returns an EnactmentBackend that responds to enactment
// requests with responses reads from a side channel.
func channelingBackend() (agent.EnactmentBackend, chan *apipb.ControlPlaneState) {
	ch := make(chan *apipb.ControlPlaneState)

	eb := enactmentBackendFunc{
		func(_ context.Context, _ *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
			n := <-ch
			return n, nil
		},
	}

	return eb, ch
}
