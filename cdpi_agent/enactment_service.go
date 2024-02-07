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
	"fmt"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	afpb "aalyria.com/spacetime/api/cdpi/v1alpha"
	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/enactment"
	"aalyria.com/spacetime/cdpi_agent/internal/channels"
	"aalyria.com/spacetime/cdpi_agent/internal/loggable"
	"aalyria.com/spacetime/cdpi_agent/internal/task"
	"aalyria.com/spacetime/cdpi_agent/internal/worker"
)

// we keep already attempted enactments in memory for this long to help resolve
// issues where the CDPI server might try and update a request after it's been
// applied.
const attemptedUpdateKeepAliveTimeout = 1 * time.Minute

func OK() *status.Status { return status.New(codes.OK, "") }

type enactmentService struct {
	// protects access to the scheduledUpdates map.
	mu sync.Mutex
	// maps updateIDs => scheduledUpdates
	scheduledUpdates map[string]*scheduledUpdate
	cc               afpb.CdpiClient
	eb               enactment.Backend
	clock            clockwork.Clock
	initState        *apipb.ControlPlaneState
	nodeID           string
	priority         uint32
}

type scheduledUpdate struct {
	timer  clockwork.Timer
	Update *apipb.ScheduledControlUpdate
	Status *status.Status
	Result *apipb.ControlPlaneState
}

func (nc *nodeController) newEnactmentService(cc afpb.CdpiClient, eb enactment.Backend) *enactmentService {
	return &enactmentService{
		eb:               eb,
		cc:               cc,
		mu:               sync.Mutex{},
		scheduledUpdates: map[string]*scheduledUpdate{},
		clock:            nc.clock,
		nodeID:           nc.id,
		initState:        nc.initState,
		priority:         nc.priority,
	}
}

func (es *enactmentService) Stats() interface{} {
	return struct {
		Backend          interface{}
		ScheduledUpdates map[string]*scheduledUpdate
		Priority         uint32
	}{
		Backend:          es.eb.Stats(),
		ScheduledUpdates: es.scheduledUpdates,
		Priority:         es.priority,
	}
}

