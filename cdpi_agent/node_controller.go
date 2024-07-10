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
	"time"

	afpb "aalyria.com/spacetime/api/cdpi/v1alpha"
	apipb "aalyria.com/spacetime/api/common"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"aalyria.com/spacetime/cdpi_agent/internal/task"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// nodeController is the logical owner of a node and its various services
// (telemetry, enactments, etc.).
type nodeController struct {
	// The node ID this controller is responsible for.
	id string
	// done is called when the controller should stop.
	done      func()
	clock     clockwork.Clock
	initState *apipb.ControlPlaneState
	services  []task.Task

	enactmentStats func() interface{}
	telemetryStats func() interface{}

	closers []func() error

	newToken func() string
}

func (a *Agent) newNodeController(node *node, done func()) (*nodeController, error) {
	nc := &nodeController{
		id:             node.id,
		done:           done,
		clock:          a.clock,
		enactmentStats: func() interface{} { return nil },
		telemetryStats: func() interface{} { return nil },
		newToken:       uuid.NewString,
	}

	rc := task.RetryConfig{
		BackoffDuration: 5 * time.Second,
		ErrIsFatal: func(err error) bool {
			switch c := status.Code(err); {
			case c == codes.Unauthenticated, c == codes.Canceled, c == codes.Unimplemented, errors.Is(err, context.Canceled):
				return true
			default:
				return false
			}
		},
		Clock: nc.clock,
	}

	nc.services = []task.Task{}
	if node.telemetryEnabled {
		telemetryConn, err := grpc.NewClient(node.telemetryEndpoint, node.telemetryDialOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed connecting to telemetry endpoint: %w", err)
		}
		nc.closers = append(nc.closers, telemetryConn.Close)

		telemetryClient := afpb.NewNetworkTelemetryStreamingClient(telemetryConn)

		ts := nc.newTelemetryService(telemetryClient, node.td)

		nc.services = append(nc.services, task.Task(ts.run).
			WithNewSpan("telemetry_service").
			WithLogField("service", "telemetry").
			WithRetries(rc).
			WithPanicCatcher())

		nc.telemetryStats = ts.Stats
	}

	if node.enactmentsEnabled {
		enactmentConn, err := grpc.NewClient(node.enactmentEndpoint, node.enactmentDialOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed connecting to enactment endpoint: %w", err)
		}
		nc.closers = append(nc.closers, enactmentConn.Close)

		schedClient := schedpb.NewSchedulingClient(enactmentConn)
		es := nc.newEnactmentService(schedClient, node.ed, nc.newToken())

		nc.services = append(nc.services, task.Task(es.run).
			WithNewSpan("enactment_service").
			WithLogField("service", "enactment").
			WithRetries(rc).
			WithPanicCatcher())

		nc.enactmentStats = es.Stats
	}

	return nc, nil
}

func (nc *nodeController) run(ctx context.Context) (resErr error) {
	defer func() {
		nc.done()

		errs := []error{resErr}
		for _, c := range nc.closers {
			errs = append(errs, c())
		}
		resErr = errors.Join(errs...)
	}()

	return task.Group(nc.services...).WithPanicCatcher()(ctx)
}

type nodeControllerStats struct {
	Enactment interface{}
	Telemetry interface{}
}

func (nc *nodeController) Stats() interface{} {
	return nodeControllerStats{
		Enactment: nc.enactmentStats(),
		Telemetry: nc.telemetryStats(),
	}
}
