// Package agent provides a CDPI agent implementation.
//
// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
// Confidential and Proprietary. All rights reserved.
package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	afpb "aalyria.com/minkowski/api/airflow"
	apipb "aalyria.com/minkowski/api/common"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Agent is a CDPI agent that coordinates change requests across multiple
// nodes.
type Agent struct {
	mu      sync.Mutex
	streams map[string]*agentStream

	cdpiClient afpb.NetworkControllerStreamingClient
	clock      clockwork.Clock
	impl       EnactmentBackend
}

// EnactmentBackend is the component that takes a ScheduledControlUpdate
// message for a given node and returns the new state for that node. If the
// error returned implements the gRPCStatus interface, the appropriate status
// will be used.
type EnactmentBackend interface {
	HandleRequest(ctx context.Context, req *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error)
}

// NewAgent creates a new Agent configured to use the provided EnactmentBackend
// and Clock.
func NewAgent(impl EnactmentBackend, clock clockwork.Clock) *Agent {
	return &Agent{streams: make(map[string]*agentStream), impl: impl, clock: clock}
}

// Start starts the Agent. This needs to be called before registering any
// nodes.
func (a *Agent) Start(ctx context.Context, serverAddr string, dialOpts ...grpc.DialOption) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cdpiClient != nil {
		return errors.New("agent: Agent already started, can't start again")
	}

	conn, err := grpc.DialContext(ctx, serverAddr, dialOpts...)
	if err != nil {
		return fmt.Errorf("agent: failed connecting to CDPI backend: %w", err)
	}

	a.cdpiClient = afpb.NewNetworkControllerStreamingClient(conn)
	return nil
}

// RegisterFor registers a given node with the agent. The corresponding stream
// will be shutdown when the provided context is cancelled.
func (a *Agent) RegisterFor(ctx context.Context, initState *afpb.ControlStateNotification) (<-chan error, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// safety checks
	if initState == nil || initState.NodeId == nil {
		return nil, errors.New("agent: no NodeID provided in initial state, can't register with backend")
	}

	if a.cdpiClient == nil {
		return nil, errors.New("goagent: agent.Start() not called, can't register")
	}

	node := *initState.NodeId
	if _, exists := a.streams[node]; exists {
		return nil, fmt.Errorf("agent: Already registered for node %s, can't register again", node)
	}

	ctx, done := context.WithCancel(ctx)
	as := &agentStream{
		id:               node,
		done:             done,
		impl:             a.impl,
		scheduledUpdates: map[string]scheduledUpdate{},
		clock:            a.clock,
	}
	a.streams[node] = as

	errCh := make(chan error)
	go func() {
		errCh <- as.start(ctx, a.cdpiClient, initState)
		close(errCh)
	}()
	return errCh, nil
}

func (a *Agent) UnregisterFor(ctx context.Context, node string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	as := a.streams[node]
	if as == nil {
		return fmt.Errorf("agent: Not registered for node %s, can't unregister", node)
	}
	delete(a.streams, node)

	return as.stop()
}

type scheduledUpdate struct {
	update *afpb.ControlStateChangeRequest
	timer  safeTimer
}

// safeTimer is a tiny wrapper around the clockwork.Timer interface that also
// provides a channel that listeners can use to determine if the timer has been
// stopped (otherwise reads from the t.Chan() channel will block forever if the
// timer is cancelled before it fires). Timer instances should not be reused.
//
// This can be removed once
// https://github.com/jonboulle/clockwork/commit/d574a97c1e79cc70d6ee2a5e6e690a6f1be6be3b
// is tagged in a release.
type safeTimer struct {
	t        clockwork.Timer
	done     chan struct{}
	stopOnce *sync.Once
}

func (s *safeTimer) Stop() bool {
	s.stopOnce.Do(func() { close(s.done) })

	return s.t.Stop()
}

type agentStream struct {
	mu sync.Mutex
	// The node ID this stream is responsible for.
	id string
	// Called when the agentStream should disconnect.
	done func()
	impl EnactmentBackend
	// Maps update IDs => scheduledUpdates.
	scheduledUpdates map[string]scheduledUpdate
	clock            clockwork.Clock
}

func (as *agentStream) stop() error {
	as.done()
	return nil
}

func (as *agentStream) start(ctx context.Context, ncClient afpb.NetworkControllerStreamingClient, initState *afpb.ControlStateNotification) error {
	log := zerolog.Ctx(ctx)

	defer as.done()
	defer func() {
		log.Debug().Msg("ending stream")
	}()

	for {
		select {
		case <-ctx.Done():
			return nil

		default:
		}

		// TODO: does init state need to change between invocations?
		if err := as.startStream(ctx, ncClient, initState); err != nil {
			if s := status.Convert(err); s.Code() == codes.Unauthenticated {
				log.Error().Err(err).Msg("authentication error, won't retry connection")
				return err
			}

			// We don't need to implement exponential backoff here because the
			// client (ncClient) already transparently handles connection
			// backoffs, so a naive sleep is sufficient here.
			// TODO: move this to a config value
			backoffMs := 5_000
			randFact := rand.Float64() - 0.5
			jitterMs := int(math.Round(randFact * float64(backoffMs)))
			delayDur := time.Duration(backoffMs+jitterMs) * time.Millisecond

			log.Error().
				Err(err).
				Dur("backoffDelay", delayDur).
				Msg("error on node stream, retrying shortly")

			as.clock.Sleep(delayDur)
		}
	}
}

