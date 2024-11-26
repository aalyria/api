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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jonboulle/clockwork"
	vnl "github.com/vishvananda/netlink"

	"aalyria.com/spacetime/agent/enactment/netlink"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
)

const (
	rtTableID             = 252
	rtTableLookupPriority = 25200
)

// setupInterface creates a dummy interface in the containter environment
// which can be used to simulate the behaviour of real Linux network interfaces
func setupInterface(nlHandle *vnl.Handle, ifaceIndex int, ifaceName string, ifaceIP string) {
	// Generate synthetic link environment for test
	link := vnl.Dummy{LinkAttrs: vnl.LinkAttrs{Index: ifaceIndex, Name: ifaceName}}
	err := nlHandle.LinkAdd(&link)
	if err != nil {
		log.Fatalf("nlHandle.LinkAdd(Dummy(%s)) failed with: %s", ifaceName, err)
	}
	addr, err := vnl.ParseAddr(ifaceIP)
	if err != nil {
		log.Fatalf("vnl.ParseAddr(%s) failure: %s", ifaceIP, err)
	}
	err = nlHandle.AddrAdd(&link, addr)
	if err != nil {
		log.Fatalf("nlHandle.AddrAdd(Dummy(%s), addr) failed: %s", ifaceName, err)
	}

	err = nlHandle.LinkSetUp(&link)
	if err != nil {
		log.Fatalf("nlHandle.LinkSetUp(Dummy(%s)) failed: %s", ifaceName, err)
	}
}

// prepopulateSpacetimeTableWithRoutes enters two "potentially conflicting routes to the route table: <tableID>,
// which are to be deleted by the netlink enactment backend before it starts adding routes to that table
func prepopulateSpacetimeTableWithRoutes(tableID int) {
	prepopRoutes := []string{"111.222.111.222/32", "222.111.222.111/32"}

	fmt.Printf("Attempting to prepopulate table %v with routes: %v\n", tableID, prepopRoutes)
	for _, dest := range prepopRoutes {
		cmd := exec.Command("ip", "route", "add", dest, "via", "192.168.200.1", "dev", "ens224", "table", strconv.Itoa(tableID))
		_, err := cmd.Output()
		if err != nil {
			log.Fatalf("Error prepopRoutes in table %v for destination %v with error: %v\n", tableID, dest, err)
		}

	}

	fmt.Printf("Successfully prepopulated table %v with routes %v\n", tableID, prepopRoutes)
}

// checkNetlinkIpRulePriorityJumpsToTable checks that the netlink enactmnent
// backedn config installed the expected ip rule at expected priority. The
// test YAML checks the output for a jump to a known table (ick).
//
// N.B.: rather than run "ip -X rule show priority NNN" this code walks the
// output of "ip -X rule show" looking for a line that begins with "NNN:".
// This is due to limitations with the stripped down /sbin/ip command in the
// container image use for this test.
//
// Checks both IPv4 and IPv6 rules.
func checkNetlinkIpRulePriorityJumpsToTable(priority int) {
	ipVersions := []string{"-4", "-6"}

	for _, ipVersion := range ipVersions {
		cmd := exec.Command("ip", ipVersion, "rule", "show")
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Fatalf("Error running '%v': %v (%v)\n", cmd.String(), strings.TrimSpace(err.Error()), string(output))
		} else {
			linePrefix := fmt.Sprintf("%v:", strconv.Itoa(priority))
			for _, line := range strings.Split(string(output), "\n") {
				if strings.HasPrefix(line, linePrefix) {
					fmt.Printf("ip %v rule: %v\n", ipVersion, line)
				}
			}
		}
	}
}

// formatFlowState implements a consistent string format for
// ControlPlaneState messages contain only a FlowState field.
func formatDriverState(d *netlink.Driver) string {
	data, err := json.Marshal(d.Stats())
	if err != nil {
		panic(fmt.Errorf("failed to marshal driver state as JSON: %w", err))
	}
	return string(data)
}

