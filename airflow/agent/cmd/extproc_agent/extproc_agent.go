// Package main provides a CDPI agent that delegates enactments to an external
// process.
//
// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
// Confidential and Proprietary. All rights reserved.
package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"aalyria.com/minkowski/airflow/agent"
	"aalyria.com/minkowski/airflow/agent/enactment/extproc"
	afpb "aalyria.com/minkowski/api/airflow"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
)

// This is the audience string for Spacetime's CDPI endpoint.
const audience = "60292403139-me68tjgajl5dcdbpnlm2ek830lvsnslq.apps.googleusercontent.com"

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
	enactmentCmd      []string
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

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if opts.serverAddr == "" {
		return nil, errors.New("No CDPI endpoint (--cdpi-endpoint) provided")
	}
	if !opts.insecure && opts.authJWT == "" {
		return nil, errors.New("No authorization JWT (--authorization-jwt) provided")
	}
	if !opts.insecure && opts.proxyAuthJWT == "" {
		return nil, errors.New("No proxy authorization JWT (--proxy-authorization-jwt) provided")
	}
	if len(opts.nodeIDs) == 0 {
		return nil, errors.New("No nodes provided (--node)")
	}
	if fs.NArg() == 0 {
		return nil, errors.New("No enactment command provided")
	}

	opts.enactmentCmd = fs.Args()
	return opts, nil
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
	resp, err := (&http.Client{}).Do(req)
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

func (opts *cliOpts) dialOpts(ctx context.Context) ([]grpc.DialOption, error) {
	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  opts.backoffBaseDelay,
				Multiplier: opts.backoffMultiplier,
				Jitter:     opts.backoffJitter,
				MaxDelay:   opts.backoffMaxDelay,
			},
			MinConnectTimeout: opts.minConnectTimeout,
		}),
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

func main() {
	var logger zerolog.Logger
	if os.Getenv("TERM") != "" {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr})
	} else {
		logger = zerolog.New(os.Stdout)
	}
	logger = logger.With().Timestamp().Logger()

	if err := run(context.Background(), logger, os.Args[1:]); err != nil {
		logger.Error().Err(err).Msg("Error running agent")
		os.Exit(1)
	}
}

func run(ctx context.Context, logger zerolog.Logger, args []string) error {
	opts, err := parseOpts("extproc_agent", args...)
	if err != nil {
		return err
	}

	logger = logger.Level(zerolog.Level(opts.logLevel))
	ctx, done := context.WithCancel(logger.WithContext(ctx))
	defer done()

	grpcOpts, err := opts.dialOpts(ctx)
	if err != nil {
		return err
	}

	impl := &extproc.EnactmentBackend{CmdFn: func() *exec.Cmd {
		return exec.CommandContext(ctx, opts.enactmentCmd[0], opts.enactmentCmd[1:]...)
	}}
	agent := agent.NewAgent(impl, clockwork.NewRealClock())
	if err := agent.Start(ctx, opts.serverAddr, grpcOpts...); err != nil {
		return fmt.Errorf("error starting agent: %w", err)
	}

	errCh := make(chan error)
	initState := &afpb.ControlStateNotification{}
	for _, n := range opts.nodeIDs {
		nodeLogger := logger.With().Str("nodeID", n).Logger()
		nodeCtx := nodeLogger.WithContext(ctx)

		initState.NodeId = proto.String(n)
		nodeErrCh, err := agent.RegisterFor(nodeCtx, initState)
		if err != nil {
			return fmt.Errorf("error registering for node: %w", err)
		}
		go func() {
			select {
			case <-ctx.Done():
			case err := <-nodeErrCh:
				errCh <- err
			}
		}()
		nodeLogger.Debug().Str("nodeID", n).Msg("registered node")
	}

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)

	combinedErrCh := make(chan error)
	go func() {
		errs := []error{}
		for {
			select {
			case <-ctx.Done():
				return

			case err := <-errCh:
				if errs = append(errs, err); len(errs) == len(opts.nodeIDs) {
					combinedErrCh <- errors.Join(errs...)
					return
				}
			}
		}
	}()

	// block 'till we're done
	select {
	case err := <-combinedErrCh:
		logger.Warn().Err(err).Msg("all node streams finished, exiting now")
		return err

	case <-interrupts:
		logger.Info().Msg("interrupted, exiting now")
		return nil

	case <-ctx.Done():
		logger.Info().Msg("context finished, exiting now")
		return ctx.Err()
	}
}