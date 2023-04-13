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

package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os/exec"
	"strings"
	"time"

	afpb "aalyria.com/spacetime/api/cdpi/v1alpha"
	"aalyria.com/spacetime/cdpi_agent"
	enact_extproc "aalyria.com/spacetime/cdpi_agent/enactment/extproc"
	"aalyria.com/spacetime/cdpi_agent/internal/task"
	telem_extproc "aalyria.com/spacetime/cdpi_agent/telemetry/extproc"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	otelsdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	channelz "google.golang.org/grpc/channelz/service"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

// multiStringFlag is a flag.Value implementation that can be set multiple
// times. We use this to handle the need to register multiple nodes with a
// single agent.
type multiStringFlag []string

func (m *multiStringFlag) String() string {
	return strings.Join([]string(*m), " ")
}

func (m *multiStringFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

// logLevelFlag is a flag.Value implementation for the zerolog.Level type.
type logLevelFlag zerolog.Level

func (l *logLevelFlag) String() string {
	return zerolog.Level(*l).String()
}

func (l *logLevelFlag) Set(value string) error {
	level, err := zerolog.ParseLevel(value)
	if err != nil {
		return err
	}

	*l = logLevelFlag(level)
	return nil
}

// jwtBearerAuth is a credentials.PerRPCCredentials implementation that adds
// the `jwt` field to the provided `header` field of outgoing RPCs.
type jwtBearerAuth struct {
	header, jwt string
}

func (j jwtBearerAuth) RequireTransportSecurity() bool { return true }
func (j jwtBearerAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{j.header: "Bearer " + j.jwt}, nil
}

// exchangeProxyJWT uses a given `jwt` and calls the Google OAuth2 endpoint in
// order to obtain an OIDC token (itself a JWT) that can be used to access
// proxied resources.
func exchangeProxyJWT(ctx context.Context, jwt string) (string, error) {
	params := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {jwt},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://www.googleapis.com/oauth2/v4/token", strings.NewReader(params.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}).Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	data := map[string]interface{}{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("error unmarshalling response body: %w", err)
	}

	if resp.StatusCode != 200 {
		desc, ok := data["error_description"]
		if !ok {
			desc = "(no error_description provided)"
		}
		errName, ok := data["error"]
		if !ok {
			errName = "unknown error"
		}
		return "", fmt.Errorf("%s: %s", errName, desc)
	}

	oidcJWT, ok := data["id_token"].(string)
	if !ok {
		return "", fmt.Errorf("bad id_token returned: %q", oidcJWT)
	}
	return oidcJWT, nil
}

// cliOpts is a struct that holds the CLI configuration for this program.
type cliOpts struct {
	authJWT, proxyAuthJWT string

	insecure          bool
	plaintext         bool
	serverAddr        string
	backoffBaseDelay  time.Duration
	backoffMultiplier float64
	backoffJitter     float64
	backoffMaxDelay   time.Duration
	minConnectTimeout time.Duration
	logLevel          logLevelFlag
	nodeIDs           multiStringFlag
	channelzAddr      string
	otelExporterAddr  string

	telemetryCmd multiStringFlag
	enactmentCmd []string
}

