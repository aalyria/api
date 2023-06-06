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

package agent

import (
	"sync"
	"time"

	apipb "aalyria.com/spacetime/api/common"

	"github.com/jonboulle/clockwork"
	"google.golang.org/protobuf/proto"
)

func hzToDuration(hz float64) time.Duration {
	// don't feed me a hz of 0!!!!!
	return time.Duration(float64(time.Second) / hz)
}

// reusableTicker is a wrapper around time.Ticker / clockwork.Ticker interface
// that can be created without being started and that always has a valid
// Chan(). If the ticker hasn't been started yet, the resulting channel will
// never receive any messages.
type reusableTicker struct {
	mu           sync.Mutex
	clock        clockwork.Clock
	activeTicker clockwork.Ticker
	inactiveCh   chan time.Time
	isActive     bool
}

func newReusableTicker(clock clockwork.Clock) *reusableTicker {
	return &reusableTicker{
		clock:      clock,
		inactiveCh: make(chan time.Time),
	}
}

func (r *reusableTicker) Stop() {
	wasActive := false
	var oldTicker clockwork.Ticker

	r.mu.Lock()
	wasActive, r.isActive = r.isActive, false
	oldTicker, r.activeTicker = r.activeTicker, nil
	r.mu.Unlock()

	if wasActive {
		oldTicker.Stop()
	}
}

func (r *reusableTicker) Start(d time.Duration) {
	newTicker := r.clock.NewTicker(d)
	var oldTicker clockwork.Ticker
	wasActive := false

	r.mu.Lock()
	wasActive, r.isActive = r.isActive, true
	oldTicker, r.activeTicker = r.activeTicker, newTicker
	r.mu.Unlock()

	if wasActive {
		oldTicker.Stop()
	}
}

func (r *reusableTicker) Chan() <-chan time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isActive {
		return r.activeTicker.Chan()
	} else {
		return r.inactiveCh
	}
}

func timeToProto(t time.Time) *apipb.DateTime {
	return &apipb.DateTime{
		UnixTimeUsec: proto.Int64(t.UnixMicro()),
	}
}
