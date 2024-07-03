// Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package agentcli provides a CDPI agent that is configured using a
// protobuf-based manifest.
package agentcli

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	otelsdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	oteltracenoop "go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	channelz "google.golang.org/grpc/channelz/service"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/protobuf/types/known/anypb"

	"aalyria.com/spacetime/auth"
	agent "aalyria.com/spacetime/cdpi_agent"
	"aalyria.com/spacetime/cdpi_agent/enactment"
	enact_extproc "aalyria.com/spacetime/cdpi_agent/enactment/extproc"
	"aalyria.com/spacetime/cdpi_agent/internal/configpb"
	"aalyria.com/spacetime/cdpi_agent/internal/protofmt"
	"aalyria.com/spacetime/cdpi_agent/internal/task"
	"aalyria.com/spacetime/cdpi_agent/telemetry"
	telemetry_extproc "aalyria.com/spacetime/cdpi_agent/telemetry/extproc"
)

// Handles are abstractions over impure, external resources like time and stdio
// streams.
type Handles interface {
	Clock() clockwork.Clock
	Stdout() io.Writer
	Stderr() io.Writer
}

type realHandles struct {
	clock clockwork.Clock
}

func (rh realHandles) Clock() clockwork.Clock { return rh.clock }
func (rh realHandles) Stdout() io.Writer      { return os.Stdout }
func (rh realHandles) Stderr() io.Writer      { return os.Stderr }

// DefaultHandles returns a Handles implementation that delegates to the
// standard implementations for external resources.
func DefaultHandles() Handles { return realHandles{clock: clockwork.NewRealClock()} }

// Provider provides a dynamic way of registering enactment or telemetry
// backend providers. If the provided configuration isn't of the appropriate
// type, the factory function is expected to return nil, [ErrUnknownConfigProto].
type Provider interface {
	EnactmentBackend(_ context.Context, _ Handles, nodeID string, conf *anypb.Any) (enactment.Backend, error)
	TelemetryBackend(_ context.Context, _ Handles, nodeID string, conf *anypb.Any) (telemetry.Backend, error)
}

// ErrUnknownConfigProto is the error a [Provider] should return if the
// provided `anypb.Any` is of an unknown type.
var ErrUnknownConfigProto = errors.New("unknown config proto")

type UnsupportedEnactmentBackend struct{}

func (*UnsupportedEnactmentBackend) EnactmentBackend(_ context.Context, _ Handles, _ string, _ *anypb.Any) (enactment.Backend, error) {
	return nil, ErrUnknownConfigProto
}

type UnsupportedTelemetryBackend struct{}

func (*UnsupportedTelemetryBackend) TelemetryBackend(_ context.Context, _ Handles, _ string, _ *anypb.Any) (telemetry.Backend, error) {
	return nil, ErrUnknownConfigProto
}

type AgentConf struct {
	AppName   string
	Handles   Handles
	Providers []Provider
}

func (ac AgentConf) Run(ctx context.Context, appName string, args []string) (err error) {
	var log zerolog.Logger
	if os.Getenv("TERM") != "" {
		log = zerolog.New(zerolog.ConsoleWriter{Out: ac.Handles.Stderr(), TimeFormat: "2006-01-02 03:04:05PM"})
	} else {
		log = zerolog.New(ac.Handles.Stderr())
	}
	ctx = log.With().Timestamp().Logger().WithContext(ctx)

	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	fs.SetOutput(ac.Handles.Stderr())
	fs.Usage = func() {
		w := fs.Output()
		fmt.Fprintf(w, "Usage: %s [options]\n", appName)
		fmt.Fprint(w, "\nOptions:\n")
		fs.PrintDefaults()
	}

	confPath := fs.String("config", "", "The path to a protobuf representation of the agent's configuration (an AgentParams message).")
	protoFormat := fs.String("format", "text", "The format (one of text, wire, or json) to read the configuration as.")
	dryRunOnly := fs.Bool("dry-run", false, "Just validate the config, don't start the agent. Exits with a non-zero return code if the config is invalid.")
	logLevel := logLevelFlag(zerolog.InfoLevel)
	fs.Var(&logLevel, "log-level", "The log level (one of disabled, warn, panic, info, fatal, error, debug, or trace) to use.")
	if err := fs.Parse(args); err == flag.ErrHelp {
		fs.Usage()
		return nil
	} else if err != nil {
		return err
	}

	params, err := readParams(*confPath, *protoFormat)
	if err != nil {
		return err
	}
	log = (*zerolog.Ctx(ctx)).Level(zerolog.Level(logLevel))
	ctx = log.WithContext(ctx)
	if *dryRunOnly {
		log.Info().Msg("config is valid")
		return nil
	}

	ctx, shutdownTracer, err := injectTracer(ctx, params)
	if err != nil {
		return err
	}
	defer shutdownTracer()

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := runChannelzServer(ctx, params); err != nil {
			return fmt.Errorf("running channelz server: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := runPprofServer(ctx, params); err != nil {
			return fmt.Errorf("running pprof server: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := ac.runAgent(ctx, params); err != nil {
			return fmt.Errorf("running agent: %w", err)
		}
		return nil
	})
	return g.Wait()
}

