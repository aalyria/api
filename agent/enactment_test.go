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

	"aalyria.com/spacetime/agent/internal/channels"
	"aalyria.com/spacetime/agent/internal/task"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	startTime = time.Now()

	testCases = []testCase{
		{
			desc:  "check Scheduling API session begins with reset and hello",
			nodes: []testNode{{id: "node-a"}},
			test: func(ctx context.Context, f *testFixture) {
				f.expectSchedulingReset(ctx, "node-a")
				f.expectSchedulingHello(ctx, "node-a")
			},
		},
		{
			desc:  "check basic Scheduling API CreateEntry yields backend Dispatch",
			nodes: []testNode{{id: "node-a"}},
			test: func(ctx context.Context, f *testFixture) {
				token := f.expectSchedulingReset(ctx, "node-a")
				f.expectSchedulingHello(ctx, "node-a")

				var requestId int64 = 0
				nextRequestId := func() int64 {
					rval := requestId
					requestId++
					return rval
				}

				var seqno uint64 = 1
				nextSeqno := func() uint64 {
					rval := seqno
					seqno++
					return rval
				}

				// send CreateEntry:SetRoute
				createEntrySetRoute := &schedpb.CreateEntryRequest{
					ScheduleManipulationToken: token,
					Seqno:                     nextSeqno(),
					Id:                        "create-route-1",
					Time:                      timestamppb.New(startTime),
					ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
						SetRoute: &schedpb.SetRoute{
							To:  "2001:db8:1::/48",
							Dev: "eth0",
							Via: "fe80::1",
						},
					},
				}
				reqScheduleCreateEntrySetRoute := &schedpb.ReceiveRequestsMessageFromController{
					RequestId: nextRequestId(),
					Request: &schedpb.ReceiveRequestsMessageFromController_CreateEntry{
						CreateEntry: createEntrySetRoute,
					},
				}
				f.sendSchedulingRequest(ctx, "node-a", reqScheduleCreateEntrySetRoute)

				req := f.waitForDispatchRequestFromController(ctx)
				if diff := cmp.Diff(createEntrySetRoute, req, protocmp.Transform()); diff != "" {
					f.t.Errorf("waitForRequestFromController(): payload proto mismatch: (-want +got):\n%s", diff)
					return
				}
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

	opts := []AgentOption{WithClock(clock)}
	for _, n := range tc.nodes {
		opts = append(opts, WithNode(n.id,
			WithEnactmentDriver(srvAddr, enact, grpc.WithTransportCredentials(insecure.NewCredentials()))))
	}
	agent := newAgent(t, opts...)

	g.Go(func() error { return agent.Run(ctx) })
	tc.test(ctx, &testFixture{t: t, srv: srv, eb: enact, clock: clock})
}

type delegatingBackend struct {
	errs []error
	reqs chan *schedpb.CreateEntryRequest
}

func newDelegatingBackend() *delegatingBackend {
	return &delegatingBackend{
		errs: []error{},
		reqs: make(chan *schedpb.CreateEntryRequest),
	}
}

func (d *delegatingBackend) Init(context.Context) error { return nil }
func (d *delegatingBackend) Close() error               { return nil }
func (d *delegatingBackend) Stats() interface{}         { return nil }

func (d *delegatingBackend) checkNoUnhandledUpdates(t *testing.T) {
	if err := errors.Join(d.errs...); err != nil {
		t.Errorf("delegatingBackend encountered error(s): %s", err)
	}
}

func (d *delegatingBackend) Dispatch(ctx context.Context, req *schedpb.CreateEntryRequest) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case d.reqs <- req:
		return nil
	}
}

type server struct {
	ctx        context.Context
	mu         *sync.Mutex
	updateCond *sync.Cond

	schedpb.UnimplementedSchedulingServer
	agents map[string]*agentSchedulingStream
	tokens map[string]chan string
}

type agentSchedulingStream struct {
	stream         *schedpb.Scheduling_ReceiveRequestsServer
	toController   chan *schedpb.ReceiveRequestsMessageToController
	fromController chan *schedpb.ReceiveRequestsMessageFromController
}

func newServer(ctx context.Context, nodes []testNode) *server {
	agents := map[string]*agentSchedulingStream{}
	tokens := map[string]chan string{}
	for _, n := range nodes {
		agents[n.id] = &agentSchedulingStream{
			toController:   make(chan *schedpb.ReceiveRequestsMessageToController),
			fromController: make(chan *schedpb.ReceiveRequestsMessageFromController),
		}
		tokens[n.id] = make(chan string)
	}

	mu := &sync.Mutex{}
	return &server{
		mu:         mu,
		updateCond: sync.NewCond(mu),
		ctx:        ctx,
		agents:     agents,
		tokens:     tokens,
	}
}

