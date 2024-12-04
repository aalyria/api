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
	"cmp"
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
	"os/signal"
	"strings"
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
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/anypb"

	agent "aalyria.com/spacetime/agent"
	"aalyria.com/spacetime/agent/enactment"
	enact_extproc "aalyria.com/spacetime/agent/enactment/extproc"
	"aalyria.com/spacetime/agent/internal/configpb"
	"aalyria.com/spacetime/agent/internal/protofmt"
	"aalyria.com/spacetime/agent/internal/task"
	"aalyria.com/spacetime/agent/telemetry"
	telemetry_extproc "aalyria.com/spacetime/agent/telemetry/extproc"
	"aalyria.com/spacetime/auth"
)

var Version = "0.0.0+development"

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
// driver providers. If the provided configuration isn't of the appropriate
// type, the factory function is expected to return nil, [ErrUnknownConfigProto].
type Provider interface {
	EnactmentDriver(_ context.Context, _ Handles, nodeID string, conf *anypb.Any) (enactment.Driver, error)
	TelemetryDriver(_ context.Context, _ Handles, nodeID string, conf *anypb.Any) (telemetry.Driver, error)
}

// ErrUnknownConfigProto is the error a [Provider] should return if the
// provided `anypb.Any` is of an unknown type.
var ErrUnknownConfigProto = errors.New("unknown config proto")

type UnsupportedEnactmentDriver struct{}

func (*UnsupportedEnactmentDriver) EnactmentDriver(_ context.Context, _ Handles, _ string, _ *anypb.Any) (enactment.Driver, error) {
	return nil, ErrUnknownConfigProto
}

type UnsupportedTelemetryDriver struct{}

func (*UnsupportedTelemetryDriver) TelemetryDriver(_ context.Context, _ Handles, _ string, _ *anypb.Any) (telemetry.Driver, error) {
	return nil, ErrUnknownConfigProto
}

type AgentConf struct {
	AppName   string
	Handles   Handles
	Providers []Provider
}

