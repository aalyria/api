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

// Package snmp provides a telemetry.Backend implementation that relies on
// SNMP "get" calls to generate telemetry reports in the specified form.
package snmp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"time"

	"aalyria.com/spacetime/agent/internal/snmp"
	"aalyria.com/spacetime/agent/telemetry"
	apipb "aalyria.com/spacetime/api/common"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"

	gosnmp "github.com/gosnmp/gosnmp"
	"github.com/iancoleman/strcase"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	errEmptyReport = errors.New("command generated an empty response")
	promMetrics    = make(map[string]*prometheus.Gauge)
)

type reportGenerator struct {
	clock         clockwork.Clock
	snmpStruct    *gosnmp.GoSNMP
	snmpMetrics   []snmp.Metric
	operStatusMap map[string]*telemetrypb.IfOperStatus
	ctx           context.Context
}

func NewDriver(ctx context.Context, snmpStruct *gosnmp.GoSNMP, snmpMetrics []snmp.Metric, collectionPeriod time.Duration,
	prometheusPort int32, deviceName string,
) (telemetry.Driver, error) {
	go func() {
		// Builds out the map of Prometheus Gauges from the contents of snmpMetrics
		// TODO: Check whether the metric should be a Counter, Gauge, etc.
		for _, metric := range snmpMetrics {
			promGauge := promauto.NewGauge(prometheus.GaugeOpts{
				Name: strcase.ToSnake(deviceName + metric.DisplayName),
				Help: strcase.ToSnake(deviceName + metric.DisplayName),
			})
			promMetrics[metric.DisplayName] = &promGauge
		}

		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":"+strconv.Itoa(int(prometheusPort)), nil); err != nil {
			os.Exit(1)
		}
	}()

	clock := clockwork.NewRealClock()
	return telemetry.NewPeriodicDriver(&reportGenerator{
		clock:         clock,
		snmpStruct:    snmpStruct,
		snmpMetrics:   snmpMetrics,
		operStatusMap: make(map[string]*telemetrypb.IfOperStatus),
		ctx:           ctx,
	}, clock, collectionPeriod)
}

func (rg *reportGenerator) Stats() interface{} {
	log := zerolog.Ctx(rg.ctx).With().Str("driver", "snmp").Logger()

	err := rg.snmpStruct.Connect()
	if err != nil {
		log.Error().Msgf("Connect() err: %v", err)
		return nil
	} else {
		log.Debug().Msg("CONNECTED!")
	}
	defer rg.snmpStruct.Conn.Close()

	metricResults, err := rg.getMetrics(&log)

	log.Debug().Msgf("Retrieved %d result(s)", len(metricResults))

	return metricResults
}

func (rg *reportGenerator) getMetrics(log *zerolog.Logger) (metricResults map[string]*snmp.MetricValue, err error) {
	// metricResults is map from oid to MetricValue object
	metricResults = make(map[string]*snmp.MetricValue)
	oids := []string{}
	for _, metric := range rg.snmpMetrics {
		metricResults[metric.Oid] = &snmp.MetricValue{Metric: metric}
		oids = append(oids, metric.Oid)
	}

	log.Debug().Msg("Call SNMP get")
	result, err2 := rg.snmpStruct.Get(oids)
	if err2 != nil {
		log.Error().Msgf("Get() err: %v", err2)
		return metricResults, err2
	}

	for _, variable := range result.Variables {
		log.Debug().Msgf("Got %+v", variable)
		metricValue := metricResults[variable.Name]

		// Update the Prom gauge with the new SNMP retrieved values.
		promMetric := promMetrics[metricValue.DisplayName]

		switch variable.Type {
		case gosnmp.OctetString:
			bytes := variable.Value.([]byte)

			// Convert string metric to a float64.
			floatValue := convertStringMetric(string(bytes))
			log.Debug().Msgf("----------Convert string to float: %v", floatValue)
			setFloatMetric(floatValue, metricValue, promMetric, log)

		case gosnmp.NoSuchInstance:
			log.Error().Msgf("NoSuchInstance for %v", variable)
		case gosnmp.NoSuchObject:
			log.Error().Msgf("NoSuchObject for %v", variable)
		default:
			// Update the Prom gauge with the new SNMP retrieved values
			floatValue, _ := gosnmp.ToBigInt(variable.Value).Float64()
			log.Debug().Msgf("----------Convert integer to float: %v", floatValue)
			setFloatMetric(floatValue, metricValue, promMetric, log)
		}
	}
	return metricResults, nil
}

