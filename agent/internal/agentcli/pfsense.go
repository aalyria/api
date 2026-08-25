// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
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

package agentcli

import (
	"context"
	"time"

	"aalyria.com/spacetime/agent/enactment"
	enactment_pfsense "aalyria.com/spacetime/agent/enactment/pfsense"
	"aalyria.com/spacetime/agent/internal/configpb"
	shared_pfsense "aalyria.com/spacetime/agent/internal/pfsense"
	"aalyria.com/spacetime/agent/telemetry"
	telemetry_pfsense "aalyria.com/spacetime/agent/telemetry/pfsense"

	"github.com/jonboulle/clockwork"
)

func newPfSenseEnactmentDriver(ctx context.Context, clock clockwork.Clock, nodeID string, conf *configpb.SdnAgent_PfSenseEnactment) (enactment.Driver, error) {
	config := enactment_pfsense.Config{
		Clock:              clock,
		APIEndpoint:        conf.GetApiEndpoint(),
		Username:           conf.GetUsername(),
		APIKey:             conf.GetApiKey(),
		Timeout:            conf.GetTimeout().AsDuration(),
		Retries:            int(conf.GetRetries()),
		InsecureSkipVerify: conf.GetInsecureSkipVerify(),
	}

	// Set defaults if not provided
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.Retries == 0 {
		config.Retries = 3
	}

	return enactment_pfsense.New(config), nil
}

func newPfSenseTelemetryDriver(ctx context.Context, clock clockwork.Clock, nodeID string, conf *configpb.SdnAgent_PfSenseTelemetry) (telemetry.Driver, error) {
	// Create shared API client configuration
	apiConfig := shared_pfsense.Config{
		APIEndpoint:        conf.GetApiEndpoint(),
		Username:           conf.GetUsername(),
		APIKey:             conf.GetApiKey(),
		Timeout:            conf.GetTimeout().AsDuration(),
		Retries:            int(conf.GetRetries()),
		InsecureSkipVerify: conf.GetInsecureSkipVerify(),
	}

	// Set defaults if not provided
	if apiConfig.Timeout == 0 {
		apiConfig.Timeout = 30 * time.Second
	}
	if apiConfig.Retries == 0 {
		apiConfig.Retries = 3
	}

	collectionPeriod := conf.GetCollectionPeriod().AsDuration()
	if collectionPeriod == 0 {
		collectionPeriod = 60 * time.Second // Default to 1 minute
	}

	return telemetry_pfsense.NewDriver(apiConfig, collectionPeriod), nil
}
