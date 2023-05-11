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

package agent

import (
	"errors"
	"testing"
)

func TestAgentValidation_noOptions(t *testing.T) {
	_, err := NewAgent()
	if err == nil {
		t.Errorf("expected NewAgent with no options to be invalid")
	}
}

func TestAgentValidation_onlyClock(t *testing.T) {
	_, err := NewAgent(WithRealClock())
	if !errors.Is(err, errNoEndpoint) {
		t.Errorf("expected NewAgent with no endpoint to cause %s, but got %v error instead", errNoEndpoint, err)
	}
}

func TestAgentValidation_onlyEndpoint(t *testing.T) {
	_, err := NewAgent(WithServerEndpoint("dns:///example.com:443"))
	if !errors.Is(err, errNoClock) {
		t.Errorf("expected NewAgent with no clock to cause %s, but got %v error instead", errNoClock, err)
	}
}

func TestAgentValidation_noNodes(t *testing.T) {
	_, err := NewAgent(
		WithServerEndpoint("dns:///example.com:443"),
		WithRealClock())

	if !errors.Is(err, errNoNodes) {
		t.Errorf("expected NewAgent with no nodes to cause %s, but got %v error instead", errNoNodes, err)
	}
}

func TestAgentValidation_noServices(t *testing.T) {
	_, err := NewAgent(
		WithServerEndpoint("dns:///example.com:443"),
		WithRealClock(),
		WithNode("a"),
		WithNode("b"),
	)

	if !errors.Is(err, errNoActiveServices) {
		t.Errorf("expected NewAgent with no nodes to cause %s, but got %v error instead", errNoActiveServices, err)
	}
}
