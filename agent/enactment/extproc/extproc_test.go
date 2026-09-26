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
	"testing"

	"aalyria.com/spacetime/agent/internal/protofmt"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
)

func TestDispatchEmptyOutput(t *testing.T) {
	for _, format := range []protofmt.Format{protofmt.JSON, protofmt.Wire, protofmt.Text} {
		t.Run(format.String(), func(t *testing.T) {
			eb := New([]string{"/bin/true"}, format)

			if err := eb.Dispatch(context.Background(), &schedpb.CreateEntryRequest{}); err != nil {
				t.Errorf("unexpected error from /bin/true command: %v", err)
				return
			}
		})
	}
}