const defaultMinConnectTimeout = 20 * time.Second

// logLevelFlag is a flag.Value implementation for the zerolog.Level type.
type logLevelFlag zerolog.Level

func (l *logLevelFlag) String() string {
	return fmt.Sprintf("%q", zerolog.Level(*l).String())
}

func (l *logLevelFlag) Set(value string) error {
	level, err := zerolog.ParseLevel(value)
	if err != nil {
		return err
	}

	*l = logLevelFlag(level)
	return nil
}

func readParams(confPath, protoFormat string) (*configpb.AgentParams, error) {
	if confPath == "" {
		return nil, errors.New("no config (--config) provided")
	}
	pf, err := protofmt.FromString(protoFormat)
	if err != nil {
		return nil, fmt.Errorf("bad --format: %w", err)
	}

	confData, err := os.ReadFile(confPath)
	if err != nil {
		return nil, err
	} else if len(confData) == 0 {
		return nil, errors.New("empty config (--config) provided")
	}

	conf := &configpb.AgentParams{}
	if err = pf.Unmarshal(confData, conf); err != nil {
		return nil, fmt.Errorf("unmarshalling config proto: %w", err)
	}

	return conf, err
}

func injectTracer(ctx context.Context, params *configpb.AgentParams) (newCtx context.Context, shutdown func(), err error) {
	endpoint := params.GetObservabilityParams().GetOtelCollectorEndpoint()
	if endpoint == "" {
		return task.InjectTracerProvider(ctx, oteltracenoop.NewTracerProvider()), func() {}, nil
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("cdpi-agent"),
			semconv.ServiceNamespaceKey.String("spacetime"),
			semconv.ServiceVersion("v0.1.0"),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating tracer resources: %w", err)
	}

	exporterOpts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
	if params.GetObservabilityParams().GetUseInsecureConnectionForOtelCollector() {
		exporterOpts = append(exporterOpts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, exporterOpts...)
	if err != nil {
		return nil, nil, err
	}
	tracerProvider := otelsdktrace.NewTracerProvider(
		otelsdktrace.WithResource(res),
		otelsdktrace.WithBatcher(exporter))

	ctx = task.InjectTracerProvider(ctx, tracerProvider)
	shutdown = func() {
		if tpErr := tracerProvider.Shutdown(ctx); tpErr != nil {
			(*zerolog.Ctx(ctx)).Error().Err(tpErr).Msg("error shutting down trace provider")
		}
	}
	return ctx, shutdown, nil
}

func getPrivateKey(ss *configpb.SigningStrategy) (io.Reader, error) {
	switch ss.Type.(type) {
	case *configpb.SigningStrategy_PrivateKeyBytes:
		return bytes.NewBuffer(ss.GetPrivateKeyBytes()), nil

	case *configpb.SigningStrategy_PrivateKeyFile:
		b, err := os.ReadFile(ss.GetPrivateKeyFile())
		if err != nil {
			return nil, err
		}
		return bytes.NewBuffer(b), nil

	default:
		return nil, errors.New("no signing strategy provided")
	}
}

func getProtoFmt(pfpb configpb.NetworkNode_ExternalCommand_ProtoFormat) protofmt.Format {
	switch pfpb.Enum() {
	case configpb.NetworkNode_ExternalCommand_JSON.Enum():
		return protofmt.JSON
	case configpb.NetworkNode_ExternalCommand_TEXT.Enum():
		return protofmt.Text
	case configpb.NetworkNode_ExternalCommand_WIRE.Enum():
		return protofmt.Wire

	case configpb.NetworkNode_ExternalCommand_PROTO_FORMAT_UNSPECIFIED.Enum():
		fallthrough
	default:
		return protofmt.JSON
	}
}

func getDialOpts(ctx context.Context, params *configpb.AgentParams, clock clockwork.Clock) ([]grpc.DialOption, error) {
	tracerProvider, _ := task.ExtractTracerProvider(ctx)

	dialOpts := []grpc.DialOption{
		grpc.WithStreamInterceptor(
			otelgrpc.StreamClientInterceptor(
				otelgrpc.WithTracerProvider(tracerProvider),
				otelgrpc.WithPropagators(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})),
			)),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
	}

	connParams := params.GetConnectionParams()
	backoffParams := connParams.GetBackoffParams()
	grpcBackoff := backoff.DefaultConfig

	if baseDelay := backoffParams.GetBaseDelay().AsDuration(); baseDelay > 0 {
		grpcBackoff.BaseDelay = baseDelay
	}
	if maxDelay := backoffParams.GetBaseDelay().AsDuration(); maxDelay > 0 {
		grpcBackoff.MaxDelay = maxDelay
	}

	grpcConnParams := grpc.ConnectParams{Backoff: grpcBackoff, MinConnectTimeout: defaultMinConnectTimeout}
	if minConnectTimeout := connParams.GetMinConnectTimeout().AsDuration(); minConnectTimeout > 0 {
		grpcConnParams.MinConnectTimeout = minConnectTimeout
	}

	switch connParams.GetTransportSecurity().GetType().(type) {
	case *configpb.ConnectionParams_TransportSecurity_Insecure:
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	case *configpb.ConnectionParams_TransportSecurity_SystemCertPool:
		cp, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("reading system tls cert pool: %w", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(cp, "")))

	default:
		return nil, errors.New("no transport security selection provided")
	}

	dialOpts = append(dialOpts, grpc.WithConnectParams(grpcConnParams))

	switch authStrat := connParams.GetAuthStrategy(); authStrat.Type.(type) {
	case *configpb.AuthStrategy_None:
		// ¯\_(ツ)_/¯

	case *configpb.AuthStrategy_Jwt_:
		jwtSpec := authStrat.GetJwt()
		pkeySrc, err := getPrivateKey(jwtSpec.GetSigningStrategy())
		if err != nil {
			return nil, err
		}

		creds, err := auth.NewCredentials(ctx, auth.Config{
			Clock:        clock,
			Email:        jwtSpec.GetEmail(),
			PrivateKeyID: jwtSpec.GetPrivateKeyId(),
			PrivateKey:   pkeySrc,
		})
		if err != nil {
			return nil, fmt.Errorf("generating authorization JWT: %w", err)
		}
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(creds))

	default:
		return nil, errors.New("no auth_strategy provided")
	}

	return dialOpts, nil
}