// printTestState outputs a string in format to be consumed by the test runner
// and compared against expected output in the YAML test definition file.
func printTestState(testName string, eb *netlink.Driver, err error) {
	fmt.Printf("%v %v, err: %v\n", testName, formatDriverState(eb), err)
}

func main() {
	nlHandle, err := vnl.NewHandle(vnl.FAMILY_ALL)
	if err != nil {
		log.Fatalf("netlink.NewHandle(\"test_namespace\", vnl.FAMILY_V4) failed with: %s", err)
	}

	setupInterface(nlHandle, 1000, "ens161", "192.168.100.2/24") // setting up the OneWeb control plane interface
	setupInterface(nlHandle, 1001, "ens224", "192.168.200.2/24") // setting up the OneWeb SD-One data plane interface
	setupInterface(nlHandle, 1002, "ens256", "192.168.1.2/24")   // setting up the Viasat interface
	setupInterface(nlHandle, 1003, "ens160", "172.16.51.1/24")   // setting up the ground mgmt interface

	ctx := context.Background()
	config := netlink.DefaultConfig(ctx, nlHandle, rtTableID, rtTableLookupPriority)
	config.Clock = clockwork.NewFakeClockAt(time.Date(1981, time.November, 0, 0, 0, 0, 0, time.UTC))

	prepopulateSpacetimeTableWithRoutes(rtTableID)

	eb := netlink.New(config)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := eb.Init(ctx); err != nil {
		log.Fatal("Failed to invoke Init() for enactment backend")
	}

	checkNetlinkIpRulePriorityJumpsToTable(rtTableLookupPriority)

	testName := "TST1"
	fmt.Printf("\nImplement zulu1 FlowRuleID for destination 104.198.75.23\n")

	err = eb.Dispatch(ctx, &schedpb.CreateEntryRequest{
		Id:    "zulu1",
		Seqno: 1,
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "216.58.201.110",
				To:   "104.198.75.23/32",
				Dev:  "ens224",
				Via:  "192.168.200.1",
			},
		},
	})

	printTestState(testName, eb, err)

	testName = "TST2"
	fmt.Printf("\nImplement zulu2 FlowRuleID for destination 104.198.75.23. Zulu1 already implements the route so zulu2 FlowRuleID is just cached\n")

	err = eb.Dispatch(ctx, &schedpb.CreateEntryRequest{
		Id:    "zulu2",
		Seqno: 2,
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "216.58.201.110",
				To:   "104.198.75.23/32",
				Dev:  "ens224",
				Via:  "192.168.200.1",
			},
		},
	})

	printTestState(testName, eb, err)

	testName = "TST3"
	fmt.Printf("\nManually delete the route to 104.198.75.23 designated by zulu1 and zulu2.\n\n")
	cmd := exec.Command("ip", "route", "del", "104.198.75.23", "table", strconv.Itoa(rtTableID))

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error:", err, "output", string(output))
		return
	}

	fmt.Printf("\nImplement zulu3 FlowRuleID for destination 104.198.75.23. This should cause the driver to re-establish the zulu1 / zulu2 route.")

	err = eb.Dispatch(ctx, &schedpb.CreateEntryRequest{
		Id:    "zulu3",
		Seqno: 3,
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "216.58.201.110",
				To:   "104.198.75.23/32",
				Dev:  "ens224",
				Via:  "192.168.200.1",
			},
		},
	})

	printTestState(testName, eb, err)

	testName = "TST4"
	fmt.Printf("\nImplement dbvr2 FlowRuleID to destination 34.135.90.47 (different route to zuluX)\n")

	err = eb.Dispatch(ctx, &schedpb.CreateEntryRequest{
		Id:    "dvbr2",
		Seqno: 4,
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "216.58.201.110",
				To:   "34.135.90.47/32",
				Dev:  "ens224",
				Via:  "192.168.200.1",
			},
		},
	})

	printTestState(testName, eb, err)

	testName = "TST5"
	fmt.Printf("\nAttempt to delete FlowRuleID zulu1 (non-existent currently)\n")

	err = eb.Dispatch(ctx, &schedpb.CreateEntryRequest{
		Id:    "zulu1",
		Seqno: 5,
		ConfigurationChange: &schedpb.CreateEntryRequest_DeleteRoute{
			DeleteRoute: &schedpb.DeleteRoute{
				From: "216.58.201.110",
				To:   "34.135.90.47/32",
			},
		},
	})

	printTestState(testName, eb, err)

	testName = "TST6"
	fmt.Printf("\nSpacetime reprovisions zulu1. Because zulu3 already implemented the route, zulu1's ADD is skipped but the FlowRuleID is cached\n")

	err = eb.Dispatch(ctx, &schedpb.CreateEntryRequest{
		Id:    "zulu1",
		Seqno: 6,
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "216.58.201.110",
				To:   "104.198.75.23/32",
				Dev:  "ens224",
				Via:  "192.168.200.1",
			},
		},
	})

	printTestState(testName, eb, err)

	testName = "TST7"
	fmt.Printf("\nSpacetime reprovisions zulu2. Because zulu1 and zulu3 already implement the route, zulu2's ADD is skipped but the FlowRuleID is cached\n")

	err = eb.Dispatch(ctx, &schedpb.CreateEntryRequest{
		Id:    "zulu2",
		Seqno: 7,
		ConfigurationChange: &schedpb.CreateEntryRequest_SetRoute{
			SetRoute: &schedpb.SetRoute{
				From: "216.58.201.110",
				To:   "104.198.75.23/32",
				Dev:  "ens224",
				Via:  "192.168.200.1",
			},
		},
	})

	printTestState(testName, eb, err)

	testName = "TST8"
	fmt.Printf("\nSpacetime provisions DELETE for zulu1. Because zulu2 and zulu3 still represent the route, the actual route is kept\n")

	err = eb.Dispatch(ctx, &schedpb.CreateEntryRequest{
		Id:    "zulu1",
		Seqno: 8,
		ConfigurationChange: &schedpb.CreateEntryRequest_DeleteRoute{
			DeleteRoute: &schedpb.DeleteRoute{
				From: "216.58.201.110",
				To:   "104.198.75.23/32",
			},
		},
	})

	printTestState(testName, eb, err)

	testName = "TST9"
	fmt.Printf("\nSpacetime provisions DELETE for zulu2. Because zulu3 still represents the route, the actual route is kept\n")

	err = eb.Dispatch(ctx, &schedpb.CreateEntryRequest{
		Id:    "zulu2",
		Seqno: 9,
		ConfigurationChange: &schedpb.CreateEntryRequest_DeleteRoute{
			DeleteRoute: &schedpb.DeleteRoute{
				From: "216.58.201.110",
				To:   "104.198.75.23/32",
			},
		},
	})

	printTestState(testName, eb, err)

	testName = "TST10"
	fmt.Printf("\nSpacetime provisions DELETE for zulu3. Since this is the last FlowRuleID representing the route to destination 104.198.75.23, the route too is deleted\n")

	err = eb.Dispatch(ctx, &schedpb.CreateEntryRequest{
		Id:    "zulu3",
		Seqno: 10,
		ConfigurationChange: &schedpb.CreateEntryRequest_DeleteRoute{
			DeleteRoute: &schedpb.DeleteRoute{
				From: "216.58.201.110",
				To:   "104.198.75.23/32",
			},
		},
	})

	printTestState(testName, eb, err)

	cmd = exec.Command("ip", "route", "show", "table", strconv.Itoa(rtTableID))

	output, err = cmd.Output()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("Last implemented route is FlowRuleID dbvr2 to destination 34.135.90.47\n%v", string(output))
}
