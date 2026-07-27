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

package worker

import (
	"context"
	"sync"
)

// Queue is something that can enqueue a work item for asynchronous processing.
type Queue[T any] interface {
	Enqueue(T) bool
}

// SerialQueue is an extremely basic Queue implementation that handles incoming
// work items one by one.
type SerialQueue[T any] struct {
	mu      sync.Mutex
	workFn  func(context.Context, T) error
	ready   chan struct{}
	doneCh  <-chan struct{}
	pending []T
}

type Pool interface {
	Go(func() error)
}

// NewSerialQueue creates a new serial queue that will handle incoming work
// items one by one. The single worker is stopped once the provided context is
// canceled. Depending on the `pool` implementation, if the `workFn` returns a
// non-nil error the context may be canceled. In both cases, any pending work
// items are lost.
func NewSerialQueue[T any](ctx context.Context, pool Pool, workFn func(context.Context, T) error) Queue[T] {
	sq := &SerialQueue[T]{
		workFn: workFn,
		mu:     sync.Mutex{},
		ready:  make(chan struct{}, 1),
		doneCh: ctx.Done(),
	}

	pool.Go(func() error { return sq.start(ctx) })

	return sq
}

func (q *SerialQueue[T]) start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-q.ready:
		}

		q.mu.Lock()
		todo := q.pending
		q.pending = nil
		q.mu.Unlock()

		for _, next := range todo {
			if err := q.workFn(ctx, next); err != nil {
				return err
			}
		}
	}
}

// Add the provided `item` to the queue. Returns true if the item was added
// successfully.
func (q *SerialQueue[T]) Enqueue(item T) bool {
	select {
	case <-q.doneCh:
		return false
	default:
	}

	q.mu.Lock()
	q.pending = append(q.pending, item)
	q.mu.Unlock()

	select {
	case q.ready <- struct{}{}:
	default:
	}
	return true
}

// A MapQueue is a grouping of multiple queues. Items are sent to a queue
// determined by the `keyFn`.
type MapQueue[K comparable, T any] struct {
	keyFn   func(T) K
	factory func(K) Queue[T]
	mu      sync.Mutex
	qs      map[K]Queue[T]
	doneCh  <-chan struct{}
}

// NewMapQueue creates a new map queue that partitions incoming work items
// based on the provided `keyFn`. If the provided `ctx` is canceled, the entire
// set of queues will be canceled and pending work items may be lost.
// Additionally, the standard `errgroup` `pool` implementation will cancel the
// context if the `workFn` returns a non-nil error, which will also cause
// pending work items to be lost.
func NewMapQueue[K comparable, T any](ctx context.Context, pool Pool, keyFn func(T) K, workFn func(context.Context, T) error) *MapQueue[K, T] {
	return &MapQueue[K, T]{
		keyFn: keyFn,
		factory: func(_ K) Queue[T] {
			return NewSerialQueue(ctx, pool, workFn)
		},
		qs:     make(map[K]Queue[T]),
		mu:     sync.Mutex{},
		doneCh: ctx.Done(),
	}
}

func (mq *MapQueue[K, T]) Enqueue(item T) bool {
	key := mq.keyFn(item)

	mq.mu.Lock()
	q, ok := mq.qs[key]
	if !ok {
		q = mq.factory(key)
		mq.qs[key] = q
	}
	mq.mu.Unlock()

	return q.Enqueue(item)
}
