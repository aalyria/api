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
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	afpb "aalyria.com/spacetime/api/cdpi/v1alpha"
	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/enactment"
	"aalyria.com/spacetime/cdpi_agent/internal/channels"

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
	"google.golang.org/protobuf/types/known/timestamppb"
)

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

func TestAgentValidation(t *testing.T) {
	_, err := NewAgent()
	if err == nil {
		t.Fatalf("expected agent with no options to be invalid")
	}
}

func TestAgent(t *testing.T) {
	clock := clockwork.NewFakeClock()

	for _, tc := range generateTestCases(clock) {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTest(t, tc, clock)
		})
	}
}

func runTest(t *testing.T, tc testCase, clock clockwork.Clock) {
	ctx, cancel := context.WithTimeout(baseContext(), time.Second)
	defer cancel()

	s := startSingleChannelServer(ctx)

	ep, stateCh := channelingBackend()
	agent := newAgent(t,
		WithClock(clock),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithServerEndpoint(fmt.Sprintf("localhost:%d", s.port)),
		WithNode(
			tc.initState.GetNodeId(),
			WithInitialState(tc.initState),
			WithEnactmentBackend(ep)))

	errCh := make(chan error)
	go func() { errCh <- agent.Run(ctx) }()

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

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestFutureEnactment(t *testing.T) {
	ctx, cancel := context.WithTimeout(baseContext(), time.Second)
	defer cancel()

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

	agent := newAgent(t,
		WithClock(clock),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithServerEndpoint(fmt.Sprintf("localhost:%d", s.port)),
		WithNode(
			initState.GetNodeId(),
			WithInitialState(initState),
			WithEnactmentBackend(ep)))

	errCh := make(chan error)
	go func() { errCh <- agent.Run(ctx) }()

	gotState := <-s.respCh
	assertProtosEqual(t, initState, gotState)

	s.reqCh <- futureChangeReq
	scheduleAck := <-s.respCh
	assertProtosEqual(t, wantScheduleAck, scheduleAck)

	go func() { stateCh <- nil }()

	clock.Advance(5 * time.Second)

	enactAck := <-s.respCh
	assertProtosEqual(t, wantEnactAck, enactAck)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestDeletedUpdateDoesntGetInvoked(t *testing.T) {
	ctx, cancel := context.WithTimeout(baseContext(), time.Second)
	defer cancel()

	s := startSingleChannelServer(ctx)
	clock := clockwork.NewFakeClock()
	initState := &afpb.ControlStateNotification{NodeId: proto.String("cool-node-1")}
	ep := failingBackend(errors.New("shouldn't have been invoked"))
	agent := newAgent(t,
		WithClock(clock),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithServerEndpoint(fmt.Sprintf("localhost:%d", s.port)),
		WithNode(
			initState.GetNodeId(),
			WithInitialState(initState),
			WithEnactmentBackend(ep)))

	errCh := make(chan error)
	go func() { errCh <- agent.Run(ctx) }()

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

	gotState := <-s.respCh
	assertProtosEqual(t, initState, gotState)

	s.reqCh <- scheduleReq
	gotState = <-s.respCh
	assertProtosEqual(t, scheduleAck, gotState)

	s.reqCh <- deletionReq
	gotState = <-s.respCh
	assertProtosEqual(t, delectionAck, gotState)

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
}

func TestCanRegisterMultipleNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(baseContext(), time.Second)
	defer cancel()

	s := startSingleChannelServer(ctx)
	clock := clockwork.NewFakeClock()
	nodeAstate := &afpb.ControlStateNotification{NodeId: proto.String("node-a")}
	nodeBstate := &afpb.ControlStateNotification{NodeId: proto.String("node-b")}
	ep := cannedStateBackend(nil)

	agent := newAgent(t,
		WithClock(clock),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithServerEndpoint(fmt.Sprintf("localhost:%d", s.port)),
		WithNode(
			nodeAstate.GetNodeId(),
			WithInitialState(nodeAstate),
			WithEnactmentBackend(ep)),
		WithNode(
			nodeBstate.GetNodeId(),
			WithInitialState(nodeBstate),
			WithEnactmentBackend(ep)))

	errCh := make(chan error)
	go func() { errCh <- agent.Run(ctx) }()

	gotStates := []*afpb.ControlStateNotification{<-s.respCh, <-s.respCh}
	sort.Slice(gotStates, func(i, j int) bool { return gotStates[i].GetNodeId() < gotStates[j].GetNodeId() })

	assertProtosEqual(t, nodeAstate, gotStates[0])
	assertProtosEqual(t, nodeBstate, gotStates[1])

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
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
		change := change
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			checkStaleChangeIsRejected(t, clock, change)
		})
	}
}

func checkStaleChangeIsRejected(t *testing.T, clock clockwork.Clock, change *afpb.ControlStateChangeRequest) {
	ctx, cancel := context.WithTimeout(baseContext(), time.Second)
	defer cancel()

	s := startSingleChannelServer(ctx)
	nodeAstate := &afpb.ControlStateNotification{NodeId: proto.String("node-a")}
	ep := cannedStateBackend(nil)

	agent := newAgent(t,
		WithClock(clock),
		WithDialOpts(grpc.WithTransportCredentials(insecure.NewCredentials())),
		WithServerEndpoint(fmt.Sprintf("localhost:%d", s.port)),
		WithNode(
			nodeAstate.GetNodeId(),
			WithInitialState(nodeAstate),
			WithEnactmentBackend(ep)))
	errCh := make(chan error)
	go func() { errCh <- agent.Run(ctx) }()

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

	cancel()
	checkErrIsDueToCanceledContext(t, <-errCh)
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
	g, ctx := errgroup.WithContext(stream.Context())

	g.Go(channels.NewSink(c.respCh).FillFrom(stream.Recv).WithCtx(ctx))
	g.Go(channels.NewSource(c.reqCh).ForwardTo(stream.Send).WithCtx(ctx))

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

// failingBackend returns an enactment.Backend that always fails with the
// provided error.
func failingBackend(err error) enactment.Backend {
	return func(_ context.Context, _ *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
		return nil, err
	}
}

// cannedStateBackend returns an enactment.Backend that always returns the
// provided ControlPlaneState.
func cannedStateBackend(s *apipb.ControlPlaneState) enactment.Backend {
	return func(_ context.Context, _ *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
		return s, nil
	}
}

// channelingBackend returns an enactment.Backend that responds to enactment
// requests with responses reads from a side channel.
func channelingBackend() (enactment.Backend, chan *apipb.ControlPlaneState) {
	ch := make(chan *apipb.ControlPlaneState)

	eb := func(_ context.Context, _ *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error) {
		n := <-ch
		return n, nil
	}

	return eb, ch
}

func baseContext() context.Context {
	log := zerolog.New(zerolog.ConsoleWriter{
		Out: os.Stdout,
	}).With().Timestamp().Logger()
	return log.WithContext(context.Background())
}

func newAgent(t *testing.T, opts ...AgentOption) *Agent {
	t.Helper()

	a, err := NewAgent(opts...)
	if err != nil {
		t.Fatalf("error creating agent: %s", err)
	}
	return a
}

func check(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}

func assertProtosEqual(t *testing.T, want, got interface{}) {
	t.Helper()

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("proto mismatch: (-want +got):\n%s", diff)
		t.FailNow()
	}
}

var rpcCanceledError = status.FromContextError(context.Canceled).Err()

func checkErrIsDueToCanceledContext(t *testing.T, err error) {
	if !errors.Is(err, context.Canceled) && !errors.Is(err, rpcCanceledError) {
		t.Error("unexpected error:", err)
	}
}