func parseOpts(app_name string, args ...string) (*cliOpts, error) {
	fs := flag.NewFlagSet(app_name, flag.ContinueOnError)
	fs.Usage = func() {
		w := fs.Output()
		fmt.Fprintf(w, "Usage: %s [options] -- [command to run]\n", app_name)
		fmt.Fprint(w, "\nOptions:\n")
		fs.PrintDefaults()
	}

	opts := &cliOpts{logLevel: logLevelFlag(zerolog.InfoLevel)}
	fs.StringVar(&opts.authJWT, "authorization-jwt", "", "The signed JWT token to use for authentication with the CDPI service.")
	fs.StringVar(&opts.proxyAuthJWT, "proxy-authorization-jwt", "", "The signed JWT token to use for authentication with the secure proxy.")
	fs.BoolVar(&opts.plaintext, "plaintext", false, "Use plain-text HTTP/2 when connecting to the CDPI endpoint (no TLS).")
	fs.BoolVar(&opts.insecure, "insecure", false, "Don't use JWTs for authentication. Incompatible with the --authorization-jwt and --proxy-authorization-jwt flags.")

	fs.StringVar(&opts.serverAddr, "cdpi-endpoint", "", "Address of the CDPI backend.")
	fs.DurationVar(&opts.backoffBaseDelay, "backoff-base-delay", 1.0*time.Second, "The amount of time to backoff after the first connection failure.")
	fs.Float64Var(&opts.backoffMultiplier, "backoff-multiplier", 1.6, "The factor with which to multiply backoffs after a failed retry. Should ideally be greater than 1.")
	fs.Float64Var(&opts.backoffJitter, "backoff-jitter", 0.2, "The factor with which backoffs are randomized.")
	fs.DurationVar(&opts.backoffMaxDelay, "backoff-max-delay", 120*time.Second, "The upper bound of backoff delay.")
	fs.DurationVar(&opts.minConnectTimeout, "min-connect-timeout", 30*time.Second, "The minimum amount of time we are willing to give a connection to complete.")
	fs.Var(&opts.nodeIDs, "node", "Node IDs to register for (can be provided multiple times).")
	fs.Var(&opts.logLevel, "log-level", "Sets the log level (one of trace, debug, info, warn, error, fatal, or panic).")
	fs.StringVar(&opts.channelzAddr, "channelz-addr", "", "Local address to start the channelz server on. Leave blank to keep the server disabled.")
	fs.StringVar(&opts.otelExporterAddr, "otel-exporter-addr", "", "Address of OTEL exporter. Leave blank to keep tracing disabled.")

	// TODO: switch to a textproto config and update the README usage doc
	fs.Var(&opts.telemetryCmd, "telemetry-cmd", "The optional command to run that will generate a NetworkStatsReport message in JSON format.")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if opts.serverAddr == "" {
		return nil, errors.New("no CDPI endpoint (--cdpi-endpoint) provided")
	}
	if !opts.insecure && opts.authJWT == "" {
		return nil, errors.New("no authorization JWT (--authorization-jwt) provided")
	}
	if !opts.insecure && opts.proxyAuthJWT == "" {
		return nil, errors.New("no proxy authorization JWT (--proxy-authorization-jwt) provided")
	}
	if len(opts.nodeIDs) == 0 {
		return nil, errors.New("no nodes provided (--node)")
	}
	if fs.NArg() == 0 {
		return nil, errors.New("no enactment command provided")
	}

	opts.enactmentCmd = fs.Args()
	return opts, nil
}

func (opts *cliOpts) dialOpts(ctx context.Context) ([]grpc.DialOption, error) {
	tp, ok := task.ExtractTracerProvider(ctx)
	if !ok {
		return nil, errors.New("no tracer provider available in provided context")
	}

	grpcOpts := []grpc.DialOption{
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  opts.backoffBaseDelay,
				Multiplier: opts.backoffMultiplier,
				Jitter:     opts.backoffJitter,
				MaxDelay:   opts.backoffMaxDelay,
			},
			MinConnectTimeout: opts.minConnectTimeout,
		}),
		grpc.WithStreamInterceptor(
			otelgrpc.StreamClientInterceptor(
				otelgrpc.WithTracerProvider(tp),
				otelgrpc.WithPropagators(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})),
			)),
	}

	if opts.plaintext {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		// TODO: provide a flag to override using the system cert pool
		cp, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("error reading system tls cert pool: %w", err)
		}
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(cp, "")))
	}

	if !opts.insecure {
		log := zerolog.Ctx(ctx)
		proxyJWT, err := exchangeProxyJWT(ctx, opts.proxyAuthJWT)
		if err != nil {
			return nil, fmt.Errorf("error exchanging proxy JWT: %w", err)
		}
		log.Trace().Str("proxyJWT", opts.proxyAuthJWT).Str("oidcToken", proxyJWT).Msg("exchanged proxy JWT for OIDC token")

		grpcOpts = append(grpcOpts,
			grpc.WithPerRPCCredentials(jwtBearerAuth{header: "authorization", jwt: opts.authJWT}),
			grpc.WithPerRPCCredentials(jwtBearerAuth{header: "proxy-authorization", jwt: proxyJWT}),
		)
	}

	return grpcOpts, nil
}