func (es *enactmentService) run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	if err := es.eb.Init(ctx); err != nil {
		return fmt.Errorf("%T.Init() failed: %w", es.eb, err)
	}

	stream, err := es.cc.Cdpi(ctx)
	if err != nil {
		return fmt.Errorf("error invoking the Cdpi interface: %w", err)
	}

	// sendCh contains the messages queued to send over the long-lived gRPC
	// stream.
	sendCh := make(chan *afpb.CdpiRequest)
	// recvCh contains the messages received over the gRPC stream
	recvCh := make(chan *afpb.CdpiResponse)
	// updateCh contains the updates that need to be applied immediately
	updateCh := make(chan *apipb.ScheduledControlUpdate)

	scheduler := &enactmentScheduler{
		mu: sync.Mutex{},
		beamQ: worker.NewMapQueue(ctx, g, func(u *apipb.ScheduledControlUpdate) string {
			return u.GetChange().GetBeamUpdate().GetInterfaceId().GetInterfaceId()
		}, es.applyUpdate),
		radioQ: worker.NewMapQueue(ctx, g, func(u *apipb.ScheduledControlUpdate) string {
			return u.GetChange().GetRadioUpdate().GetInterfaceId()
		}, es.applyUpdate),
		flowQ: worker.NewMapQueue(ctx, g, func(u *apipb.ScheduledControlUpdate) string {
			// TODO: this is inelegant to say the least, but there's
			// *always one forward rule present here.
			for _, b := range u.GetChange().GetFlowUpdate().GetRule().GetActionBucket() {
				for _, a := range b.GetAction() {
					switch a.ActionType.(type) {
					case *apipb.FlowRule_ActionBucket_Action_Forward_:
						return a.GetForward().GetOutInterfaceId()
					}
				}
			}
			return ""
		}, es.applyUpdate),
		tunnelQ: worker.NewSerialQueue(ctx, g, es.applyUpdate),
	}

	// Something like this (remember that a CdpiResponse is what the server
	// sends this service and a CdpiRequest is what this service responds
	// with):
	//
	// ┌──────┐    ┌────────┐   ┌──────┐    ┌────────┐     ┌─────────┐
	// │recvCh│    │mainLoop│   │sendCh│    │updateCh│     │EmitEvent│
	// └──┬───┘    └───┬────┘   └──┬───┘    └───┬────┘     └────┬────┘
	//    │            │           │            │               │
	//    │CdpiResponse│           │            │               │
	//    │───────────>│           │            │               │
	//    │            │           │            │               │
	//    │            │CdpiRequest│            │               │
	//    │            │──────────>│            │               │
	//    │            │           │            │               │
	//    │            │ ScheduledControlUpdate │               │
	//    │            │───────────────────────>│               │
	//    │            │           │            │               │
	//    │            │           │            │CdpiClientEvent│
	//    │            │           │            │──────────────>│
	// ┌──┴───┐    ┌───┴────┐   ┌──┴───┐    ┌───┴────┐     ┌────┴────┐
	// │recvCh│    │mainLoop│   │sendCh│    │updateCh│     │EmitEvent│
	// └──────┘    └────────┘   └──────┘    └────────┘     └─────────┘

	g.Go(channels.NewSource(sendCh).ForwardTo(stream.Send).
		WithStartingStoppingLogs("sendLoop", zerolog.TraceLevel).
		WithLogField("task", "send").
		WithNewSpan("sendLoop").
		WithPanicCatcher().
		WithCtx(ctx))

	g.Go(channels.NewSink(recvCh).FillFrom(stream.Recv).
		WithStartingStoppingLogs("recvLoop", zerolog.TraceLevel).
		WithLogField("task", "recv").
		WithPanicCatcher().
		WithCtx(ctx))

	g.Go(es.enactmentLoop(updateCh, scheduler).
		WithStartingStoppingLogs("enactmentLoop", zerolog.TraceLevel).
		WithLogField("task", "enactment").
		WithNewSpan("enactmentLoop").
		WithPanicCatcher().
		WithCtx(ctx))

	g.Go(es.mainLoop(recvCh, sendCh, updateCh).
		WithStartingStoppingLogs("mainLoop", zerolog.TraceLevel).
		WithLogField("task", "main").
		WithNewSpan("mainLoop").
		WithPanicCatcher().
		WithCtx(ctx))

	g.Go(func() error {
		hello := es.hello()
		zerolog.Ctx(ctx).Debug().Object("hello", loggable.Proto(hello)).Msg("sending hello")
		select {
		case sendCh <- hello:
			return nil
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	})

	if es.initState != nil {
		if es.initState.ForwardingState != nil {
			es.initState.GetForwardingState().Timestamp = timeToProto(es.clock.Now())
		}

		g.Go(func() error {
			_, err := es.cc.UpdateNodeState(ctx, &afpb.CdpiNodeStateRequest{
				NodeId: &es.nodeID,
				State:  es.initState,
			})
			return err
		})
	}

	return g.Wait()
}

func (es *enactmentService) hello() *afpb.CdpiRequest {
	return &afpb.CdpiRequest{
		Hello: &afpb.CdpiRequest_Hello{
			NodeId:          &es.nodeID,
			ChannelPriority: proto.Uint32(es.priority),
		},
	}
}

// enactmentLoop reads change requests from the `updateCh`, applies them, and
// uses the unary NotifyControlUpdateStatus method to deliver updates to the
// CDPI server. All enactments should go through this channel to ensure that
// they're applied serially.
func (es *enactmentService) enactmentLoop(updateCh <-chan *apipb.ScheduledControlUpdate, scheduler *enactmentScheduler) task.Task {
	return func(ctx context.Context) error {
		for {
			var toEnact *apipb.ScheduledControlUpdate
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case toEnact = <-updateCh:
				scheduler.enqueue(ctx, toEnact)
			}
		}
	}
}