func (as *agentStream) startStream(ctx context.Context, ncClient afpb.NetworkControllerStreamingClient, initState *afpb.ControlStateNotification) error {
	log := zerolog.Ctx(ctx)

	ctx, done := context.WithCancel(ctx)
	defer done()

	g, ctx := errgroup.WithContext(ctx)
	cpi, err := ncClient.ControlPlaneInterface(ctx)
	if err != nil {
		return fmt.Errorf("error invoking the ControlPlaneInterface: %w", err)
	}

	if err := cpi.Send(initState); err != nil {
		return fmt.Errorf("error sending initial state: %w", err)
	}

	reqCh := make(chan *afpb.ControlStateChangeRequest)
	g.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
				req, err := cpi.Recv()
				if err != nil {
					return err
				}

				select {
				case reqCh <- req:
				case <-ctx.Done():
					log.Warn().Msg("discarding received request because context was cancelled")
					return nil
				}
			}
		}
	})

	propagateError := func(err error) {
		g.Go(func() error { return err })
	}

	log.Debug().Msg("entering control loop")

sendLoop:
	for {
		select {
		case <-ctx.Done():
			break sendLoop

		case req := <-reqCh:
			switch r := req.Type.(type) {
			case *afpb.ControlStateChangeRequest_ScheduledUpdate:
				updateAck := as.handleScheduledUpdate(ctx, req, reqCh)
				if err := cpi.Send(updateAck); err != nil {
					propagateError(fmt.Errorf("error sending response to scheduled update: %w", err))
					break sendLoop
				}

			case *afpb.ControlStateChangeRequest_ScheduledDeletion:
				deletionAck := as.handleScheduledDeletion(ctx, r.ScheduledDeletion)
				if err := cpi.Send(deletionAck); err != nil {
					propagateError(fmt.Errorf("error sending response to scheduled deletion: %w", err))
					break sendLoop
				}

			case *afpb.ControlStateChangeRequest_ControlPlanePingRequest:
				pong := as.handlePingRequest(ctx, r.ControlPlanePingRequest)
				if err := cpi.Send(pong); err != nil {
					propagateError(fmt.Errorf("error sending ping response: %w", err))
					break sendLoop
				}
			}
		}
	}

	return g.Wait()
}

func (as *agentStream) handleScheduledUpdate(ctx context.Context, req *afpb.ControlStateChangeRequest, reqCh chan<- *afpb.ControlStateChangeRequest) *afpb.ControlStateNotification {
	log := zerolog.Ctx(ctx)

	// TODO: check that this field is set
	upd := req.GetScheduledUpdate()
	tte := upd.TimeToEnact.AsTime()

	log.Debug().
		Time("now", as.clock.Now()).
		Time("tte", tte).
		Msg("handling scheduled update")

	if tte.After(as.clock.Now()) {
		return as.handleFutureScheduledUpdate(ctx, req, reqCh)
	} else if tte.Before(as.clock.Now()) && shouldRejectStaleRequest(upd) {
		return as.rejectStaleRequest(ctx, upd)
	} else {
		return as.handleImmediateScheduledUpdate(ctx, req)
	}
}

func (as *agentStream) handleFutureScheduledUpdate(ctx context.Context, req *afpb.ControlStateChangeRequest, reqCh chan<- *afpb.ControlStateChangeRequest) *afpb.ControlStateNotification {
	upd := req.GetScheduledUpdate()
	updateID := *upd.UpdateId
	tte := upd.TimeToEnact.AsTime()

	delay := tte.Sub(as.clock.Now())
	// TODO: check for dupes

	log := zerolog.Ctx(ctx).With().Str("updateID", updateID).Logger()

	as.mu.Lock()
	timer := safeTimer{
		t:        as.clock.NewTimer(delay),
		done:     make(chan struct{}),
		stopOnce: &sync.Once{},
	}

	go func() {
		select {
		case <-ctx.Done():
			log.Warn().Msg("discarding pending request because context was cancelled")
			timer.Stop()
			return
		case <-timer.done:
			return

		case <-timer.t.Chan():
		}

		select {
		case reqCh <- req:
		case <-ctx.Done():
			log.Warn().Msg("discarding pending request because context was cancelled")
		}
	}()
	as.scheduledUpdates[updateID] = scheduledUpdate{
		update: req,
		timer:  timer,
	}
	as.mu.Unlock()

	return &afpb.ControlStateNotification{
		NodeId: proto.String(as.id),
		Statuses: []*apipb.ScheduledControlUpdateStatus{
			{
				UpdateId: &updateID,
				State:    &apipb.ScheduledControlUpdateStatus_Scheduled{}},
		},
	}
}