func setFloatMetric(floatValue float64, metricValue *snmp.MetricValue, promMetric *prometheus.Gauge,
	log *zerolog.Logger,
) {
	if metricValue.AlertConstraint != nil && metricValue.AlertConstraint.ShiftMetric != nil {
		log.Info().Msgf("Shift %s value %v by %v", metricValue.DisplayName, floatValue,
			*metricValue.AlertConstraint.ShiftMetric)
		floatValue += *metricValue.AlertConstraint.ShiftMetric
	}
	metricValue.Value = floatValue
	(*promMetric).Set(floatValue)
}

// shiftMetric is used in development/debug situations where we want to manually shift
// the value of a given metric.
func shiftMetric(metricValue *snmp.MetricValue, log *zerolog.Logger) {
	if metricValue.AlertConstraint != nil && metricValue.AlertConstraint.ShiftMetric != nil {
		log.Info().Msgf("Shift %s value %v by %v", metricValue.DisplayName, metricValue.Value,
			*metricValue.AlertConstraint.ShiftMetric)
		metricValue.Value += *metricValue.AlertConstraint.ShiftMetric
	} else {
		log.Info().Msgf("NOT shift %s value %v. Constraint: %v", metricValue.DisplayName, metricValue.Value,
			metricValue.AlertConstraint)
	}
}

func (rg *reportGenerator) runBulkWalk(log *zerolog.Logger) {
	log.Info().Msg("\n\n******** BulkWalk **********\n")
	err := rg.snmpStruct.BulkWalk("1.3.6.1.4.1.29890.1.5.3.5", printValue)
	if err != nil {
		log.Error().Msgf("Walk Error: %v\n", err)
		os.Exit(1)
	}
}

func convertStringMetric(stringValue string) float64 {
	// Some SNMP device OIDs report numeric values as strings with units, like "26.4 C deg" or
	// "1200 milliVolts". Assuming that the units never change (i.e. a given OID always reports the
	// same units instead of for ex switching from milliVolts to Volts when > 1000), then we can simply
	// strip the non-numeric values and assume that the units are the default.
	// If the above proves not true, we'll need to add some logic to do some conversions, such as
	// dividing by 1000 if we see "milli", for ex.  The problem there though is that some manufacturers
	// might use "mV" and others "milliVolt", so this would lead to many permutations. Hopefully, the units
	// stay consistent.
	// Define a regex to match non-numeric characters
	re := regexp.MustCompile(`[^0-9.]`)

	// Replace all matched characters with an empty string
	filteredStringValue := re.ReplaceAllString(stringValue, "")
	numericValue, _ := strconv.ParseFloat(filteredStringValue, 64)
	return numericValue
}

func printValue(pdu gosnmp.SnmpPDU) error {
	fmt.Printf("(pdu.Name=Value) %s = ", pdu.Name)

	switch pdu.Type {
	case gosnmp.OctetString:
		b := pdu.Value.([]byte)
		fmt.Printf("STRING: %s\n", string(b))
	default:
		fmt.Printf("TYPE %d: %d\n", pdu.Type, gosnmp.ToBigInt(pdu.Value))
	}
	return nil
}

