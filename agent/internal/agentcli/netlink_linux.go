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

//go:build linux

package agentcli

import (
	"context"
	"fmt"

	"aalyria.com/spacetime/agent/enactment"
	enact_netlink "aalyria.com/spacetime/agent/enactment/netlink"
	"aalyria.com/spacetime/agent/internal/configpb"
	"aalyria.com/spacetime/agent/telemetry"
	telemetry_netlink "aalyria.com/spacetime/agent/telemetry/netlink"

	"github.com/jonboulle/clockwork"
	vnl "github.com/vishvananda/netlink"
)

func newNetlinkEnactmentDriver(ctx context.Context, clock clockwork.Clock, nodeID string, conf *configpb.NetworkNode_NetlinkEnactment) (enactment.Driver, error) {
	nlHandle, err := vnl.NewHandle(vnl.FAMILY_ALL)
	if err != nil {
		return nil, fmt.Errorf("creating new netlink handle for enactments: %w", err)
	}

	return enact_netlink.New(
		enact_netlink.DefaultConfig(
			ctx,
			nlHandle,
			int(conf.GetRouteTableId()),
			int(conf.GetRouteTableLookupPriority()))), nil
}

func newNetlinkTelemetryDriver(ctx context.Context, clock clockwork.Clock, nodeID string, conf *configpb.NetworkNode_NetlinkTelemetry) (telemetry.Driver, error) {
	interfaceIDs := make([]string, 0, len(conf.GetMonitoredInterfaces()))
	for _, mi := range conf.GetMonitoredInterfaces() {
		interfaceIDs = append(interfaceIDs, mi.GetInterfaceId())
	}

	return telemetry_netlink.New(clock, interfaceIDs, vnl.LinkByName), nil
}
