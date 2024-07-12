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
	"fmt"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
)

func TestHzToDuration(t *testing.T) {
	t.Parallel()

	for hz, want := range map[float64]time.Duration{
		0.1:  10 * time.Second,
		1.0:  1 * time.Second,
		10:   100 * time.Millisecond,
		100:  10 * time.Millisecond,
		1000: 1 * time.Millisecond,
	} {
		hz, want := hz, want

		t.Run(fmt.Sprintf("%fhz = %s", hz, want), func(t *testing.T) {
			t.Parallel()
			got := hzToDuration(hz)
			if got != want {
				t.Errorf("converting %fhz to duration, got %s but wanted %s", hz, got, want)
			}
		})
	}
}

func TestReusableTicker(t *testing.T) {
	t.Parallel()
	fc := clockwork.NewFakeClock()
	rt := newReusableTicker(fc)

	rt.Start(30 * time.Second)
	fc.Advance(30 * time.Second)
	tickedAt := <-rt.Chan()
	if tickedAt != fc.Now() {
		t.Errorf("ticker sent wrong time on channel, got %s, want %s", tickedAt, fc.Now())
	}

	rt.Stop()
	fc.Advance(3 * time.Hour)
	select {
	case tickedAt = <-rt.Chan():
		t.Errorf("ticker incorrectly ticked with value %s after being stopped", tickedAt)
	default:
		// yay
	}
}

func TestReusableTicker_CallingStartMultipleTimes(t *testing.T) {
	t.Parallel()
	fc := clockwork.NewFakeClock()
	rt := newReusableTicker(fc)

	rt.Start(30 * time.Second)
	rt.Start(30 * time.Second)
	rt.Start(30 * time.Second)

	fc.Advance(30 * time.Second)
	tickedAt := <-rt.Chan()
	if tickedAt != fc.Now() {
		t.Errorf("ticker sent wrong time on channel, got %s, want %s", tickedAt, fc.Now())
	}

	rt.Stop()
	fc.Advance(3 * time.Hour)
	select {
	case tickedAt = <-rt.Chan():
		t.Errorf("ticker incorrectly ticked with value %s after being stopped", tickedAt)
	default:
		// yay
	}
}