// Note this is just for telemetryfe feedback (not used for Grafana display).
func (rg *reportGenerator) GenerateReport(ctx context.Context, nodeID string) (*telemetrypb.ExportMetricsRequest, error) {
	log := zerolog.Ctx(ctx).With().Str("driver", "snmp").Logger()
	ts := rg.clock.Now()
	log.Debug().Msgf("running telemetry command for node %s", nodeID)

	// This function is called periodically by the agent, and so the SNMP metrics are retrieved from the network device(s) anew
	// Calling this manually triggers the prometheus gauges to be updated so when the /metric endpoints are later scraped,
	// metrics data has been updated.
	metricResults := rg.Stats().(map[string]*snmp.MetricValue)
	operStatus := telemetrypb.IfOperStatus_IF_OPER_STATUS_UNKNOWN.Enum()
	constraintInterfaceID := ""
	constraintNodeID := ""

	// We can't immediately set the InterfaceMetrics with status following each metric as we loop
	// because we might get a DOWN and then an UP for the same nodeKey (like if a constraint
	// exceeds the max but a previously-failing constraint starts succeeding). This will hold
	// the final InterfaceMetrics (with the final status) that we need.
	nodeInterfaceKeyToInterfaceMetricsMap := make(map[string]*telemetrypb.InterfaceMetrics, 0)

	for metricOid, metricValue := range metricResults {
		if metricValue.AlertConstraint != nil {
			constraint := *metricValue.AlertConstraint
			metricName := metricValue.DisplayName
			constraintInterfaceID = metricValue.AlertConstraint.InterfaceID
			constraintNodeID = metricValue.AlertConstraint.NodeID

			// This node key is later parsed by TelemtryFE to extract the constraint node ID and the
			// node ID that comes from sdn_agent (which is passed into this function).
			nodeKey := getNodeKey(constraintNodeID, nodeID)

			// nodeInterfaceKey is used as the key for a map of statuses which should have independent
			// values for each constraintNodeID (e.g. midland_gw) and interface (e.g. FEEDER_GS_0)
			nodeInterfaceKey := getNodeInterfaceKey(constraintNodeID, constraintInterfaceID)

			log.Debug().Msgf("%s: MIN nil %v, MAX nil %v", metricName, constraint.Min == nil, constraint.Max == nil)

			if constraint.Min != nil && *constraint.Min > metricValue.Value {
				log.Warn().Msgf("%s: min value %v but is %v", metricName, *constraint.Min, metricValue.Value)
				operStatus = telemetrypb.IfOperStatus_IF_OPER_STATUS_DOWN.Enum()
			} else if constraint.Max != nil && *constraint.Max < metricValue.Value {
				log.Warn().Msgf("%s: max value %v but is %v", metricName, *constraint.Max, metricValue.Value)
				operStatus = telemetrypb.IfOperStatus_IF_OPER_STATUS_DOWN.Enum()
			} else {
				log.Info().Msgf("%s: value %v passes constraints", metricName, metricValue.Value)
				operStatus = telemetrypb.IfOperStatus_IF_OPER_STATUS_UP.Enum()
			}

			oldOperStatus, _ := rg.operStatusMap[metricOid]
			log.Debug().Msgf("last status %v ; new status  %v", oldOperStatus, operStatus)

			if constraint.TriggerIfUnchanged || oldOperStatus == nil || *operStatus != *oldOperStatus {
				log.Debug().Msgf("%s: sending status. constraint.TriggerIfUnchanged: %v, oldStatus: %v, newStatus: %v ",
					metricName, constraint.TriggerIfUnchanged, oldOperStatus, operStatus)
				rg.operStatusMap[metricOid] = operStatus

				// Check if, in this same function call, we already had a requested operational status
				// update request for this nodeInterfaceKey.  If so, logically combine those (giving precendence
				// to failure conditions) to get the final status (for example, up + down = down).
				// Because of this, we never send more than one value in the OperationalStateDataPoints list.
				previousInterfaceMetrics, ok := nodeInterfaceKeyToInterfaceMetricsMap[nodeInterfaceKey]
				addNewInterfaceMetrics := true
				if ok {
					previousStatus := previousInterfaceMetrics.GetOperationalStateDataPoints()[0].GetValue()
					log.Info().Msgf("%s: nodeInterfaceKey %s: previous %s latest %s",
						metricName, nodeInterfaceKey, previousStatus, *operStatus)
					operStatus = combineStatuses(&previousStatus, operStatus)
					log.Info().Msgf("%s: ===> the above reduced to: %s", metricName, *operStatus)

					// If the new operStatus matches the old, we don't have to do anything because that value is already
					// in the map.
					if *operStatus == previousStatus {
						addNewInterfaceMetrics = false
					}
				}

				// If addNewInterfaceMetrics, then we need to add a new InterfaceMetrics object to the map.
				if addNewInterfaceMetrics {
					textNetIfaceID, err := prototext.Marshal(&apipb.NetworkInterfaceId{
						NodeId:      proto.String(nodeKey),
						InterfaceId: proto.String(constraintInterfaceID),
					})
					if err != nil {
						log.Err(err).Msg("marshalling textproto interface ID")
						return nil, nil
					}

					nodeInterfaceKeyToInterfaceMetricsMap[nodeInterfaceKey] = &telemetrypb.InterfaceMetrics{
						InterfaceId: proto.String(string(textNetIfaceID)),
						OperationalStateDataPoints: []*telemetrypb.IfOperStatusDataPoint{{
							Time:  timestamppb.New(ts),
							Value: operStatus,
						}},
						StandardInterfaceStatisticsDataPoints: []*telemetrypb.StandardInterfaceStatisticsDataPoint{{}},
					}
				}
			} else {
				log.Info().Msgf("%s: NOT sending status. constraint.TriggerIfUnchanged: %v, oldStatus: %v, newStatus: %v ",
					metricName, constraint.TriggerIfUnchanged, *oldOperStatus, *operStatus)
			}
		}
	}

	// If no status updates, just return.
	if len(nodeInterfaceKeyToInterfaceMetricsMap) == 0 {
		return nil, nil
	}

	for _, value := range nodeInterfaceKeyToInterfaceMetricsMap {
		log.Info().Msgf("**** SENDING STATUS **** %v", value)
	}

	return &telemetrypb.ExportMetricsRequest{
		InterfaceMetrics: slices.Collect(maps.Values(nodeInterfaceKeyToInterfaceMetricsMap)),
	}, nil
}

