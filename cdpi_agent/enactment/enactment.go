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

package enactment

import (
	"context"

	apipb "aalyria.com/spacetime/api/common"
)

// Backend is the component that takes a ScheduledControlUpdate message for a
// given node and returns the new state for that node. If the error returned
// implements the gRPCStatus interface, the appropriate status will be used.
type Backend interface {
	Apply(context.Context, *apipb.ScheduledControlUpdate) (*apipb.ControlPlaneState, error)
	Stats() interface{}
	Init(context.Context) error
	Close() error
}
