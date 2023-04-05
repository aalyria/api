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

// Package task provides some useful helpers to express common tasks like
// retries and adding contextual information to a context-scoped logger.
package task

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	otelsdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

var (
	errNoTasks = errors.New("no tasks provided")
)

// Task is an abstraction over any context-bound, fallible activity.
type Task func(context.Context) error

type RetryConfig struct {
	Clock           clockwork.Clock
	MaxRetries      int
	BackoffDuration time.Duration
	ErrIsFatal      func(error) bool
}

// WithRetries returns a new task that will retry the inner task
// according to the provided retryConfig.
func (t Task) WithRetries(rc RetryConfig) Task {
	return func(ctx context.Context) error {
		log := zerolog.Ctx(ctx)
		var err error

		for retryCount := 0; rc.MaxRetries == 0 || retryCount <= rc.MaxRetries; retryCount++ {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			default:
			}

			if err = t(ctx); err != nil {
				if rc.ErrIsFatal != nil && rc.ErrIsFatal(err) {
					return err
				}

				// delayDur is within [0.5 * backoff, 1.5 * backoff]
				randFact := rand.Float64() - 0.5
				jitterMs := time.Millisecond * time.Duration(
					math.Round(randFact*float64(rc.BackoffDuration.Milliseconds())))
				delayDur := rc.BackoffDuration + jitterMs

				log.Error().
					Err(err).
					Dur("backoffDelay", delayDur).
					Msg("error, retrying shortly")

				timer := rc.Clock.NewTimer(delayDur)
				select {
				case <-ctx.Done():
					if !timer.Stop() {
						// drain the timer channel if we weren't able to stop it
						<-timer.Chan()
					}

				case <-timer.Chan():
				}
			}
		}
		return err
	}
}

// WithLogContext returns a new task that will apply the provided function
// to the context-scoped logger before invoking the inner task.
func (t Task) WithLogContext(fn func(zerolog.Context) zerolog.Context) Task {
	return func(ctx context.Context) error {
		log := fn(zerolog.Ctx(ctx).With()).Logger()
		return t(log.WithContext(ctx))
	}
}

// WithLogField returns a new task that will add the provided key/value pair
// to the context-scoped logger before invoking the inner task.
func (t Task) WithLogField(k, v string) Task {
	return t.WithLogContext(func(logctx zerolog.Context) zerolog.Context {
		return logctx.Str(k, v)
	})
}

// WithStartingStoppingLogs returns a new task that will log a "starting"
// message before and a "stopping" message after invoking the inner task.
func (t Task) WithStartingStoppingLogs(name string, lvl zerolog.Level) Task {
	return func(ctx context.Context) error {
		log := zerolog.Ctx(ctx)

		log.WithLevel(lvl).Msg(name + " starting")
		err := t(ctx)
		log.WithLevel(lvl).Err(err).Msg(name + " stopping")

		return err
	}
}

// WithCtx converts the inner task into a `func() error` that uses the provided
// context.
func (t Task) WithCtx(ctx context.Context) func() error {
	return func() error { return t(ctx) }
}

// AsThunk returns a no-argument, no-result function that calls the inner task
// and sets the provided `err` pointer to the returned error. Note that `err`
// will be overwritten regardless of the inner function result.
func (t Task) AsThunk(ctx context.Context, err *error) func() {
	return func() { *err = t(ctx) }
}

func Noop() Task { return func(_ context.Context) error { return nil } }

// WithOtelTracerProvider returns a task that injects the provided
// TracerProvider into the context before invoking the inner task.
func (t Task) WithOtelTracerProvider(tp *otelsdktrace.TracerProvider) Task {
	return func(ctx context.Context) error {
		return t(InjectTracerProvider(ctx, tp))
	}
}

// WithOtelTracer returns a task that injects a tracer, created using the `pkg`
// argument, into the context before invoking the inner task.
func (t Task) WithOtelTracer(pkg string) Task {
	return func(ctx context.Context) error {
		log := zerolog.Ctx(ctx)
		log.Trace().Msg("adding otel tracer to context")

		tp, ok := ExtractTracerProvider(ctx)
		if !ok {
			return fmt.Errorf("tracing requested, but no tracer provider present in context")
		}
		return t(InjectTracer(ctx, tp.Tracer(pkg)))
	}
}

// WithNewSpan starts a new trace span named `name`.
func (t Task) WithNewSpan(name string, opts ...oteltrace.SpanStartOption) Task {
	return func(ctx context.Context) (err error) {
		if tracer, ok := ExtractTracer(ctx); ok {
			var span oteltrace.Span
			ctx, span = tracer.Start(ctx, name, opts...)
			defer func() {
				if err != nil {
					span.SetStatus(otelcodes.Error, err.Error())
					span.RecordError(err)
				}
				span.End()
			}()
		}

		return t(ctx)
	}
}

// WithSpanAttributes returns a task that sets the trace span attributes to
// `attrs` before invoking the inner task.
func (t Task) WithSpanAttributes(attrs ...attribute.KeyValue) Task {
	return func(ctx context.Context) error {
		span := oteltrace.SpanFromContext(ctx)
		span.SetAttributes(attrs...)

		return t(ctx)
	}
}

// Group takes a sequence of Tasks and returns a task that starts them all in
// separate goroutines and waits for them all to finish. The semantics mirror
// those of the errgroup package, so only the first non-nil error will be
// returned.
func Group(fns ...Task) Task {
	if len(fns) == 0 {
		return func(ctx context.Context) error { return errNoTasks }
	}

	return func(ctx context.Context) error {
		g, ctx := errgroup.WithContext(ctx)
		for _, f := range fns {
			g.Go(f.WithCtx(ctx))
		}
		return g.Wait()
	}
}

// LoopUntilError returns a Task that runs the inner task in a loop until it
// returns a non-nil error.
func (t Task) LoopUntilError() Task {
	return func(ctx context.Context) error {
		for {
			if err := t(ctx); err != nil {
				return err
			}
		}
	}
}

type tracerKey struct{}
type tracerProviderKey struct{}

// ExtractTracer extracts the otel tracer from the provided context. Use
// `InjectTracer` to prepare a context for use with this function.
func ExtractTracer(ctx context.Context) (oteltrace.Tracer, bool) {
	t, ok := ctx.Value(tracerKey{}).(oteltrace.Tracer)
	return t, ok
}

// InjectTracer returns a new context with the provided Tracer injected. Use
// `ExtractTracer` to retrieve it.
func InjectTracer(ctx context.Context, t oteltrace.Tracer) context.Context {
	return context.WithValue(ctx, tracerKey{}, t)
}

// ExtractTracerProvider extracts the otel tracer provider from the provided
// context. Use `InjectTracerProvider` to prepare a context for use with this
// function.
func ExtractTracerProvider(ctx context.Context) (*otelsdktrace.TracerProvider, bool) {
	tp, ok := ctx.Value(tracerProviderKey{}).(*otelsdktrace.TracerProvider)
	return tp, ok
}

// InjectTracerProvider returns a new context with the provided TracerProvider
// injected. Use `ExtractTracerProvider` to retrieve it.
func InjectTracerProvider(ctx context.Context, tp *otelsdktrace.TracerProvider) context.Context {
	return context.WithValue(ctx, tracerProviderKey{}, tp)
}