func (ac *AgentConf) getNodeOpts(ctx context.Context, node *configpb.NetworkNode, clock clockwork.Clock) (nodeOpts []agent.NodeOption, err error) {
	switch node.GetStateBackend().GetType().(type) {
	case *configpb.NetworkNode_StateBackend_StaticInitialState:
		initState := node.GetStateBackend().GetStaticInitialState()
		if initState == nil {
			return nil, errors.New("missing required initial state")
		}

		nodeOpts = append(nodeOpts, agent.WithInitialState(initState))
	default:
		return nil, errors.New("missing required state backend")
	}

enactmentSwitch:
	switch conf := node.GetEnactmentBackend().GetType().(type) {
	case *configpb.NetworkNode_EnactmentBackend_ExternalCommand:
		enactCmd := node.GetEnactmentBackend().GetExternalCommand()
		nodeOpts = append(nodeOpts, agent.WithEnactmentBackend(enact_extproc.New(func(ctx context.Context) *exec.Cmd {
			// nosemgrep: dangerous-exec-command
			return exec.CommandContext(ctx, enactCmd.GetArgs()[0], enactCmd.GetArgs()[1:]...)
		}, getProtoFmt(enactCmd.GetProtoFormat()))))
	case *configpb.NetworkNode_EnactmentBackend_Netlink:
		eb, err := newNetlinkEnactmentBackend(ctx, clock, node.GetId(), conf.Netlink)
		if err != nil {
			return nil, err
		}
		nodeOpts = append(nodeOpts, agent.WithEnactmentBackend(eb))
	case *configpb.NetworkNode_EnactmentBackend_Dynamic:
		for _, p := range ac.Providers {
			eb, err := p.EnactmentBackend(ctx, ac.Handles, node.GetId(), conf.Dynamic)
			if errors.Is(err, ErrUnknownConfigProto) {
				continue
			} else if err != nil {
				return nil, err
			}

			nodeOpts = append(nodeOpts, agent.WithEnactmentBackend(eb))
			break enactmentSwitch
		}

		return nil, fmt.Errorf("no provider recognized proto of type %s for node %s", conf.Dynamic.GetTypeUrl(), node.GetId())
	}

telemetrySwitch:
	switch conf := node.GetTelemetryBackend().GetType().(type) {
	case *configpb.NetworkNode_TelemetryBackend_ExternalCommand:
		telCmd := node.GetTelemetryBackend().GetExternalCommand()
		nodeOpts = append(nodeOpts, agent.WithTelemetryBackend(telemetry_extproc.New(func(ctx context.Context, nodeID string) *exec.Cmd {
			// nosemgrep: dangerous-exec-command
			return exec.CommandContext(ctx, telCmd.GetArgs()[0], telCmd.GetArgs()[1:]...)
		}, getProtoFmt(telCmd.GetProtoFormat()))))
	case *configpb.NetworkNode_TelemetryBackend_Netlink:
		tb, err := newNetlinkTelemetryBackend(ctx, clock, node.GetId(), conf.Netlink)
		if err != nil {
			return nil, err
		}
		nodeOpts = append(nodeOpts, agent.WithTelemetryBackend(tb))
	case *configpb.NetworkNode_TelemetryBackend_Dynamic:
		for _, p := range ac.Providers {
			tb, err := p.TelemetryBackend(ctx, ac.Handles, node.GetId(), conf.Dynamic)
			if errors.Is(err, ErrUnknownConfigProto) {
				continue
			} else if err != nil {
				return nil, err
			}

			nodeOpts = append(nodeOpts, agent.WithTelemetryBackend(tb))
			break telemetrySwitch
		}
		return nil, fmt.Errorf("no provider recognized proto of type %s for node %s", conf.Dynamic.GetTypeUrl(), node.GetId())
	}

	return nodeOpts, nil
}