type enactmentScheduler struct {
	mu sync.Mutex

	beamQ   worker.Queue[*apipb.ScheduledControlUpdate]
	radioQ  worker.Queue[*apipb.ScheduledControlUpdate]
	flowQ   worker.Queue[*apipb.ScheduledControlUpdate]
	tunnelQ worker.Queue[*apipb.ScheduledControlUpdate]
}

func (s *enactmentScheduler) enqueue(ctx context.Context, update *apipb.ScheduledControlUpdate) {
	switch update.Change.UpdateType.(type) {
	case *apipb.ControlPlaneUpdate_BeamUpdate:
		s.beamQ.Enqueue(update)
	case *apipb.ControlPlaneUpdate_TunnelUpdate:
		s.tunnelQ.Enqueue(update)
	case *apipb.ControlPlaneUpdate_FlowUpdate:
		s.flowQ.Enqueue(update)
	case *apipb.ControlPlaneUpdate_RadioUpdate:
		s.radioQ.Enqueue(update)
	}
}

func (es *enactmentService) applyUpdate(ctx context.Context, upd *apipb.ScheduledControlUpdate) error {
	updateID := *upd.UpdateId
	log := zerolog.Ctx(ctx).With().Str("updateID", updateID).Int64("sequenceNum", sequenceNumberOf(upd)).Logger()
	var newState *apipb.ControlPlaneState
	eb := task.Task(func(ctx context.Context) (err error) {
		newState, err = es.eb.Apply(ctx, upd)
		return err
	}).WithNewSpan("enactment.Backend")

	var enactmentResult *status.Status
	if err := eb(ctx); err != nil {
		log.Error().Err(err).Msg("error handling update")
		enactmentResult = status.Convert(err)
	} else {
		enactmentResult = OK()
	}

	es.mu.Lock()
	if su, ok := es.scheduledUpdates[updateID]; ok {
		su.Status = enactmentResult
		su.Result = newState
	} else {
		log.Error().Msgf("failed to find update in state map")
	}
	es.mu.Unlock()

	es.clock.AfterFunc(attemptedUpdateKeepAliveTimeout, func() {
		es.mu.Lock()
		delete(es.scheduledUpdates, updateID)
		es.mu.Unlock()
	})

	log.Trace().
		Interface("state", newState).
		Interface("status", enactmentResult).
		Msg("got new state from backend")

	if newState != nil {
		if _, err := es.cc.UpdateNodeState(ctx, &afpb.CdpiNodeStateRequest{
			NodeId: &es.nodeID,
			State:  newState,
		}); err != nil {
			return err
		}
	}
	if _, err := es.cc.UpdateRequestStatus(ctx, &afpb.CdpiRequestStatusRequest{
		NodeId: &es.nodeID,
		Status: &apipb.ScheduledControlUpdateStatus{
			UpdateId:  &updateID,
			Timestamp: timeToProto(es.clock.Now()),
			State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
				EnactmentAttempted: enactmentResult.Proto(),
			},
		},
	}); err != nil {
		return err
	}
	return nil
}

