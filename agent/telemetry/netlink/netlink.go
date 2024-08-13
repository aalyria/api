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

package netlink

import (
	"context"
	"errors"

	"aalyria.com/spacetime/agent/telemetry"
	apipb "aalyria.com/spacetime/api/common"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	vnl "github.com/vishvananda/netlink"
	"google.golang.org/protobuf/proto"
)

var errNoStats = errors.New("could not generate stats for any interface")

type driver struct {
	clock        clockwork.Clock
	interfaceIDs []string
	linkByName   func(string) (vnl.Link, error)
}

func New(
	clock clockwork.Clock,
	interfaceIDs []string,
	linkByName func(string) (vnl.Link, error),
) telemetry.Driver {
	return &driver{
		clock:        clock,
		interfaceIDs: interfaceIDs,
		linkByName:   linkByName,
	}
}

func (tb *driver) Init(context.Context) error { return nil }
func (tb *driver) Close() error               { return nil }
func (tb *driver) Stats() interface{}         { return nil }

func (tb *driver) GenerateReport(
	ctx context.Context,
	nodeID string,
) (*apipb.NetworkStatsReport, error) {
	log := zerolog.Ctx(ctx).With().Str("backend", "netlink").Logger()

	// NOTE: This assumes the netlink stats are returned fast enough that we can use the same
	// timestamp for all metrics.
	ts := tb.clock.Now().UnixMicro()

	interfaceStatsById := make(map[string]*apipb.InterfaceStats)

	for _, interfaceId := range tb.interfaceIDs {
		link, err := tb.linkByName(interfaceId)
		if err != nil {
			log.Warn().Err(err).Msgf("error retrieving link for interface %s", interfaceId)
			continue
		}

		attrs := link.Attrs()
		if attrs == nil {
			log.Warn().Msgf("link has not attrs for interface %s", interfaceId)
			continue
		}

		stats := attrs.Statistics
		if stats == nil {
			log.Warn().Msgf("link attrs have no stats for interface %s", interfaceId)
			continue
		}

		interfaceStatsById[interfaceId] = &apipb.InterfaceStats{
			Timestamp: &apipb.DateTime{
				UnixTimeUsec: proto.Int64(ts),
			},
			TxPackets: proto.Int64(int64(stats.TxPackets)),
			RxPackets: proto.Int64(int64(stats.RxPackets)),
			TxBytes:   proto.Int64(int64(stats.TxBytes)),
			RxBytes:   proto.Int64(int64(stats.RxBytes)),
			TxDropped: proto.Int64(int64(stats.TxDropped)),
			RxDropped: proto.Int64(int64(stats.RxDropped)),
			TxErrors:  proto.Int64(int64(stats.TxErrors)),
			RxErrors:  proto.Int64(int64(stats.RxErrors)),
		}
	}

	if len(interfaceStatsById) == 0 {
		return nil, errNoStats
	}

	report := &apipb.NetworkStatsReport{
		NodeId: proto.String(nodeID),
		Timestamp: &apipb.DateTime{
			UnixTimeUsec: proto.Int64(ts),
		},
		InterfaceStatsById: interfaceStatsById,
	}
	return report, nil
}
