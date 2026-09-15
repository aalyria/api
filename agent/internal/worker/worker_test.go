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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/google/go-cmp/cmp"
	"github.com/jonboulle/clockwork"
	"golang.org/x/sync/errgroup"
)

func TestMapQueue(t *testing.T) {
	clock := clockwork.NewFakeClock()
	ctx, cancel := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)
	defer g.Wait()
	defer cancel()

	type input struct {
		key      string
		duration time.Duration
		payload  int
	}

	respCh := make(chan int)
	keyFn := func(i input) string { return i.key }
	workFn := func(_ context.Context, i input) error {
		clock.Sleep(i.duration)
		respCh <- i.payload
		return nil
	}

	mq := NewMapQueue(ctx, g, keyFn, workFn)
	mq.Enqueue(input{key: "a", duration: 1 * time.Second, payload: 1})
	mq.Enqueue(input{key: "a", duration: 3 * time.Second, payload: 2})
	mq.Enqueue(input{key: "b", duration: 1 * time.Second, payload: 3})

	// both the "a" and "b" 1s tasks should have been started in parallel
	clock.BlockUntil(2)
	clock.Advance(1 * time.Second)

	earlyResults := []int{<-respCh, <-respCh}
	if diff := cmp.Diff([]int{1, 3}, earlyResults, cmpopts.SortSlices(func(l, r int) bool {
		return l < r
	})); diff != "" {
		t.Errorf("earlyResults mismatch (-want +got):\n%s", diff)
	}

	// The second, longer "a" task should now be started
	clock.BlockUntil(1)
	clock.Advance(3 * time.Second)

	want := 2
	if got := <-respCh; got != want {
		t.Errorf("expected final job to finish with result %d, but got %d", want, got)
	}
}

func TestMapQueue_ctxCanceled(t *testing.T) {
	clock := clockwork.NewFakeClock()
	ctx, cancel := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)
	defer g.Wait()
	defer cancel()

	type input struct {
		key      string
		duration time.Duration
		payload  int
	}

	respCh := make(chan int)
	keyFn := func(i input) string { return i.key }
	workFn := func(_ context.Context, i input) error {
		clock.Sleep(i.duration)
		respCh <- i.payload
		return nil
	}

	mq := NewMapQueue(ctx, g, keyFn, workFn)

	cancel()
	if mq.Enqueue(input{key: "b", duration: 1 * time.Second, payload: 3}) != false {
		t.Errorf("expected queue.Enqueue to return false after ctx was canceled")
	}
}
