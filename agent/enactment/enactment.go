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

	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
)

// Driver is the component that takes a CreateEntryRequest message for a
// given SDN agent and dispatches it. If the error returned implements the
// gRPCStatus interface, the appropriate status will be used.
type Driver interface {
	// Init initializes the driver.
	Init(context.Context) error
	// Dispatch handles the provided [schedpb.CreateEntryRequest]. Drivers are
	// expected to handle retries.
	Dispatch(context.Context, *schedpb.CreateEntryRequest) error
	// Stats returns internal statistics for the driver in an unstructured
	// form. The results are exposed as a JSON endpoint via the pprof server,
	// if it's configured.
	Stats() any
	// Close closes the driver. Reusing a closed driver is a fatal error.
	Close() error
}