func (es *enactmentService) mainLoop(recvCh <-chan *afpb.CdpiResponse, sendCh chan<- *afpb.CdpiRequest, updateCh chan<- *apipb.ScheduledControlUpdate,
) task.Task {
	return func(ctx context.Context) error {
		for {

			var incomingReq *afpb.CdpiResponse
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case incomingReq = <-recvCh:
			}

			changeReq := &afpb.ControlStateChangeRequest{}
			if err := proto.Unmarshal(incomingReq.GetRequestPayload(), changeReq); err != nil {
				return fmt.Errorf("error unmarshalling request payload: %w", err)
			}

			var response *afpb.ControlStateNotification
			switch changeReq.Type.(type) {
			case *afpb.ControlStateChangeRequest_ScheduledUpdate:
				response = es.handleScheduledUpdate(ctx, changeReq, updateCh)
			case *afpb.ControlStateChangeRequest_ScheduledDeletion:
				response = es.handleScheduledDeletion(ctx, changeReq.GetScheduledDeletion())
			case *afpb.ControlStateChangeRequest_ControlPlanePingRequest:
				response = es.handlePingRequest(ctx, changeReq.GetControlPlanePingRequest())
			default:
				return fmt.Errorf("unknown request type received: %v", changeReq.Type)
			}

			payloadBytes, err := proto.Marshal(response)
			if err != nil {
				return fmt.Errorf("error marshalling response: %w", err)
			}

			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case sendCh <- &afpb.CdpiRequest{
				Response: &afpb.CdpiRequest_Response{
					RequestId: incomingReq.RequestId,
					Status:    OK().Proto(),
					Payload:   payloadBytes,
				},
			}:
			}
		}
	}
}

func (es *enactmentService) handleScheduledUpdate(ctx context.Context, req *afpb.ControlStateChangeRequest, updateCh chan<- *apipb.ScheduledControlUpdate) *afpb.ControlStateNotification {
	log := zerolog.Ctx(ctx)

	// TODO: check that this field is set
	upd := req.GetScheduledUpdate()
	tte := upd.TimeToEnact.AsTime()
	now := es.clock.Now()

	log.Debug().
		Time("now", now).
		Time("tte", tte).
		Msg("handling scheduled update")

	if tte.Before(now) && shouldRejectStaleRequest(upd) {
		return es.rejectStaleRequest(ctx, upd)
	} else {
		return es.handleFutureScheduledUpdate(ctx, req, updateCh)
	}
}

func (es *enactmentService) handleFutureScheduledUpdate(
	ctx context.Context, req *afpb.ControlStateChangeRequest, updateCh chan<- *apipb.ScheduledControlUpdate,
) *afpb.ControlStateNotification {
	upd := req.GetScheduledUpdate()
	updateID := *upd.UpdateId
	tte := upd.TimeToEnact.AsTime()
	now := es.clock.Now()
	delay := tte.Sub(now)

	log := zerolog.Ctx(ctx).
		With().
		Time("scheduledAt", now).
		Time("tte", tte).
		Str("updateID", updateID).
		Logger()

	newSeqNum := sequenceNumberOf(upd)
	state := &apipb.ScheduledControlUpdateStatus{
		UpdateId:  &updateID,
		Timestamp: timeToProto(now),
	}

	updateCallback := func() {
		select {
		case updateCh <- upd:
		case <-ctx.Done():
			log.Warn().
				Object("request", loggable.Proto(upd)).
				Msg("discarding pending request because context was cancelled")
		}
	}

	es.mu.Lock()
	switch existingUpd, updateExists := es.scheduledUpdates[updateID]; {
	case updateExists && sequenceNumberOf(existingUpd.Update) >= newSeqNum:
		// Our version of the update supercedes the most recently received one,
		// no need to change anything.
		state.State = &apipb.ScheduledControlUpdateStatus_Scheduled{
			Scheduled: status.New(codes.InvalidArgument, "more recent update already exists").Proto(),
		}

	case updateExists && !existingUpd.timer.Stop():
		// The most recently received version of the update supercedes our
		// version, but its timer has already fired so it's too late to change
		// it.
		state.State = &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
			EnactmentAttempted: status.New(codes.FailedPrecondition, "update already enacted").Proto(),
		}

	default:
		// Either we didn't have a previous version of this update or we
		// managed to stop the timer before it fired. Swap out the stored
		// version with the most recently received one and set a timer for it.
		es.scheduledUpdates[updateID] = &scheduledUpdate{
			timer:  es.clock.AfterFunc(delay, updateCallback),
			Update: upd,
		}
		state.State = &apipb.ScheduledControlUpdateStatus_Scheduled{Scheduled: OK().Proto()}
	}
	es.mu.Unlock()

	return &afpb.ControlStateNotification{Statuses: []*apipb.ScheduledControlUpdateStatus{state}}
}