func (ac AgentConf) Run(ctx context.Context, args []string) (err error) {
	var log zerolog.Logger
	if os.Getenv("TERM") != "" {
		log = zerolog.New(zerolog.ConsoleWriter{Out: ac.Handles.Stderr(), TimeFormat: "2006-01-02 03:04:05PM"})
	} else {
		log = zerolog.New(ac.Handles.Stderr())
	}
	ctx = log.With().Timestamp().Logger().WithContext(ctx)

	fs := flag.NewFlagSet(ac.AppName, flag.ContinueOnError)
	fs.SetOutput(ac.Handles.Stderr())
	fs.Usage = func() {
		w := fs.Output()
		fmt.Fprintf(w, "Usage: %s [options]\n", ac.AppName)
		fmt.Fprint(w, "\nOptions:\n")
		fs.PrintDefaults()
	}

	versionFlag := fs.Bool("version", false, "Print the version")
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

	if *versionFlag {
		w := fs.Output()
		fmt.Fprintf(w, "%s version %s\n", ac.AppName, Version)
		return nil
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

func getDialOpts(ctx context.Context, connParams *configpb.ConnectionParams, clock clockwork.Clock) ([]grpc.DialOption, error) {
	tracerProvider, _ := task.ExtractTracerProvider(ctx)

	dialOpts := []grpc.DialOption{
		grpc.WithStreamInterceptor(
			otelgrpc.StreamClientInterceptor(
				otelgrpc.WithTracerProvider(tracerProvider),
				otelgrpc.WithPropagators(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})),
			)),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                cmp.Or(connParams.GetKeepalivePeriod().AsDuration(), 30*time.Second),
			PermitWithoutStream: true,
		}),
	}

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
		host, _, err := net.SplitHostPort(connParams.GetEndpointUri())
		// If parsing host:port fails, let's use the whole param as host
		// and let downstream libraries fail if the host is actually invalid.
		if err != nil {
			host = connParams.GetEndpointUri()
		}

		creds, err := auth.NewCredentials(ctx, auth.Config{
			Clock:        clock,
			Email:        jwtSpec.GetEmail(),
			PrivateKeyID: jwtSpec.GetPrivateKeyId(),
			PrivateKey:   pkeySrc,
			Host:         host,
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
	// EndpointUri should be in the format `hostname[:port]`, but we want to backward support configs that used the dns:/// prefix.
	const dnsSchema = "dns:///"
	if cp := node.GetEnactmentDriver().GetConnectionParams(); cp != nil {
		cp.EndpointUri = strings.TrimPrefix(cp.EndpointUri, dnsSchema)
	}
	if cp := node.GetTelemetryDriver().GetConnectionParams(); cp != nil {
		cp.EndpointUri = strings.TrimPrefix(cp.EndpointUri, dnsSchema)
	}

enactmentSwitch:
	switch conf := node.GetEnactmentDriver().GetType().(type) {
	case *configpb.NetworkNode_EnactmentDriver_ExternalCommand:
		dialOpts, err := getDialOpts(ctx, node.EnactmentDriver.GetConnectionParams(), clock)
		if err != nil {
			return nil, err
		}

		enactCmd := node.GetEnactmentDriver().GetExternalCommand()
		ed := enact_extproc.New(enactCmd.GetArgs(), getProtoFmt(enactCmd.GetProtoFormat()))

		nodeOpts = append(nodeOpts, agent.WithEnactmentDriver(node.GetEnactmentDriver().GetConnectionParams().EndpointUri, ed, dialOpts...))

	case *configpb.NetworkNode_EnactmentDriver_Netlink:
		dialOpts, err := getDialOpts(ctx, node.EnactmentDriver.GetConnectionParams(), clock)
		if err != nil {
			return nil, err
		}

		ed, err := newNetlinkEnactmentDriver(ctx, clock, node.GetId(), conf.Netlink)
		if err != nil {
			return nil, err
		}
		nodeOpts = append(nodeOpts, agent.WithEnactmentDriver(node.GetEnactmentDriver().GetConnectionParams().EndpointUri, ed, dialOpts...))

	case *configpb.NetworkNode_EnactmentDriver_Dynamic:
		dialOpts, err := getDialOpts(ctx, node.EnactmentDriver.GetConnectionParams(), clock)
		if err != nil {
			return nil, err
		}

		for _, p := range ac.Providers {
			ed, err := p.EnactmentDriver(ctx, ac.Handles, node.GetId(), conf.Dynamic)
			if errors.Is(err, ErrUnknownConfigProto) {
				continue
			} else if err != nil {
				return nil, err
			}

			nodeOpts = append(nodeOpts, agent.WithEnactmentDriver(node.GetEnactmentDriver().GetConnectionParams().EndpointUri, ed, dialOpts...))
			break enactmentSwitch
		}

		return nil, fmt.Errorf("no provider recognized proto of type %s for node %s", conf.Dynamic.GetTypeUrl(), node.GetId())
	}

telemetrySwitch:
	switch conf := node.GetTelemetryDriver().GetType().(type) {
	case *configpb.NetworkNode_TelemetryDriver_ExternalCommand:
		dialOpts, err := getDialOpts(ctx, node.TelemetryDriver.GetConnectionParams(), clock)
		if err != nil {
			return nil, err
		}

		telCmd := node.GetTelemetryDriver().GetExternalCommand()
		td, err := telemetry_extproc.NewDriver(telCmd.GetCommand().GetArgs(), getProtoFmt(telCmd.GetCommand().GetProtoFormat()), telCmd.GetCollectionPeriod().AsDuration())
		if err != nil {
			return nil, err
		}
		nodeOpts = append(nodeOpts, agent.WithTelemetryDriver(node.GetTelemetryDriver().GetConnectionParams().EndpointUri, td, dialOpts...))

	case *configpb.NetworkNode_TelemetryDriver_Netlink:
		dialOpts, err := getDialOpts(ctx, node.TelemetryDriver.GetConnectionParams(), clock)
		if err != nil {
			return nil, err
		}

		td, err := newNetlinkTelemetryDriver(ctx, clock, node.GetId(), conf.Netlink)
		if err != nil {
			return nil, err
		}
		nodeOpts = append(nodeOpts, agent.WithTelemetryDriver(node.GetTelemetryDriver().GetConnectionParams().EndpointUri, td, dialOpts...))

	case *configpb.NetworkNode_TelemetryDriver_Dynamic:
		dialOpts, err := getDialOpts(ctx, node.TelemetryDriver.GetConnectionParams(), clock)
		if err != nil {
			return nil, err
		}

		for _, p := range ac.Providers {
			td, err := p.TelemetryDriver(ctx, ac.Handles, node.GetId(), conf.Dynamic)
			if errors.Is(err, ErrUnknownConfigProto) {
				continue
			} else if err != nil {
				return nil, err
			}

			nodeOpts = append(nodeOpts, agent.WithTelemetryDriver(node.GetTelemetryDriver().GetConnectionParams().EndpointUri, td, dialOpts...))
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

	agentOpts := []agent.AgentOption{agent.WithClock(clock)}

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

	zerolog.Ctx(ctx).Info().Msg("starting agent")
	return a.Run(ctx)
}
