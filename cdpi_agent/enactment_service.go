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

	afpb "aalyria.com/spacetime/api/cdpi/v1alpha"
	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/enactment"
	"aalyria.com/spacetime/cdpi_agent/internal/channels"
	"aalyria.com/spacetime/cdpi_agent/internal/loggable"
	"aalyria.com/spacetime/cdpi_agent/internal/task"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func OK() *status.Status { return status.New(codes.OK, "") }

// enactmentService represents the control aspect of a node's stream.
type enactmentService struct {
	nc *nodeController

	// protects access to the scheduledUpdates map.
	mu sync.Mutex
	// Maps update IDs => scheduledUpdates.
	scheduledUpdates map[string]scheduledUpdate
	ctrlClient       afpb.NetworkControllerStreamingClient
	// TODO: does init state need to change between invocations?
	initState *afpb.ControlStateNotification
	eb        enactment.Backend
}

type scheduledUpdate struct {
	update *afpb.ControlStateChangeRequest
	timer  safeTimer
}

func (nc *nodeController) newEnactmentService(cc afpb.NetworkControllerStreamingClient, eb enactment.Backend) task.Task {
	return task.Task((&enactmentService{
		nc:               nc,
		mu:               sync.Mutex{},
		scheduledUpdates: map[string]scheduledUpdate{},
		ctrlClient:       cc,
		initState:        nc.initState,
		eb:               eb,
	}).run).WithNewSpan("enactment_service")
}

func (es *enactmentService) run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	cpi, err := es.ctrlClient.ControlPlaneInterface(ctx)
	if err != nil {
		return fmt.Errorf("error invoking the ControlPlaneInterface: %w", err)
	}

	// sendCh contains the messages queued to send over the gRPC stream
	sendCh := make(chan *afpb.ControlStateNotification)
	// recvCh contains the messages received over the gRPC stream
	recvCh := make(chan *afpb.ControlStateChangeRequest)
	// updateCh contains the updates that need to be applied immediately
	updateCh := make(chan *apipb.ScheduledControlUpdate)

	g.Go(channels.NewSource(sendCh).ForwardTo(cpi.Send).
		WithStartingStoppingLogs("sendLoop", zerolog.TraceLevel).
		WithLogField("task", "send").
		WithNewSpan("sendLoop").
		WithCtx(ctx))

	g.Go(channels.NewSink(recvCh).FillFrom(cpi.Recv).
		WithStartingStoppingLogs("recvLoop", zerolog.TraceLevel).
		WithLogField("task", "recv").
		WithNewSpan("recvLoop").
		WithCtx(ctx))

	g.Go(es.enactmentLoop(sendCh, updateCh).
		WithStartingStoppingLogs("enactmentLoop", zerolog.TraceLevel).
		WithLogField("task", "enactment").
		WithNewSpan("enactmentLoop").
		WithCtx(ctx))

	g.Go(es.mainLoop(recvCh, sendCh, updateCh).
		WithStartingStoppingLogs("mainLoop", zerolog.TraceLevel).
		WithLogField("task", "main").
		WithNewSpan("mainLoop").
		WithCtx(ctx))

	sendCh <- es.initState

	err = g.Wait()
	zerolog.Ctx(ctx).Trace().Err(err).Msg("service finished")
	return err
}

// enactmentLoop reads change requests from the `updateCh`, applies them, and
// sends a response via the `sendCh`. All enactments should go through this
// channel to ensure that they're applied serially.
func (es *enactmentService) enactmentLoop(sendCh chan<- *afpb.ControlStateNotification, updateCh <-chan *apipb.ScheduledControlUpdate) task.Task {
	mapFn := func(ctx context.Context, upd *apipb.ScheduledControlUpdate) (*afpb.ControlStateNotification, error) {
		zerolog.Ctx(ctx).Trace().Object("update", loggable.Proto(upd)).Msg("applying update")

		return es.applyUpdate(ctx, upd), nil
	}

	return channels.MapBetween(updateCh, sendCh, mapFn)
}

// mainLoop processes incoming change requests and transforms them into
// state notifications.
func (es *enactmentService) mainLoop(recvCh <-chan *afpb.ControlStateChangeRequest, sendCh chan<- *afpb.ControlStateNotification, updateCh chan<- *apipb.ScheduledControlUpdate) task.Task {
	return func(ctx context.Context) error {
		for {
			var changeReq *afpb.ControlStateChangeRequest
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case changeReq = <-recvCh:
			}

			var response *afpb.ControlStateNotification
			switch changeReq.Type.(type) {
			case *afpb.ControlStateChangeRequest_ScheduledUpdate:
				response = es.handleScheduledUpdate(ctx, changeReq, sendCh, updateCh)
			case *afpb.ControlStateChangeRequest_ScheduledDeletion:
				response = es.handleScheduledDeletion(ctx, changeReq.GetScheduledDeletion())
			case *afpb.ControlStateChangeRequest_ControlPlanePingRequest:
				response = es.handlePingRequest(ctx, changeReq.GetControlPlanePingRequest())
			default:
				return fmt.Errorf("unknown request type received: %v", changeReq.Type)
			}

			if response == nil {
				continue
			}

			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case sendCh <- response:
			}
		}
	}
}

