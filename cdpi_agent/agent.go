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

	afpb "aalyria.com/spacetime/api/cdpi/v1alpha"
	apipb "aalyria.com/spacetime/api/common"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"aalyria.com/spacetime/cdpi_agent/enactment"
	"aalyria.com/spacetime/cdpi_agent/internal/task"
	"aalyria.com/spacetime/cdpi_agent/telemetry"

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
	errNoEndpoint       = errors.New("no server endpoint provied (see WithServerEndpoint)")
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
	clock    clockwork.Clock
	nodes    map[string]*node
	endpoint string
	dialOpts []grpc.DialOption
}

// NewAgent creates a new Agent configured with the provided options.
func NewAgent(opts ...AgentOption) (*Agent, error) {
	a := &Agent{
		nodes:    map[string]*node{},
		dialOpts: []grpc.DialOption{grpc.WithBlock()},
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
	if a.endpoint == "" {
		errs = append(errs, errNoEndpoint)
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

// WithServerEndpoint configures the Agent to connect to the provided endpoint.
func WithServerEndpoint(endpoint string) AgentOption {
	return agentOptFunc(func(a *Agent) {
		a.endpoint = endpoint
	})
}

// WithDialOpts configures the Agent to use the provided DialOptions when
// connecting to the CDPI endpoint.
//
// NOTE: The CDPI agent always uses the `grpc.WithBlock` option to ensure
// initial connection errors are caught immediately, whereas logical errors are
// often more tightly scoped to individual RPCs.
func WithDialOpts(dialOpts ...grpc.DialOption) AgentOption {
	return agentOptFunc(func(a *Agent) {
		a.dialOpts = append(a.dialOpts, dialOpts...)
	})
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
	initState         *apipb.ControlPlaneState
	eb                enactment.Backend
	tb                telemetry.Backend
	id                string
	priority          uint32
	enactmentsEnabled bool
	telemetryEnabled  bool
}

func WithChannelPriority(priority uint32) NodeOption {
	return nodeOptFunc(func(n *node) { n.priority = priority })
}

// WithEnactmentBackend configures the EnactmentBackend for the given Node.
func WithEnactmentBackend(eb enactment.Backend) NodeOption {
	return nodeOptFunc(func(n *node) {
		n.eb = eb
		n.enactmentsEnabled = true
	})
}

// WithTelemetryBackend configures the telemetry.Backend for the given Node.
func WithTelemetryBackend(tb telemetry.Backend) NodeOption {
	return nodeOptFunc(func(n *node) {
		n.tb = tb
		n.telemetryEnabled = true
	})
}

// WithInitialState configures the initial state of the Node.
func WithInitialState(initState *apipb.ControlPlaneState) NodeOption {
	return nodeOptFunc(func(n *node) {
		n.initState = initState
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
	log := zerolog.Ctx(ctx)

	log.Trace().Str("endpoint", a.endpoint).Msg("contacting the CDPI endpoint")
	conn, err := grpc.NewClient(a.endpoint, a.dialOpts...)
	if err != nil {
		return fmt.Errorf("agent: failed connecting to CDPI backend: %w", err)
	}

	cdpiClient := afpb.NewCdpiClient(conn)
	schedClient := schedpb.NewSchedulingClient(conn)
	telemetryClient := afpb.NewNetworkTelemetryStreamingClient(conn)

	for _, n := range a.nodes {
		ctx, done := context.WithCancel(ctx)

		nc := a.newNodeController(n, done, cdpiClient, schedClient, telemetryClient)
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