func (opts *cliOpts) newAgent(ctx context.Context) (a *agent.Agent, err error) {
	err = task.Task(func(ctx context.Context) error {
		grpcOpts, err := opts.dialOpts(ctx)
		if err != nil {
			return err
		}

		enactmentImpl := enact_extproc.New(func(ctx context.Context) *exec.Cmd {
			return exec.CommandContext(ctx, opts.enactmentCmd[0], opts.enactmentCmd[1:]...)
		})

		clock := clockwork.NewRealClock()
		agentOpts := []agent.AgentOption{
			agent.WithClock(clock),
			agent.WithServerEndpoint(opts.serverAddr),
			agent.WithDialOpts(grpcOpts...),
		}

		// TODO: change cli to allow per-node opts, for now both enactment
		// and telemetry commands are applied to all nodes
		commonNodeOpts := []agent.NodeOption{agent.WithEnactmentBackend(enactmentImpl)}
		if cmd := opts.telemetryCmd; len(cmd) != 0 {
			commonNodeOpts = append(commonNodeOpts, agent.WithTelemetryBackend(telem_extproc.New(func(ctx context.Context) *exec.Cmd {
				return exec.CommandContext(ctx, opts.telemetryCmd[0], opts.telemetryCmd[1:]...)
			})))
		}

		for _, n := range opts.nodeIDs {
			nodeOpts := append([]agent.NodeOption{
				agent.WithInitialState(&afpb.ControlStateNotification{NodeId: proto.String(n)})},
				commonNodeOpts...)

			agentOpts = append(agentOpts, agent.WithNode(n, nodeOpts...))
		}

		a, err = agent.NewAgent(agentOpts...)
		return err
	}).WithNewSpan("opts.newAgent")(ctx)

	return a, err
}

func (opts *cliOpts) channelzServer() task.Task {
	if opts.channelzAddr == "" {
		return task.Noop()
	}

	return func(ctx context.Context) error {
		lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", opts.channelzAddr)
		if err != nil {
			return fmt.Errorf("channelz error: %w", err)
		}

		srv := grpc.NewServer(
			grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor()),
			grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
		)
		channelz.RegisterChannelzServiceToServer(srv)

		return task.Group(
			func(ctx context.Context) error {
				<-ctx.Done()
				srv.Stop()
				return nil
			},
			task.Task(func(ctx context.Context) error {
				zerolog.Ctx(ctx).Debug().
					Str("addr", lis.Addr().String()).
					Msg("starting channelz server")

				return srv.Serve(lis)
			}).WithSpanAttributes(attribute.String("channelz.address", lis.Addr().String())),
		)(ctx)
	}
}

func (o *cliOpts) newTracerProvider(ctx context.Context) (*otelsdktrace.TracerProvider, error) {
	traceOpts := []otelsdktrace.TracerProviderOption{}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("extproc_agent"),
			semconv.ServiceNamespaceKey.String("spacetime"),
			semconv.ServiceVersion("v0.1.0"),
		),
	)
	if err != nil {
		return nil, err
	}
	traceOpts = append(traceOpts, otelsdktrace.WithResource(res))

	// Only add an exporter to the options if requested. This way we can still
	// include the tracing infra without actually exporting anything if it's
	// not requested.
	if o.otelExporterAddr != "" {
		exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(o.otelExporterAddr))
		if err != nil {
			return nil, err
		}
		traceOpts = append(traceOpts, otelsdktrace.WithBatcher(exporter))
	}

	return otelsdktrace.NewTracerProvider(traceOpts...), nil
}

func (o *cliOpts) WithTeardown(ctx context.Context) (context.Context, func(), error) {
	log := (*zerolog.Ctx(ctx)).Level(zerolog.Level(o.logLevel))

	tp, err := o.newTracerProvider(ctx)
	if err != nil {
		return nil, nil, err
	}

	ctx = log.WithContext(ctx)
	ctx = task.InjectTracerProvider(ctx, tp)

	shutdown := func() {
		if tpErr := tp.Shutdown(ctx); tpErr != nil {
			log.Error().Err(tpErr).Msg("error shutting down trace provider")
		}
	}

	return ctx, shutdown, nil
}
