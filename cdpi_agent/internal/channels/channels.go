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

// Package channels provide some simple adapters to facilitate common
// read/write patterns.
package channels

import (
	"context"

	"aalyria.com/spacetime/cdpi_agent/internal/task"

	"github.com/rs/zerolog"
)

type Source[T any] <-chan T
type Sink[T any] chan<- T
type Receiver[T any] func() (T, error)
type Sender[T any] func(T) error

// NewSource takes a readable channel and returns a Source, which can be used
// to chain common transformations fluently.
func NewSource[T any](c <-chan T) Source[T] { return Source[T](c) }

// NewSink takes a writable channel and returns a Sink, which can be used to
// chain common transformations fluently.
func NewSink[T any](c chan<- T) Sink[T] { return Sink[T](c) }

func (s Sink[T]) FillFrom(recv Receiver[T]) task.Task {
	return task.Task(func(ctx context.Context) error {
		log := zerolog.Ctx(ctx)
		log.Trace().Msg("waiting for message")
		msg, err := recv()
		if err != nil {
			return err
		}
		log.Trace().Msg("got msg")

		select {
		case <-ctx.Done():
			log.Warn().Msg("discarding received msg because context was cancelled")
			return context.Cause(ctx)
		case s <- msg:
		}

		return nil
	}).
		WithNewSpan("channels.Sink.FillFrom").
		LoopUntilError()
}

func (s Source[T]) ForwardTo(send Sender[T]) task.Task {
	return task.Task(func(ctx context.Context) error {
		log := zerolog.Ctx(ctx)
		select {
		case <-ctx.Done():
			return context.Cause(ctx)

		case msg := <-s:
			log.Trace().Msg("sending message")
			if err := send(msg); err != nil {
				return err
			}
		}
		return nil
	}).
		WithNewSpan("channels.Source.ForwardTo").
		LoopUntilError()
}

type MapFn[A, B any] func(context.Context, A) (B, error)

func MapBetween[A, B any](src Source[A], dst Sink[B], fn MapFn[A, B]) task.Task {
	return task.Task(func(ctx context.Context) error {
		var before A
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case before = <-src:
		}

		after, err := fn(ctx, before)
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case dst <- after:
		}
		return nil
	}).
		WithNewSpan("channels.MapBetween").
		LoopUntilError()
}
