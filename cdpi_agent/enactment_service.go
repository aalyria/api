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

	apipb "aalyria.com/spacetime/api/common"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"aalyria.com/spacetime/cdpi_agent/enactment"
	"aalyria.com/spacetime/cdpi_agent/internal/channels"
	"aalyria.com/spacetime/cdpi_agent/internal/loggable"
	"aalyria.com/spacetime/cdpi_agent/internal/task"
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

func (nc *nodeController) newEnactmentService(sc schedpb.SchedulingClient, eb enactment.Backend, manipToken string) *enactmentService {
	return &enactmentService{
		eb:                        eb,
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
		if _, err := es.sc.Reset(ctx, reset); err != nil {
			return err
		}

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
