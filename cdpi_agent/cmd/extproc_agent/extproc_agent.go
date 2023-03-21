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

// Package main provides a CDPI agent that delegates enactments to an external
// process.
package main

import (
	"context"
	"errors"
	_ "net/http/pprof"
	"os"
	"os/signal"

	"aalyria.com/spacetime/cdpi_agent/internal/task"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rs/zerolog"
)

func main() {
	var logger zerolog.Logger
	if os.Getenv("TERM") != "" {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "03:04:05PM"})
	} else {
		logger = zerolog.New(os.Stdout)
	}
	logger = logger.With().Timestamp().Logger()
	ctx := logger.WithContext(context.Background())

	if err := run(ctx, os.Args[1:]); errors.Is(err, context.Canceled) {
		logger.Debug().Msg("agent interrupted, exiting")
		os.Exit(int(codes.Canceled))
	} else if err != nil {
		logger.Error().Err(err).Msg("error running agent")
		os.Exit(int(status.Code(err)))
	}
}

func run(ctx context.Context, args []string) error {
	opts, err := parseOpts("extproc_agent", args...)
	if err != nil {
		return err
	}

	ctx, shutdown, err := opts.WithTeardown(ctx)
	if err != nil {
		return err
	}
	defer shutdown()

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	run := func(ctx context.Context) error {
		a, err := opts.newAgent(ctx)
		if err != nil {
			return err
		}

		return task.Group(
			opts.channelzServer().WithNewSpan("channelz"),
			task.Task(a.Run).WithNewSpan("agent.Run"),
		)(ctx)
	}

	return task.Task(run).
		WithNewSpan("run").
		WithOtelTracer("aalyria.com/cdpi_agent/cmd/extproc_agent")(ctx)
}