func getNodeKey(constraintNodeID, sdnAgentNodeID string) string {
	return fmt.Sprintf("%s+%s", constraintNodeID, sdnAgentNodeID)
}

// nodeInterfaceKey is used as the key for a map of statuses which should have independent
// values for each (constraint) nodeID (e.g. midland_gw) and interface (e.g. FEEDER_GS_0)
func getNodeInterfaceKey(nodeID, interfaceID string) string {
	return fmt.Sprintf("%s+%s", nodeID, interfaceID)
}

// For 2 statuses to be set on the same value, combine them to get a single status. Any
// failure state combined with a success state is a failure state.
func combineStatuses(status1, status2 *telemetrypb.IfOperStatus) *telemetrypb.IfOperStatus {
	if status1 == nil {
		return status2
	}
	if status2 == nil {
		return status1
	}
	if *status1 == *status2 {
		return status1
	}
	if isFailureStatus(status1) {
		return status1
	}
	if isFailureStatus(status2) {
		return status2
	}

	// So both statuses are either UP, UNSPECIFIED, or UNKNOWN. Let's consider the status to
	// be UP if either is UP.
	if *status1 == telemetrypb.IfOperStatus_IF_OPER_STATUS_UP {
		return status1
	}
	return status2
}

func isFailureStatus(status *telemetrypb.IfOperStatus) bool {
	return status != nil && (*status == telemetrypb.IfOperStatus_IF_OPER_STATUS_DOWN ||
		*status == telemetrypb.IfOperStatus_IF_OPER_STATUS_LOWER_LAYER_DOWN ||
		*status == telemetrypb.IfOperStatus_IF_OPER_STATUS_NOT_PRESENT ||
		*status == telemetrypb.IfOperStatus_IF_OPER_STATUS_DORMANT ||
		*status == telemetrypb.IfOperStatus_IF_OPER_STATUS_TESTING)
}
