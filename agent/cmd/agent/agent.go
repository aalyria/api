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

// Package main provides a CDPI agent that is configured using a protobuf-based
// manifest.
package main

import (
	"context"
	"fmt"
	"os"

	"aalyria.com/spacetime/agent/internal/agentcli"
)

func main() {
	if err := (agentcli.AgentConf{
		AppName:   "agent",
		Handles:   agentcli.DefaultHandles(),
		Providers: []agentcli.Provider{},
	}).Run(context.Background(), os.Args[0], os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "fatal error: %v\n", err)
		os.Exit(2)
	}
}