func (as *agentStream) handleImmediateScheduledUpdate(ctx context.Context, req *afpb.ControlStateChangeRequest) *afpb.ControlStateNotification {
	upd := req.GetScheduledUpdate()
	updateID := *upd.UpdateId
	log := zerolog.Ctx(ctx).With().Str("updateID", updateID).Logger()

	as.mu.Lock()
	if su, ok := as.scheduledUpdates[updateID]; ok {
		su.timer.Stop()
		delete(as.scheduledUpdates, updateID)
		log.Debug().Msg("deleting scheduled update")
	}
	as.mu.Unlock()

	// handle update
	newState, err := as.impl.HandleRequest(ctx, upd)
	if err != nil {
		log.Error().Err(err).Msg("error handling update")

		return &afpb.ControlStateNotification{
			NodeId: proto.String(as.id),
			Statuses: []*apipb.ScheduledControlUpdateStatus{{
				UpdateId: &updateID,
				State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
					EnactmentAttempted: status.Convert(err).Proto(),
				},
			}},
		}
	}

	return &afpb.ControlStateNotification{
		NodeId: proto.String(as.id),
		State:  newState,
		Statuses: []*apipb.ScheduledControlUpdateStatus{{
			UpdateId: &updateID,
			State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
				EnactmentAttempted: status.New(codes.OK, "").Proto(),
			},
		}},
	}
}

func (as *agentStream) handleScheduledDeletion(ctx context.Context, r *apipb.ScheduledControlDeletion) *afpb.ControlStateNotification {
	log := zerolog.Ctx(ctx).With().Strs("updateIDs", r.UpdateIds).Logger()
	log.Debug().Msg("received scheduled deletion")

	notif := &afpb.ControlStateNotification{NodeId: proto.String(as.id)}

	as.mu.Lock()
	for _, updateID := range r.UpdateIds {
		updateStatus := &apipb.ScheduledControlUpdateStatus{
			UpdateId: proto.String(updateID),
			Timestamp: &apipb.DateTime{
				UnixTimeUsec: proto.Int64(as.clock.Now().UnixMicro()),
			},
		}

		su, exists := as.scheduledUpdates[updateID]
		if exists && su.timer.Stop() {
			delete(as.scheduledUpdates, updateID)
			updateStatus.State = &apipb.ScheduledControlUpdateStatus_Unscheduled{
				Unscheduled: status.New(codes.OK, "").Proto(),
			}
		} else {
			// the scheduled update has already fired, so the deletion has failed
			updateStatus.State = &apipb.ScheduledControlUpdateStatus_Unscheduled{
				Unscheduled: status.New(codes.DeadlineExceeded, "scheduled update already attempted").Proto(),
			}
		}
		notif.Statuses = append(notif.Statuses, updateStatus)
	}
	as.mu.Unlock()

	return notif
}

func (as *agentStream) handlePingRequest(ctx context.Context, r *afpb.ControlPlanePingRequest) *afpb.ControlStateNotification {
	log := zerolog.Ctx(ctx).With().Int64("pingID", *r.Id).Logger()

	log.Debug().Msg("handling ping request")

	return &afpb.ControlStateNotification{
		NodeId: proto.String(as.id),
		ControlPlanePingResponse: &afpb.ControlPlanePingResponse{
			Id:            r.Id,
			Status:        status.New(codes.OK, "").Proto(),
			TimeOfReceipt: timestamppb.New(as.clock.Now()),
		},
	}
}

// The time to enact is in the past. If the update is addition, the scheduler
// will reject this delayed request; otherwise (i.e., the update is a
// deletion), the scheduler should still attempt to schedule all updates
// ASAP.
func shouldRejectStaleRequest(r *apipb.ScheduledControlUpdate) bool {
	switch c := r.Change; c.UpdateType.(type) {
	case *apipb.ControlPlaneUpdate_BeamUpdate:
		return c.GetBeamUpdate().GetOperation() == apipb.BeamUpdate_ADD
	case *apipb.ControlPlaneUpdate_RadioUpdate:
		return true
	case *apipb.ControlPlaneUpdate_FlowUpdate:
		return c.GetFlowUpdate().GetOperation() == apipb.FlowUpdate_ADD
	case *apipb.ControlPlaneUpdate_TunnelUpdate:
		return c.GetTunnelUpdate().GetOperation() == apipb.TunnelUpdate_ADD
	default:
		return true
	}
}

func (as *agentStream) rejectStaleRequest(ctx context.Context, r *apipb.ScheduledControlUpdate) *afpb.ControlStateNotification {
	zerolog.Ctx(ctx).
		Error().
		Str("updateID", *r.UpdateId).
		Time("tte", r.TimeToEnact.AsTime()).
		Msg("Rejecting update because it is too old to enact")

	return &afpb.ControlStateNotification{
		NodeId: proto.String(as.id),
		Statuses: []*apipb.ScheduledControlUpdateStatus{{
			UpdateId: r.UpdateId,
			State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
				EnactmentAttempted: status.New(codes.DeadlineExceeded, "Update received by node but is too old to enact").Proto(),
			},
		}},
	}
}
