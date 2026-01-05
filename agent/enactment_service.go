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
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"aalyria.com/spacetime/agent/enactment"
	"aalyria.com/spacetime/agent/internal/channels"
	"aalyria.com/spacetime/agent/internal/loggable"
	"aalyria.com/spacetime/agent/internal/task"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
)

// We keep already attempted enactments in memory for this long to help resolve
// issues where the SBI server might try and update a request after it's been
// applied.
const attemptedUpdateKeepAliveTimeout = 1 * time.Minute

func OK() *status.Status { return status.New(codes.OK, "") }

type enactmentService struct {
	sc     schedpb.SchedulingClient
	ed     enactment.Driver
	clock  clockwork.Clock
	nodeID string

	scheduleManipulationToken string
	schedMgr                  *scheduleManager
	reqsFromController        chan *schedpb.ReceiveRequestsMessageFromController
	rspsToController          chan *schedpb.ReceiveRequestsMessageToController
	dispatchTimer             chan string
	enactmentResults          chan *enactmentResult
}

func (nc *nodeController) newEnactmentService(sc schedpb.SchedulingClient, ed enactment.Driver, manipToken string) *enactmentService {
	return &enactmentService{
		ed:                        ed,
		sc:                        sc,
		clock:                     nc.clock,
		nodeID:                    nc.id,
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
		Driver                    any
		Schedule                  *scheduleManager
		ScheduleManipulationToken string
	}{
		Driver:                    es.ed.Stats(),
		Schedule:                  es.schedMgr,
		ScheduleManipulationToken: es.scheduleManipulationToken,
	}
}

type enactmentResult struct {
	id     string
	err    error
	tStamp time.Time
}

type scheduleManager struct {
	LastFinalize time.Time
	Entries      map[string]*scheduleEvent
}

func newScheduleManager() *scheduleManager {
	return &scheduleManager{
		LastFinalize: time.Time{},
		Entries:      map[string]*scheduleEvent{},
	}
}

type scheduleEvent struct {
	DeletePending bool
	timer         clockwork.Timer
	StartTime     time.Time
	EndTime       time.Time
	Error         error
	Req           *schedpb.ReceiveRequestsMessageFromController
}

func (sm *scheduleManager) createEntry(timer clockwork.Timer, req *schedpb.ReceiveRequestsMessageFromController) {
	sm.Entries[req.GetCreateEntry().Id] = &scheduleEvent{
		timer: timer,
		Req:   req,
	}
}

func (sm *scheduleManager) deleteEntry(id string) {
	// DeleteEntry has been called; clean up on aisle five.
	entry, ok := sm.Entries[id]
	if !ok {
		// It's ok to delete entries that don't exist.
		return
	}

	if entry.EndTime.Before(entry.StartTime) {
		// The enactment backend has been called but has not yet completed.
		entry.DeletePending = true
		return
	}

	entry.timer.Stop()
	delete(sm.Entries, id)
}

func (sm *scheduleManager) finalizeEntries(upTo time.Time) {
	// TODO:
	//   for each entry:
	//     if entry.time.Before(upTo) and enacted [and return code captured]:
	//       delete entry
	sm.LastFinalize = upTo
}

func (sm *scheduleManager) recordResult(result *enactmentResult) {
	entry, ok := sm.Entries[result.id]
	if !ok {
		// Entry somehow deleted after being kicked off.
		// TODO: log (will want ctx here)
		return
	}

	entry.Error = result.err
	entry.EndTime = result.tStamp

	if entry.DeletePending {
		// A DeleteEntryRequest arrived after the request had
		// already been Dispatch()-ed.
		sm.deleteEntry(result.id)
		return
	}
	// TODO: somehow relay info to controler.
}

func (es *enactmentService) run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	if err := es.ed.Init(ctx); err != nil {
		return fmt.Errorf("%T.Init() failed: %w", es.ed, err)
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
					zerolog.Ctx(ctx).Info().
						Uint64("seqno.this", seqno).
						Uint64("seqno.nextExpected", nextSeqno).
						Msg("seqno greater than next expected; delaying scheduling")
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
			entry, ok := es.schedMgr.Entries[id]
			if !ok {
				// Likely a DeleteEntry() racing with the Timer firing.
				continue
			}
			if entry.StartTime.After(entry.EndTime) {
				zerolog.Ctx(ctx).Warn().Object("req", loggable.Proto(entry.Req)).Msg("Dispatch already in progress")
				continue
			}
			if entry.EndTime.After(entry.StartTime) {
				zerolog.Ctx(ctx).Warn().Object("req", loggable.Proto(entry.Req)).Msg("Dispatch already completed")
				continue
			}
			entry.StartTime = es.clock.Now()
			go func() {
				result := &enactmentResult{
					id: id,
				}
				result.err = es.ed.Dispatch(ctx, entry.Req.GetCreateEntry())
				result.tStamp = es.clock.Now()

				select {
				case <-ctx.Done():
					return
				case es.enactmentResults <- result:
				}
			}()
		// [3] Note the return code (and other results) from a backend call.
		case result := <-es.enactmentResults:
			zerolog.Ctx(ctx).Debug().
				Str("result.id", result.id).
				AnErr("result.err", result.err).
				Time("result.tStamp", result.tStamp).
				Msg("recv'd Dispatch result")
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
		zerolog.Ctx(ctx).Debug().Object("response", loggable.Proto(resp)).Msg("sending response")
		return nil
	}
}

func (es *enactmentService) handleSchedulingRequest(ctx context.Context, req *schedpb.ReceiveRequestsMessageFromController) *status.Status {
	switch req.Request.(type) {
	case *schedpb.ReceiveRequestsMessageFromController_CreateEntry:
		id := req.GetCreateEntry().Id
		if _, ok := es.schedMgr.Entries[id]; ok {
			// Ignore repeated scheduling of the same entry ID.
			zerolog.Ctx(ctx).Debug().Str("entry ID", id).Msg("ignoring scheduling attempt of duplicate entry")
			return OK()
		}
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
		if !upTo.After(es.schedMgr.LastFinalize) {
			return status.Newf(
				codes.FailedPrecondition,
				"received finalize up_to %d <= latest finalize up_to (%d)",
				upTo.UnixNano(),
				es.schedMgr.LastFinalize.UnixNano())
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