func (es *enactmentService) applyUpdate(ctx context.Context, upd *apipb.ScheduledControlUpdate) *afpb.ControlStateNotification {
	updateID := *upd.UpdateId
	log := zerolog.Ctx(ctx).With().Str("updateID", updateID).Logger()

	es.mu.Lock()
	if su, ok := es.scheduledUpdates[updateID]; ok {
		su.timer.Stop()
		delete(es.scheduledUpdates, updateID)
		log.Debug().Msg("deleting scheduled update")
	}
	es.mu.Unlock()

	var newState *apipb.ControlPlaneState
	if err := task.Wrap(func(ctx context.Context) (*apipb.ControlPlaneState, error) {
		return es.eb(ctx, upd)
	}, &newState).WithNewSpan("enactment.Backend")(ctx); err != nil {
		log.Error().Err(err).Msg("error handling update")

		return &afpb.ControlStateNotification{
			NodeId: proto.String(es.nc.id),
			Statuses: []*apipb.ScheduledControlUpdateStatus{{
				UpdateId: &updateID,
				State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
					EnactmentAttempted: status.Convert(err).Proto(),
				},
			}},
		}
	}

	return &afpb.ControlStateNotification{
		NodeId: proto.String(es.nc.id),
		State:  newState,
		Statuses: []*apipb.ScheduledControlUpdateStatus{{
			UpdateId: &updateID,
			State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
				EnactmentAttempted: OK().Proto(),
			},
		}},
	}
}

func (es *enactmentService) handleScheduledUpdate(ctx context.Context, req *afpb.ControlStateChangeRequest, sendCh chan<- *afpb.ControlStateNotification, updateCh chan<- *apipb.ScheduledControlUpdate) *afpb.ControlStateNotification {
	log := zerolog.Ctx(ctx)

	// TODO: check that this field is set
	upd := req.GetScheduledUpdate()
	tte := upd.TimeToEnact.AsTime()

	log.Debug().
		Time("now", es.nc.clock.Now()).
		Time("tte", tte).
		Msg("handling scheduled update")

	if tte.After(es.nc.clock.Now()) {
		return es.handleFutureScheduledUpdate(ctx, req, sendCh, updateCh)
	} else if tte.Before(es.nc.clock.Now()) && shouldRejectStaleRequest(upd) {
		return es.rejectStaleRequest(ctx, upd)
	} else {
		// for enactments that should be applied immediately we just forward
		// them to the enactmentLoop (to ensure that enactments are always
		// applied serially per-node) and return nil here (because the
		// enactmentLoop will handle acking the request)
		select {
		case <-ctx.Done():
		case updateCh <- upd:
		}
		return nil
	}
}

func (es *enactmentService) handleFutureScheduledUpdate(ctx context.Context, req *afpb.ControlStateChangeRequest, sendCh chan<- *afpb.ControlStateNotification, updateCh chan<- *apipb.ScheduledControlUpdate) *afpb.ControlStateNotification {
	upd := req.GetScheduledUpdate()
	updateID := *upd.UpdateId
	tte := upd.TimeToEnact.AsTime()
	now := es.nc.clock.Now()
	delay := tte.Sub(now)

	log := zerolog.Ctx(ctx).
		With().
		Time("scheduledAt", now).
		Time("tte", tte).
		Str("updateID", updateID).
		Logger()

	es.mu.Lock()
	timer := safeTimer{
		t:        es.nc.clock.NewTimer(delay),
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
		case updateCh <- upd:
		case <-ctx.Done():
			log.Warn().Msg("discarding pending request because context was cancelled")
		}
	}()

	// TODO: check for dupes
	es.scheduledUpdates[updateID] = scheduledUpdate{
		update: req,
		timer:  timer,
	}
	es.mu.Unlock()

	return &afpb.ControlStateNotification{
		NodeId: proto.String(es.nc.id),
		Statuses: []*apipb.ScheduledControlUpdateStatus{
			{
				UpdateId: &updateID,
				State:    &apipb.ScheduledControlUpdateStatus_Scheduled{}},
		},
	}
}

func (es *enactmentService) handleScheduledDeletion(ctx context.Context, r *apipb.ScheduledControlDeletion) *afpb.ControlStateNotification {
	log := zerolog.Ctx(ctx).With().Strs("updateIDs", r.UpdateIds).Logger()
	log.Debug().Msg("received scheduled deletion")

	notif := &afpb.ControlStateNotification{NodeId: proto.String(es.nc.id)}

	es.mu.Lock()
	defer es.mu.Unlock()

	for _, updateID := range r.UpdateIds {
		updateStatus := &apipb.ScheduledControlUpdateStatus{
			UpdateId: proto.String(updateID),
			Timestamp: &apipb.DateTime{
				UnixTimeUsec: proto.Int64(es.nc.clock.Now().UnixMicro()),
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
		NodeId: proto.String(es.nc.id),
		ControlPlanePingResponse: &afpb.ControlPlanePingResponse{
			Id:            r.Id,
			Status:        OK().Proto(),
			TimeOfReceipt: timestamppb.New(es.nc.clock.Now()),
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
		NodeId: proto.String(es.nc.id),
		Statuses: []*apipb.ScheduledControlUpdateStatus{{
			UpdateId: r.UpdateId,
			State: &apipb.ScheduledControlUpdateStatus_EnactmentAttempted{
				EnactmentAttempted: status.New(codes.DeadlineExceeded, "Update received by node but is too old to enact").Proto(),
			},
		}},
	}
}
