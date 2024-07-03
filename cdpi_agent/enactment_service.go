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
	"cmp"
	"context"
	"fmt"
	"slices"
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
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
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
	sc               schedpb.SchedulingClient
	eb               enactment.Backend
	clock            clockwork.Clock
	initState        *apipb.ControlPlaneState
	nodeID           string
	priority         uint32

	scheduleManipulationToken string
	schedMgr                  *scheduleManager
	reqsFromController        chan *schedpb.ReceiveRequestsMessageFromController
	rspsToController          chan *schedpb.ReceiveRequestsMessageToController
	dispatchTimer             chan string
	enactmentResults          chan *enactmentResult
}

type scheduledUpdate struct {
	timer  clockwork.Timer
	Update *apipb.ScheduledControlUpdate
	Status *status.Status
	Result *apipb.ControlPlaneState
}

func (nc *nodeController) newEnactmentService(cc afpb.CdpiClient, sc schedpb.SchedulingClient, eb enactment.Backend, manipToken string) *enactmentService {
	return &enactmentService{
		eb:                        eb,
		cc:                        cc,
		sc:                        sc,
		mu:                        sync.Mutex{},
		scheduledUpdates:          map[string]*scheduledUpdate{},
		clock:                     nc.clock,
		nodeID:                    nc.id,
		initState:                 nc.initState,
		priority:                  nc.priority,
		scheduleManipulationToken: manipToken,
		schedMgr:                  newScheduleManager(),
		reqsFromController:        make(chan *schedpb.ReceiveRequestsMessageFromController),
		rspsToController:          make(chan *schedpb.ReceiveRequestsMessageToController),
		dispatchTimer:             make(chan string),
		enactmentResults:          make(chan *enactmentResult),
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

type enactmentResult struct {
	id     string
	err    error
	tStamp time.Time
}

type scheduleManager struct {
	lastFinalize time.Time
	entries      map[string]*scheduleEvent
}

func newScheduleManager() *scheduleManager {
	return &scheduleManager{
		lastFinalize: time.Time{},
		entries:      map[string]*scheduleEvent{},
	}
}

type scheduleEvent struct {
	deletePending bool
	timer         clockwork.Timer
	startTime     time.Time
	endTime       time.Time
	err           error
	req           *schedpb.ReceiveRequestsMessageFromController
}

func (sm *scheduleManager) createEntry(timer clockwork.Timer, req *schedpb.ReceiveRequestsMessageFromController) {
	sm.entries[req.GetCreateEntry().Id] = &scheduleEvent{
		timer: timer,
		req:   req,
	}
}

func (sm *scheduleManager) deleteEntry(id string) {
	// DeleteEntry has been called; clean up on aisle five.
	entry, ok := sm.entries[id]
	if !ok {
		// It's ok to delete entries that don't exist.
		return
	}

	if entry.endTime.Before(entry.startTime) {
		// The enactment backend has been called but has not yet completed.
		entry.deletePending = true
		return
	}

	entry.timer.Stop()
	delete(sm.entries, id)
}

func (sm *scheduleManager) finalizeEntries(upTo time.Time) {
	// TODO:
	//   for each entry:
	//     if entry.time.Before(upTo) and enacted [and return code captured]:
	//       delete entry
	sm.lastFinalize = upTo
}

func (sm *scheduleManager) recordResult(result *enactmentResult) {
	entry, ok := sm.entries[result.id]
	if !ok {
		// Entry somehow deleted after being kicked off.
		// TODO: log (will want ctx here)
		return
	}

	entry.err = result.err
	entry.endTime = result.tStamp
	// TODO: log (will want ctx here)

	if entry.deletePending {
		// A DeleteEntryRequest arrived after the request had
		// already been Dispatch()-ed.
		sm.deleteEntry(result.id)
		return
	}
	// TODO: somehow relay info to controler.
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

	schedulingRequestStream, err := es.sc.ReceiveRequests(ctx)
	if err != nil {
		return fmt.Errorf("error invoking the Scheduling ReceiveRequests interface: %w", err)
	}

	g.Go(channels.NewSource(es.rspsToController).ForwardTo(schedulingRequestStream.Send).
		WithStartingStoppingLogs("toController", zerolog.TraceLevel).
		WithLogField("task", "send").
		WithNewSpan("sendLoop").
		WithPanicCatcher().
		WithCtx(ctx))

	g.Go(channels.NewSink(es.reqsFromController).FillFrom(schedulingRequestStream.Recv).
		WithStartingStoppingLogs("fromController", zerolog.TraceLevel).
		WithLogField("task", "recv").
		WithPanicCatcher().
		WithCtx(ctx))

	g.Go(task.Task(es.mainScheduleLoop).
		WithStartingStoppingLogs("mainScheduleLoop", zerolog.TraceLevel).
		WithLogField("task", "main").
		WithNewSpan("mainScheduleLoop").
		WithPanicCatcher().
		WithCtx(ctx))

	// Send an immediate Reset to convey a new schedule_manipulation_token, followed
	// by a Hello.
	g.Go(func() error {
		reset := es.schedReset()
		zerolog.Ctx(ctx).Debug().Object("reset", loggable.Proto(reset)).Msg("sending reset")
		es.sc.Reset(ctx, reset)

		hello := es.schedHello()
		zerolog.Ctx(ctx).Debug().Object("hello", loggable.Proto(hello)).Msg("sending hello")
		select {
		case es.rspsToController <- hello:
			break
		case <-ctx.Done():
			return context.Cause(ctx)
		}

		return nil
	})

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

func (es *enactmentService) schedHello() *schedpb.ReceiveRequestsMessageToController {
	return &schedpb.ReceiveRequestsMessageToController{
		Hello: &schedpb.ReceiveRequestsMessageToController_Hello{
			AgentId: es.nodeID,
		},
	}
}

func (es *enactmentService) schedReset() *schedpb.ResetRequest {
	return &schedpb.ResetRequest{
		AgentId:                   es.nodeID,
		ScheduleManipulationToken: es.scheduleManipulationToken,
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

func (es *enactmentService) mainScheduleLoop(ctx context.Context) error {
	var nextSeqno uint64 = 1
	pendingRequests := []*schedpb.ReceiveRequestsMessageFromController{}

	for {
		select {
		// [0] Handle context shutdown.
		case <-ctx.Done():
			return context.Cause(ctx)
		// [1] Process requests from the controller.
		case req := <-es.reqsFromController:
			// Check for correct schedule manipulation token, if present.
			token, ok := getScheduleManipulationToken(req)
			if ok && token != es.scheduleManipulationToken {
				err := es.sendResponse(ctx, req.RequestId, status.Newf(
					codes.FailedPrecondition,
					"received schedule manipulation token %q is not current (%s)", token, es.scheduleManipulationToken))
				if err != nil {
					return err
				}
				// Carry on; we may sync up at some point in the future.
				continue
			}

			_, ok = getSeqno(req)
			if !ok {
				// Presently, all requests supported here have a sequence number.
				err := es.sendResponse(ctx, req.RequestId, status.Newf(
					codes.Unimplemented,
					"unable to handle request %q which has no seqno", req.RequestId))
				if err != nil {
					return err
				}
				// Carry on awaiting a message we can understand.
				continue
			}
			// Sort requests by seqno.
			pendingRequests = append(pendingRequests, req)
			slices.SortFunc(pendingRequests, func(l, r *schedpb.ReceiveRequestsMessageFromController) int {
				return cmp.Compare(mustSeqno(l), mustSeqno(r))
			})

			// Process any old requests followed by all next expected requests.
			// Any requests sequenced after the next expected are blocked until
			// missing requests arrive.
			for len(pendingRequests) > 0 {
				req = pendingRequests[0]
				seqno := mustSeqno(req)
				if seqno > nextSeqno {
					break
				}
				status := es.handleSchedulingRequest(ctx, req)
				err := es.sendResponse(ctx, req.RequestId, status)
				if err != nil {
					return err
				}
				pendingRequests = pendingRequests[1:]
				if seqno == nextSeqno {
					nextSeqno++
				}
			}
		// [2] Handle timer firing to launch a call to the backend.
		case id := <-es.dispatchTimer:
			entry, ok := es.schedMgr.entries[id]
			if !ok {
				// Likely a DeleteEntry() racing with the Timer firing.
				continue
			}
			entry.startTime = es.clock.Now()
			go func() {
				result := &enactmentResult{
					id: id,
				}
				result.err = es.eb.Dispatch(ctx, entry.req.GetCreateEntry())
				result.tStamp = es.clock.Now()

				select {
				case <-ctx.Done():
					return
				case es.enactmentResults <- result:
				}
			}()
		// [3] Note the return code (and other results) from a backend call.
		case result := <-es.enactmentResults:
			es.schedMgr.recordResult(result)
		}
	}
}

func (es *enactmentService) sendResponse(ctx context.Context, id int64, status *status.Status) error {
	resp := &schedpb.ReceiveRequestsMessageToController{
		Response: &schedpb.ReceiveRequestsMessageToController_Response{
			RequestId: id,
			Status:    status.Proto(),
		},
	}

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case es.rspsToController <- resp:
		return nil
	}
}

func (es *enactmentService) handleSchedulingRequest(ctx context.Context, req *schedpb.ReceiveRequestsMessageFromController) *status.Status {
	switch req.Request.(type) {
	case *schedpb.ReceiveRequestsMessageFromController_CreateEntry:
		id := req.GetCreateEntry().Id
		delay := req.GetCreateEntry().Time.AsTime().Sub(es.clock.Now())
		timer := es.clock.AfterFunc(delay, func() {
			select {
			case <-ctx.Done():
				return
			case es.dispatchTimer <- id:
			}
		})
		es.schedMgr.createEntry(timer, req)
		return OK()

	case *schedpb.ReceiveRequestsMessageFromController_DeleteEntry:
		es.schedMgr.deleteEntry(req.GetDeleteEntry().Id)
		return OK()

	case *schedpb.ReceiveRequestsMessageFromController_Finalize:
		upTo := req.GetFinalize().UpTo.AsTime()
		if !upTo.After(es.schedMgr.lastFinalize) {
			return status.Newf(
				codes.FailedPrecondition,
				"received finalize up_to %d <= latest finalize up_to (%d)",
				upTo.UnixNano(),
				es.schedMgr.lastFinalize.UnixNano())
		}

		es.schedMgr.finalizeEntries(upTo)
		return OK()

	default:
		return status.New(codes.Unimplemented, "unknown ReceiveRequestsFromController request type")
	}
}

func getScheduleManipulationToken(req *schedpb.ReceiveRequestsMessageFromController) (string, bool) {
	switch req.Request.(type) {
	case *schedpb.ReceiveRequestsMessageFromController_CreateEntry:
		return req.GetCreateEntry().ScheduleManipulationToken, true
	case *schedpb.ReceiveRequestsMessageFromController_DeleteEntry:
		return req.GetDeleteEntry().ScheduleManipulationToken, true
	case *schedpb.ReceiveRequestsMessageFromController_Finalize:
		return req.GetFinalize().ScheduleManipulationToken, true
	default:
		return "", false
	}
}

func getSeqno(req *schedpb.ReceiveRequestsMessageFromController) (uint64, bool) {
	switch req.Request.(type) {
	case *schedpb.ReceiveRequestsMessageFromController_CreateEntry:
		return req.GetCreateEntry().Seqno, true
	case *schedpb.ReceiveRequestsMessageFromController_DeleteEntry:
		return req.GetDeleteEntry().Seqno, true
	case *schedpb.ReceiveRequestsMessageFromController_Finalize:
		return req.GetFinalize().Seqno, true
	default:
		return 0, false
	}
}

func mustSeqno(req *schedpb.ReceiveRequestsMessageFromController) uint64 {
	seqno, ok := getSeqno(req)
	if !ok {
		panic(fmt.Errorf("failed to get seqno when required; id %d", req.RequestId))
	}
	return seqno
}

func getId(req *schedpb.ReceiveRequestsMessageFromController) (string, bool) {
	switch req.Request.(type) {
	case *schedpb.ReceiveRequestsMessageFromController_CreateEntry:
		return req.GetCreateEntry().Id, true
	case *schedpb.ReceiveRequestsMessageFromController_DeleteEntry:
		return req.GetDeleteEntry().Id, true
	default:
		return "", false
	}
}
