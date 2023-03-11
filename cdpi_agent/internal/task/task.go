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
	"math"
	"math/rand"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
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

func (t Task) With(fn func(Task) Task) Task { return fn(t) }

func (t Task) WithCtx(ctx context.Context) func() error {
	return func() error { return t(ctx) }
}