func (s *server) start(ctx context.Context, t *testing.T, g *errgroup.Group) string {
	nl, err := (&net.ListenConfig{}).Listen(ctx, "tcp", ":0")
	if err != nil {
		t.Fatalf("error starting tcp listener: %s", err)
	}

	grpcSrv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	schedpb.RegisterSchedulingServer(grpcSrv, s)

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

func (s *server) ReceiveRequests(stream schedpb.Scheduling_ReceiveRequestsServer) error {
	hello, err := stream.Recv()
	if err != nil {
		return err
	}

	nid := hello.GetHello().GetAgentId()

	s.mu.Lock()
	ss, ok := s.agents[nid]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown node connecting: %q", nid)
	}

	defer close(ss.fromController)

	select {
	case <-s.ctx.Done():
		return context.Canceled
	case ss.toController <- hello:
	}

	return task.Group(
		channels.NewSink(ss.toController).FillFrom(stream.Recv).
			WithStartingStoppingLogs("server_stream", zerolog.TraceLevel).
			WithLogField("nodeID", nid).
			WithLogField("direction", "server.Recv"),
		channels.NewSource(ss.fromController).ForwardTo(stream.Send).
			WithStartingStoppingLogs("server_stream", zerolog.TraceLevel).
			WithLogField("nodeID", nid).
			WithLogField("direction", "server.Send"),
	)(s.ctx)
}

func (s *server) Reset(ctx context.Context, reset *schedpb.ResetRequest) (*emptypb.Empty, error) {
	nid := reset.GetAgentId()

	s.mu.Lock()
	ch, ok := s.tokens[nid]
	if !ok {
		return nil, fmt.Errorf("unknown node reseting: %q", nid)
	}
	s.mu.Unlock()
	go func() {
		select {
		case ch <- reset.GetScheduleManipulationToken():
		}
	}()
	return &emptypb.Empty{}, nil
}

func (s *server) RecvFromNode(ctx context.Context, nid string) (*schedpb.ReceiveRequestsMessageToController, error) {
	ss, ok := s.agents[nid]
	if !ok {
		return nil, fmt.Errorf("RecvFromNode(%q): unknown node provided", nid)
	}

	select {
	case rsp := <-ss.toController:
		return rsp, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

func (s *server) SendToNode(ctx context.Context, nid string, msg *schedpb.ReceiveRequestsMessageFromController) error {
	ss, ok := s.agents[nid]
	if !ok {
		return fmt.Errorf("SendToNode(%q): unknown node provided", nid)
	}

	select {
	case ss.fromController <- msg:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (s *server) WaitForSchedulingReset(ctx context.Context, nid string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	s.mu.Lock()
	ch, ok := s.tokens[nid]
	if !ok {
		return "", fmt.Errorf("unknown node reseting: %q", nid)
	}
	s.mu.Unlock()
	select {
	case token := <-ch:
		return token, nil
	case <-ctx.Done():
		return "", context.Cause(ctx)
	}
}

func (s *server) WaitForSchedulingHello(ctx context.Context, nid string) (*schedpb.ReceiveRequestsMessageToController_Hello, error) {
	hello, err := s.RecvFromNode(ctx, nid)
	if err != nil {
		return nil, err
	}
	return hello.GetHello(), nil
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
	clock *clockwork.FakeClock
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

func (f *testFixture) advanceClock(ctx context.Context, dur time.Duration) {
	zerolog.Ctx(ctx).Debug().Dur("duration", dur).Msg("advancing clock")
	f.clock.Advance(dur)
}

func (f *testFixture) expectSchedulingReset(ctx context.Context, nid string) string {
	token, err := f.srv.WaitForSchedulingReset(ctx, nid)
	if err != nil {
		f.t.Errorf("WaitForSchedulingReset(%q): %s", nid, err)
		f.t.FailNow()
		return ""
	}

	if token == "" {
		f.t.Errorf("WaitForSchedulingReset(%q): empty schedule manipulation token", nid)
		f.t.FailNow()
		return ""
	}

	zerolog.Ctx(ctx).Debug().Str("nodeID", nid).Msg("WaitForSchedulingReset() succeeded")
	return token
}

func (f *testFixture) expectSchedulingHello(ctx context.Context, nid string) {
	hello, err := f.srv.WaitForSchedulingHello(ctx, nid)
	if err != nil {
		f.t.Errorf("WaitForSchedulingHello(%q): %s", nid, err)
		f.t.FailNow()
	}

	if hello.AgentId != nid {
		f.t.Errorf("WaitForSchedulingHello(%q): unexpected agent ID %q", nid, hello.AgentId)
		f.t.FailNow()
	}

	zerolog.Ctx(ctx).Debug().Str("nodeID", nid).Msg("WaitForSchedulingHello() succeeded")
}

func (f *testFixture) sendSchedulingRequest(ctx context.Context, nid string, req *schedpb.ReceiveRequestsMessageFromController) {
	err := f.srv.SendToNode(ctx, nid, req)
	if err != nil {
		f.t.Errorf("sendSchedulingRequest(%q): %s", nid, err)
		f.t.FailNow()
	}
}

func (f *testFixture) waitForDispatchRequestFromController(ctx context.Context) *schedpb.CreateEntryRequest {
	select {
	case <-ctx.Done():
		return nil
	case req := <-f.eb.reqs:
		return req
	}
}
