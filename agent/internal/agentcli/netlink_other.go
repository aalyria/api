// Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !linux

package agentcli

import (
	"context"
	"fmt"

	"aalyria.com/spacetime/agent/enactment"
	"aalyria.com/spacetime/agent/internal/configpb"
	"aalyria.com/spacetime/agent/telemetry"

	"github.com/jonboulle/clockwork"
)

func newNetlinkEnactmentDriver(ctx context.Context, clock clockwork.Clock, nodeID string, conf *configpb.NetworkNode_NetlinkEnactment) (enactment.Driver, error) {
	return nil, fmt.Errorf("netlink enactment driver is unavailable on this platform")
}

func newNetlinkTelemetryDriver(ctx context.Context, clock clockwork.Clock, nodeID string, conf *configpb.NetworkNode_NetlinkTelemetry) (telemetry.Driver, error) {
	return nil, fmt.Errorf("netlink telemetry driver is unavailable on this platform")
}
