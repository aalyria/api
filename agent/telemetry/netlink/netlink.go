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
	"time"

	"aalyria.com/spacetime/agent/telemetry"
	apipb "aalyria.com/spacetime/api/common"
	telemetrypb "aalyria.com/spacetime/telemetry/v1alpha"

	"github.com/jonboulle/clockwork"
	"github.com/rs/zerolog"
	vnl "github.com/vishvananda/netlink"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var errNoStats = errors.New("could not generate stats for any interface")

type reportGenerator struct {
	clock        clockwork.Clock
	interfaceIDs []string
	linkByName   func(string) (vnl.Link, error)
}

func NewDriver(
	clock clockwork.Clock,
	interfaceIDs []string,
	linkByName func(string) (vnl.Link, error),
	collectionPeriod time.Duration,
) (telemetry.Driver, error) {
	return telemetry.NewPeriodicDriver(&reportGenerator{
		clock:        clock,
		interfaceIDs: interfaceIDs,
		linkByName:   linkByName,
	}, clock, collectionPeriod)
}

func (rg *reportGenerator) Stats() any { return nil }

func (rg *reportGenerator) GenerateReport(ctx context.Context, nodeID string) (*telemetrypb.ExportMetricsRequest, error) {
	log := zerolog.Ctx(ctx).With().Str("backend", "netlink").Logger()
	// NOTE: This assumes the netlink stats are returned fast enough that we can use the same
	// timestamp for all metrics.
	ts := rg.clock.Now()

	interfaceMetrics := []*telemetrypb.InterfaceMetrics{}

	for _, interfaceID := range rg.interfaceIDs {
		log := log.With().Str("interfaceID", interfaceID).Logger()

		textNetIfaceID, err := prototext.Marshal(&apipb.NetworkInterfaceId{
			NodeId:      proto.String(nodeID),
			InterfaceId: proto.String(interfaceID),
		})
		if err != nil {
			log.Err(err).Msg("marshalling textproto interface ID")
			continue
		}

		link, err := rg.linkByName(interfaceID)
		if err != nil {
			log.Warn().Err(err).Msg("retrieving link for interface")
			continue
		}

		attrs := link.Attrs()
		if attrs == nil {
			log.Warn().Msg("link has no attrs")
			continue
		}

		stats := attrs.Statistics
		if stats == nil {
			log.Warn().Msg("link attrs have no stats")
			continue
		}

		interfaceMetrics = append(interfaceMetrics, &telemetrypb.InterfaceMetrics{
			InterfaceId: string(textNetIfaceID),
			OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{{
				Time:  timestamppb.New(ts),
				Value: netlinkOperStateToTelemetryOperState(attrs.OperState),
			}},
			StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{{
				Time:      timestamppb.New(ts),
				RxPackets: int64(stats.RxPackets),
				TxPackets: int64(stats.TxPackets),
				RxBytes:   int64(stats.RxBytes),
				TxBytes:   int64(stats.TxBytes),
				TxErrors:  int64(stats.TxErrors),
				RxErrors:  int64(stats.RxErrors),
				RxDropped: int64(stats.RxDropped),
				TxDropped: int64(stats.TxDropped),
			}},
		})
	}

	if len(interfaceMetrics) == 0 {
		return nil, errNoStats
	}

	return &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: interfaceMetrics,
	}, nil
}

func netlinkOperStateToTelemetryOperState(s vnl.LinkOperState) telemetrypb.IfOperStatus {
	switch s {
	case vnl.OperUnknown:
		return telemetrypb.IfOperStatus_IF_OPER_STATUS_UNKNOWN
	case vnl.OperNotPresent:
		return telemetrypb.IfOperStatus_IF_OPER_STATUS_NOT_PRESENT
	case vnl.OperDown:
		return telemetrypb.IfOperStatus_IF_OPER_STATUS_DOWN
	case vnl.OperLowerLayerDown:
		return telemetrypb.IfOperStatus_IF_OPER_STATUS_LOWER_LAYER_DOWN
	case vnl.OperTesting:
		return telemetrypb.IfOperStatus_IF_OPER_STATUS_TESTING
	case vnl.OperDormant:
		return telemetrypb.IfOperStatus_IF_OPER_STATUS_DORMANT
	case vnl.OperUp:
		return telemetrypb.IfOperStatus_IF_OPER_STATUS_UP
	default:
		return telemetrypb.IfOperStatus_IF_OPER_STATUS_UNSPECIFIED
	}
}