func (es *enactmentService) handleScheduledDeletion(ctx context.Context, r *apipb.ScheduledControlDeletion) *afpb.ControlStateNotification {
	log := zerolog.Ctx(ctx).With().Strs("updateIDs", r.UpdateIds).Logger()
	log.Debug().Msg("received scheduled deletion")

	notif := &afpb.ControlStateNotification{}

	es.mu.Lock()
	defer es.mu.Unlock()

	for _, updateID := range r.UpdateIds {
		updateStatus := &apipb.ScheduledControlUpdateStatus{
			UpdateId: proto.String(updateID),
			Timestamp: &apipb.DateTime{
				UnixTimeUsec: proto.Int64(es.clock.Now().UnixMicro()),
			},
		}

		su, exists := es.scheduledUpdates[updateID]
		if exists && su.timer.Stop() {
			delete(es.scheduledUpdates, updateID)
			updateStatus.State = &apipb.ScheduledControlUpdateStatus_Unscheduled{
				Unscheduled: OK().Proto(),
			}
		} else {
			// the scheduled update has already fired, so the deletion has failed
			updateStatus.State = &apipb.ScheduledControlUpdateStatus_Unscheduled{
				Unscheduled: status.New(codes.DeadlineExceeded, "scheduled update already attempted").Proto(),
			}
		}
		notif.Statuses = append(notif.Statuses, updateStatus)
	}
	return notif
}

func (es *enactmentService) handlePingRequest(ctx context.Context, r *afpb.ControlPlanePingRequest) *afpb.ControlStateNotification {
	log := zerolog.Ctx(ctx).With().Int64("pingID", *r.Id).Logger()

	log.Debug().Msg("handling ping request")

	return &afpb.ControlStateNotification{
		ControlPlanePingResponse: &afpb.ControlPlanePingResponse{
			Id:            r.Id,
			Status:        OK().Proto(),
			TimeOfReceipt: timestamppb.New(es.clock.Now()),
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

func (es *enactmentService) rejectStaleRequest(ctx context.Context, r *apipb.ScheduledControlUpdate) *afpb.ControlStateNotification {
	zerolog.Ctx(ctx).
		Error().
		Str("updateID", *r.UpdateId).
		Time("tte", r.TimeToEnact.AsTime()).
		Msg("Rejecting update because it is too old to enact")

	return &afpb.ControlStateNotification{
		Statuses: []*apipb.ScheduledControlUpdateStatus{{
			UpdateId:  r.UpdateId,
			Timestamp: timeToProto(es.clock.Now()),
			State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
				EnactmentAttempted: status.New(codes.DeadlineExceeded, "Update received by node but is too old to enact").Proto(),
			},
		}},
	}
}

func sequenceNumberOf(r *apipb.ScheduledControlUpdate) int64 {
	switch chg := r.Change; chg.UpdateType.(type) {
	case *apipb.ControlPlaneUpdate_BeamUpdate:
		return chg.GetBeamUpdate().GetPerInterfaceSequenceNumber()
	case *apipb.ControlPlaneUpdate_FlowUpdate:
		return chg.GetFlowUpdate().GetSequenceNumber()
	case *apipb.ControlPlaneUpdate_RadioUpdate:
		return chg.GetRadioUpdate().GetPerInterfaceSequenceNumber()
	case *apipb.ControlPlaneUpdate_TunnelUpdate:
		return chg.GetTunnelUpdate().GetSequenceNumber()

	default:
		return -1
	}
}
