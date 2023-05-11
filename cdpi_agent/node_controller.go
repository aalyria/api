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
	"time"

	afpb "aalyria.com/spacetime/api/cdpi/v1alpha"
	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/internal/task"

	"github.com/jonboulle/clockwork"
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
	// The channel priority of this controller's stream.
	priority uint32
}

func (a *Agent) newNodeController(node *node, done func(), cdpiClient afpb.CdpiClient, telemetryClient afpb.NetworkTelemetryStreamingClient) *nodeController {
	nc := &nodeController{
		id:        node.id,
		priority:  node.priority,
		done:      done,
		clock:     a.clock,
		initState: node.initState,
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
		nc.services = append(nc.services, nc.newTelemetryService(telemetryClient, node.tb).
			WithLogField("service", "telemetry").
			WithRetries(rc).
			WithPanicCatcher())
	}
	if node.enactmentsEnabled {
		nc.services = append(nc.services, nc.newEnactmentService(cdpiClient, node.eb).
			WithLogField("service", "enactment").
			WithRetries(rc).
			WithPanicCatcher())
	}

	return nc
}

func (nc *nodeController) run(ctx context.Context) error {
	defer nc.done()
	return task.Group(nc.services...).WithPanicCatcher()(ctx)
}
