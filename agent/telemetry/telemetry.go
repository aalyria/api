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

package telemetry

import (
	"context"

	apipb "aalyria.com/spacetime/api/common"
)

// Driver is the component that generates TelemetryUpdate messages for a given
// node. It may be called either periodically or as a result of a one-off
// [apipb.TelemetryRequest].
type Driver interface {
	// Init initializes the driver.
	Init(context.Context) error
	// GenerateReport generates an [apipb.NetworkStatsReport] for the given [nodeID].
	GenerateReport(ctx context.Context, nodeID string) (*apipb.NetworkStatsReport, error)
	// Stats returns internal statistics for the driver in an unstructured
	// form. The results are exposed as a JSON endpoint via the pprof server,
	// if it's configured.
	Stats() any
	// Close closes the driver. Reusing a closed driver is a fatal error.
	Close() error
}
