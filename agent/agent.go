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

// Package agent provides a CDPI agent implementation.
package agent

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"sync"

	"aalyria.com/spacetime/agent/enactment"
	"aalyria.com/spacetime/agent/internal/task"
	"aalyria.com/spacetime/agent/telemetry"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
)

var (
	statsMap   *expvar.Map
	statsMapMu = &sync.Mutex{}
)

func init() {
	statsMap = expvar.NewMap("agent")
}

var (
	errNoClock          = errors.New("no clock provided (see WithClock)")
	errNoNodes          = errors.New("no nodes configured (see WithNode)")
	errNoActiveServices = errors.New("no services configured for node (see WithEnactmentBackend and WithTelemetryBackend)")
)

// AgentOption provides a well-typed and sound mechanism to configure an Agent.
type AgentOption interface{ apply(*Agent) }

// agentOptFunc is a shorthand for creating simple AgentOptions.
type agentOptFunc func(*Agent)

func (fn agentOptFunc) apply(a *Agent) { fn(a) }

// NodeOption provides a well-typed and sound mechanism to configure an
// individual node that an Agent will manage.
type NodeOption interface{ apply(n *node) }

// nodeOptFunc is a shorthand for creating simple NodeOptions.
type nodeOptFunc func(*node)

func (fn nodeOptFunc) apply(n *node) { fn(n) }

// Agent is a CDPI agent that coordinates change requests across multiple
// nodes.
type Agent struct {
	clock clockwork.Clock
	nodes map[string]*node
}

// NewAgent creates a new Agent configured with the provided options.
func NewAgent(opts ...AgentOption) (*Agent, error) {
	a := &Agent{
		nodes: map[string]*node{},
	}

	for _, opt := range opts {
		opt.apply(a)
	}

	return a, a.validate()
}

func (a *Agent) validate() error {
	errs := []error{}
	if a.clock == nil {
		errs = append(errs, errNoClock)
	}
	if len(a.nodes) == 0 {
		errs = append(errs, errNoNodes)
	}
	for _, n := range a.nodes {
		if !n.enactmentsEnabled && !n.telemetryEnabled {
			errs = append(errs, fmt.Errorf("node %q has no services enabled: %w", n.id, errNoActiveServices))
		}
	}
	return errors.Join(errs...)
}

// WithClock configures the Agent to use the provided clock.
func WithClock(clock clockwork.Clock) AgentOption {
	return agentOptFunc(func(a *Agent) {
		a.clock = clock
	})
}

// WithRealClock configures the Agent to use a real clock.
func WithRealClock() AgentOption {
	return WithClock(clockwork.NewRealClock())
}

// WithNode configures a network node for the agent to represent.
func WithNode(id string, opts ...NodeOption) AgentOption {
	n := &node{id: id}
	for _, f := range opts {
		f.apply(n)
	}
	return agentOptFunc(func(a *Agent) {
		a.nodes[id] = n
	})
}

type node struct {
	ed       enactment.Driver
	td       telemetry.Driver
	id       string
	priority uint32

	enactmentEndpoint, telemetryEndpoint string
	enactmentsEnabled, telemetryEnabled  bool
	enactmentDialOpts, telemetryDialOpts []grpc.DialOption
}

// WithEnactmentDriver configures the [enactment.Driver] for the given Node.
func WithEnactmentDriver(endpoint string, d enactment.Driver, dialOpts ...grpc.DialOption) NodeOption {
	return nodeOptFunc(func(n *node) {
		n.ed = d
		n.enactmentEndpoint = endpoint
		n.enactmentDialOpts = dialOpts
		n.enactmentsEnabled = true
	})
}

// WithTelemetryDriver configures the [telemetry.Driver] for the given Node.
func WithTelemetryDriver(endpoint string, d telemetry.Driver, dialOpts ...grpc.DialOption) NodeOption {
	return nodeOptFunc(func(n *node) {
		n.td = d
		n.telemetryEndpoint = endpoint
		n.telemetryDialOpts = dialOpts
		n.telemetryEnabled = true
	})
}

// Run starts the Agent and blocks until a fatal error is encountered or all
// node controllers terminate.
func (a *Agent) Run(ctx context.Context) error {
	agentMap := &expvar.Map{}
	agentMap.Init()

	statKey := fmt.Sprintf("%p", a)

	statsMapMu.Lock()
	statsMap.Set(statKey, agentMap)
	statsMapMu.Unlock()

	defer func() {
		statsMapMu.Lock()
		statsMap.Delete(statKey)
		statsMapMu.Unlock()
	}()

	// We can't use an errgroup here because we explicitly want all the errors,
	// not just the first one.
	//
	// TODO: switch this to sourcegraph's conc library
	errCh := make(chan error)
	if err := a.start(ctx, agentMap, errCh); err != nil {
		return err
	}

	errs := []error{}
	for err := range errCh {
		if errs = append(errs, err); len(errs) == len(a.nodes) {
			break
		}
	}
	return errors.Join(errs...)
}

func (a *Agent) start(ctx context.Context, agentMap *expvar.Map, errCh chan error) error {
	for _, n := range a.nodes {
		ctx, done := context.WithCancel(ctx)

		nc, err := a.newNodeController(n, done)
		if err != nil {
			return fmt.Errorf("node %q: %w", n.id, err)
		}
		agentMap.Set(n.id, expvar.Func(nc.Stats))

		srv := task.Task(nc.run).
			WithStartingStoppingLogs("node controller", zerolog.DebugLevel).
			WithLogField("nodeID", n.id).
			WithSpanAttributes(attribute.String("aalyria.nodeID", n.id)).
			WithNewSpan("node_controller")

		go func() { errCh <- srv(ctx) }()
	}
	return nil
}