func runPprofServer(ctx context.Context, params *configpb.AgentParams) error {
	addr := params.GetObservabilityParams().GetPprofAddress()
	if addr == "" {
		return nil
	}

	lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}

	srv := &http.Server{
		BaseContext: func(_ net.Listener) context.Context { return ctx },
		Handler:     http.DefaultServeMux,
	}
	log := zerolog.Ctx(ctx).With().Str("addr", lis.Addr().String()).Logger()
	log.Info().Msg("starting pprof server")
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return srv.Serve(lis)
	})
	g.Go(func() error {
		<-ctx.Done()
		log.Debug().Msg("stopping pprof server")
		return srv.Shutdown(ctx)
	})
	if err := g.Wait(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runChannelzServer(ctx context.Context, params *configpb.AgentParams) error {
	addr := params.GetObservabilityParams().GetChannelzAddress()
	if addr == "" {
		return nil
	}

	lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}

	srv := grpc.NewServer(
		grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor()),
		grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
	)
	channelz.RegisterChannelzServiceToServer(srv)

	log := zerolog.Ctx(ctx).With().Str("addr", lis.Addr().String()).Logger()
	log.Info().Msg("starting channelz server")

	g, ctx := errgroup.WithContext(ctx)
	g.Go(task.Task(func(ctx context.Context) error {
		return srv.Serve(lis)
	}).WithSpanAttributes(attribute.String("channelz.address", lis.Addr().String())).WithCtx(ctx))
	g.Go(func() error {
		<-ctx.Done()
		log.Debug().Msg("stopping channelz server")
		srv.GracefulStop()
		return nil
	})
	return g.Wait()
}

func (ac *AgentConf) runAgent(ctx context.Context, params *configpb.AgentParams) error {
	clock := clockwork.NewRealClock()

	dialOpts, err := getDialOpts(ctx, params, clock)
	if err != nil {
		return err
	}

	endpoint := params.GetConnectionParams().GetCdpiEndpoint()
	agentOpts := []agent.AgentOption{
		agent.WithClock(clock),
		agent.WithServerEndpoint(endpoint),
		agent.WithDialOpts(dialOpts...),
	}

	for _, node := range params.GetNetworkNodes() {
		nodeOpts, err := ac.getNodeOpts(ctx, node, clock)
		if err != nil {
			return fmt.Errorf("node %s: %w", node.Id, err)
		}
		agentOpts = append(agentOpts, agent.WithNode(node.GetId(), nodeOpts...))
	}

	a, err := agent.NewAgent(agentOpts...)
	if err != nil {
		return err
	}

	zerolog.Ctx(ctx).Info().Str("endpoint", endpoint).Msg("starting agent")
	return a.Run(ctx)
}
