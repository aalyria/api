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
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
)

func baseContext(t *testing.T) context.Context {
	log := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Stack().Caller().Logger()
	return log.WithContext(context.Background())
}

func newAgent(t *testing.T, opts ...AgentOption) *Agent {
	t.Helper()

	a, err := NewAgent(opts...)
	if err != nil {
		t.Fatalf("error creating agent: %s", err)
	}
	return a
}

func check(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}

func assertProtosEqual(t *testing.T, want, got interface{}) {
	t.Helper()

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("proto mismatch: (-want +got):\n%s", diff)
		t.FailNow()
	}
}

var rpcCanceledError = status.FromContextError(context.Canceled).Err()

func checkErrIsDueToCanceledContext(t *testing.T, err error) {
	if !errors.Is(err, context.Canceled) && !errors.Is(err, rpcCanceledError) {
		t.Error("unexpected error:", err)
	}
}


