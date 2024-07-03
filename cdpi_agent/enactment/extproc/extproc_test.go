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

package extproc

import (
	"context"
	"os/exec"
	"testing"

	apipb "aalyria.com/spacetime/api/common"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"aalyria.com/spacetime/cdpi_agent/internal/protofmt"
	"github.com/google/go-cmp/cmp"
)

func TestApplyEmptyOutput(t *testing.T) {
	for _, format := range []protofmt.Format{protofmt.JSON, protofmt.Wire, protofmt.Text} {
		t.Run(format.String(), func(t *testing.T) {
			eb := New(func(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, "/bin/true") }, format)

			newState, err := eb.Apply(context.Background(), &apipb.ScheduledControlUpdate{})
			if err != nil {
				t.Errorf("unexpected error from /bin/true command: %v", err)
				return
			}
			if newState != nil {
				t.Errorf("unexpected non-nil new state from /bin/true: %v", newState)
			}
		})
	}
}

func TestApplyNonEmptyOutput(t *testing.T) {
	args := []string{"/bin/sh", "-c", `echo 'beam_states: {beam_task_ids: ["a", "b"]}'`}
	eb := New(func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, args[0], args[1:]...)
	}, protofmt.Text)

	newState, err := eb.Apply(context.Background(), &apipb.ScheduledControlUpdate{})
	if err != nil {
		t.Errorf(`unexpected error from %s: %v`, args, err)
		return
	}
	if newState == nil {
		t.Errorf("unexpected nil new state from %s: %v", args, newState)
	}
	want := []string{"a", "b"}
	got := newState.GetBeamStates().GetBeamTaskIds()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatched beam task IDs (-want +got): %s\n", diff)
	}
}

func TestDispatchEmptyOutput(t *testing.T) {
	for _, format := range []protofmt.Format{protofmt.JSON, protofmt.Wire, protofmt.Text} {
		t.Run(format.String(), func(t *testing.T) {
			eb := New(func(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, "/bin/true") }, format)

			err := eb.Dispatch(context.Background(), &schedpb.CreateEntryRequest{})
			if err != nil {
				t.Errorf("unexpected error from /bin/true command: %v", err)
				return
			}
		})
	}
}
