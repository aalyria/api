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

	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
)

// Driver is the component that gathers and reports metrics for a given
// node.
type Driver interface {
	// Run will periodically report metrics by calling the provided `reportMetrics` callback.
	Run(
		ctx context.Context,
		nodeID string,
		reportMetrics func(*telemetrypb.ExportMetricsRequest) error,
	) error
	// Stats returns internal statistics for the driver in an unstructured
	// form. The results are exposed as a JSON endpoint via the pprof server,
	// if it's configured.
	Stats() any
}
