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
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	apipb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/enactment/netlink"

	"github.com/jonboulle/clockwork"
	vnl "github.com/vishvananda/netlink"
)

const (
	rtTableID             = 252
	rtTableLookupPriority = 25200
)

// setupInterface creates a dummy interface in the containter environment
// which can be used to simulate the behaviour of real Linux network interfaces
func setupInterface(nlHandle *vnl.Handle, ifaceIndex int, ifaceName string, ifaceIP string) {
	// Generate synthetic link environment for test
	link := vnl.Dummy{vnl.LinkAttrs{Index: ifaceIndex, Name: ifaceName}}
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
func formatFlowState(flowState *apipb.ControlPlaneState) string {
	fwdState := flowState.GetForwardingState()

	var b strings.Builder
	fmt.Fprintf(&b, "forwarding_state:{timestamp:{%v}", fwdState.GetTimestamp())
	flowRuleIDs := fwdState.GetFlowRuleIds()
	sort.Sort(sort.Reverse(sort.StringSlice(flowRuleIDs)))
	for _, flow_rule_id := range flowRuleIDs {
		fmt.Fprintf(&b, " flow_rule_ids:\"%v\"", flow_rule_id)
	}
	fmt.Fprintf(&b, "}")

	return b.String()
}

// printTestState outputs a string in format to be consumed by the test runner
// and compared against expected output in the YAML test definition file.
func printTestState(testName string, newState *apipb.ControlPlaneState, err error) {
	fmt.Printf("%v %v, err: %v\n", testName, formatFlowState(newState), err)
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
	updateId := "64988bc4-d401-428a-b7be-926bebb5f832"

	newState, err := eb.Apply(ctx, &apipb.ScheduledControlUpdate{
		UpdateId: &updateId,
		Change: &apipb.ControlPlaneUpdate{
			UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
				FlowUpdate: &apipb.FlowUpdate{
					FlowRuleId: proto.String("zulu1"),
					Operation:  apipb.FlowUpdate_ADD.Enum(),
					Rule: &apipb.FlowRule{
						Classifier: &apipb.PacketClassifier{
							IpHeader: &apipb.PacketClassifier_IpHeader{
								SrcIpRange: proto.String("216.58.201.110"),   // might be deprecated
								DstIpRange: proto.String("104.198.75.23/32"), // IP of the GCP ingress
							},
						},
						ActionBucket: []*apipb.FlowRule_ActionBucket{
							{
								Action: []*apipb.FlowRule_ActionBucket_Action{
									{
										ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
											Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
												OutInterfaceId: proto.String("ens224"),           // local OneWeb data plane VLAN iface
												NextHopIp:      proto.String("192.168.200.1/32"), // dummy IP of OneWeb UT
											},
										},
									},
								},
							},
						},
					},
					SequenceNumber: proto.Int64(1),
				},
			},
		},
	})

	printTestState(testName, newState, err)

	testName = "TST2"
	fmt.Printf("\nImplement zulu2 FlowRuleID for destination 104.198.75.23. Zulu1 already implements the route so zulu2 FlowRuleID is just cached\n")
	updateId = "64288bc3-d401-428a-b7be-926bebb5f832"

	newState, err = eb.Apply(ctx, &apipb.ScheduledControlUpdate{
		UpdateId: &updateId,
		Change: &apipb.ControlPlaneUpdate{
			UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
				FlowUpdate: &apipb.FlowUpdate{
					FlowRuleId: proto.String("zulu2"),
					Operation:  apipb.FlowUpdate_ADD.Enum(),
					Rule: &apipb.FlowRule{
						Classifier: &apipb.PacketClassifier{
							IpHeader: &apipb.PacketClassifier_IpHeader{
								SrcIpRange: proto.String("216.58.201.110"),   // might be deprecated
								DstIpRange: proto.String("104.198.75.23/32"), // IP of the GCP ingress
							},
						},
						ActionBucket: []*apipb.FlowRule_ActionBucket{
							{
								Action: []*apipb.FlowRule_ActionBucket_Action{
									{
										ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
											Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
												OutInterfaceId: proto.String("ens224"),           // local OneWeb data plane VLAN iface
												NextHopIp:      proto.String("192.168.200.1/32"), // dummy IP of OneWeb UT
											},
										},
									},
								},
							},
						},
					},
					SequenceNumber: proto.Int64(1),
				},
			},
		},
	})

	printTestState(testName, newState, err)

	testName = "TST3"
	fmt.Printf("\nManually delete the route to 104.198.75.23 designated by zulu1 and zulu2\n\n")
	cmd := exec.Command("ip", "route", "del", "104.198.75.23", "table", strconv.Itoa(rtTableID))

	output, err := cmd.Output()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("\nImplement zulu3 FlowRuleID for destination 104.198.75.23. Without syncing, cdpi_agent would think this is already implemented, and the route ADD would be skipped, leaving us without a route\n")
	updateId = "64288bc4-d401-428a-b7be-926bebb5f832"

	newState, err = eb.Apply(ctx, &apipb.ScheduledControlUpdate{
		UpdateId: &updateId,
		Change: &apipb.ControlPlaneUpdate{
			UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
				FlowUpdate: &apipb.FlowUpdate{
					FlowRuleId: proto.String("zulu3"),
					Operation:  apipb.FlowUpdate_ADD.Enum(),
					Rule: &apipb.FlowRule{
						Classifier: &apipb.PacketClassifier{
							IpHeader: &apipb.PacketClassifier_IpHeader{
								SrcIpRange: proto.String("216.58.201.110"),   // might be deprecated
								DstIpRange: proto.String("104.198.75.23/32"), // IP of the GCP ingress
							},
						},
						ActionBucket: []*apipb.FlowRule_ActionBucket{
							{
								Action: []*apipb.FlowRule_ActionBucket_Action{
									{
										ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
											Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
												OutInterfaceId: proto.String("ens224"),           // local OneWeb data plane VLAN iface
												NextHopIp:      proto.String("192.168.200.1/32"), // dummy IP of OneWeb UT
											},
										},
									},
								},
							},
						},
					},
					SequenceNumber: proto.Int64(1),
				},
			},
		},
	})

	printTestState(testName, newState, err)

	testName = "TST4"
	fmt.Printf("\nImplement dbvr2 FlowRuleID to destination 34.135.90.47 (different route to zuluX)\n")
	updateId = "9bac075f-9560-494e-b925-56f6c88a9d23"

	newState, err = eb.Apply(ctx, &apipb.ScheduledControlUpdate{
		UpdateId: &updateId,
		Change: &apipb.ControlPlaneUpdate{
			UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
				FlowUpdate: &apipb.FlowUpdate{
					FlowRuleId: proto.String("dbvr2"),
					Operation:  apipb.FlowUpdate_ADD.Enum(),
					Rule: &apipb.FlowRule{
						Classifier: &apipb.PacketClassifier{
							IpHeader: &apipb.PacketClassifier_IpHeader{
								SrcIpRange: proto.String("216.58.201.110"),  // might be deprecated
								DstIpRange: proto.String("34.135.90.47/32"), // IP of the GCP ingress
							},
						},
						ActionBucket: []*apipb.FlowRule_ActionBucket{
							{
								Action: []*apipb.FlowRule_ActionBucket_Action{
									{
										ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
											Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
												OutInterfaceId: proto.String("ens224"),           // local ViaSat VLAN iface
												NextHopIp:      proto.String("192.168.200.1/32"), // dummy IP of ViaSat UT/router
											},
										},
									},
								},
							},
						},
					},
					SequenceNumber: proto.Int64(3),
				},
			},
		},
	})

	printTestState(testName, newState, err)

	testName = "TST5"
	fmt.Printf("\nAttempt to delete FlowRuleID zulu1 (non-existant currently)\n")
	updateId = "a97b4dac-d216-45a5-9ed7-129c39ab0438"

	newState, err = eb.Apply(ctx, &apipb.ScheduledControlUpdate{
		UpdateId: &updateId,
		Change: &apipb.ControlPlaneUpdate{
			UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
				FlowUpdate: &apipb.FlowUpdate{
					FlowRuleId:     proto.String("zulu1"),
					Operation:      apipb.FlowUpdate_DELETE.Enum(),
					SequenceNumber: proto.Int64(2),
				},
			},
		},
	})

	printTestState(testName, newState, err)

	testName = "TST6"
	fmt.Printf("\nSpacetime reprovisions zulu1. Because zulu3 already implemented the route, zulu1's ADD is skipped but the FlowRuleID is cached")
	updateId = "a97s4dac-d216-45a5-9ed7-129c39ab0438"

	newState, err = eb.Apply(ctx, &apipb.ScheduledControlUpdate{
		UpdateId: &updateId,
		Change: &apipb.ControlPlaneUpdate{
			UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
				FlowUpdate: &apipb.FlowUpdate{
					FlowRuleId: proto.String("zulu1"),
					Operation:  apipb.FlowUpdate_ADD.Enum(),
					Rule: &apipb.FlowRule{
						Classifier: &apipb.PacketClassifier{
							IpHeader: &apipb.PacketClassifier_IpHeader{
								SrcIpRange: proto.String("216.58.201.110"),   // might be deprecated
								DstIpRange: proto.String("104.198.75.23/32"), // IP of the GCP ingress
							},
						},
						ActionBucket: []*apipb.FlowRule_ActionBucket{
							{
								Action: []*apipb.FlowRule_ActionBucket_Action{
									{
										ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
											Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
												OutInterfaceId: proto.String("ens224"),           // local OneWeb data plane VLAN iface
												NextHopIp:      proto.String("192.168.200.1/32"), // dummy IP of OneWeb UT
											},
										},
									},
								},
							},
						},
					},
					SequenceNumber: proto.Int64(1),
				},
			},
		},
	})

	printTestState(testName, newState, err)

	testName = "TST7"
	fmt.Printf("\nSpacetime reprovisions zulu2. Because zulu1 and zulu3 already implement the route, zulu2's ADD is skipped but the FlowRuleID is cached")
	updateId = "64288bc3-d401-428a-b7be-926bebb5f832"

	newState, err = eb.Apply(ctx, &apipb.ScheduledControlUpdate{
		UpdateId: &updateId,
		Change: &apipb.ControlPlaneUpdate{
			UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
				FlowUpdate: &apipb.FlowUpdate{
					FlowRuleId: proto.String("zulu2"),
					Operation:  apipb.FlowUpdate_ADD.Enum(),
					Rule: &apipb.FlowRule{
						Classifier: &apipb.PacketClassifier{
							IpHeader: &apipb.PacketClassifier_IpHeader{
								SrcIpRange: proto.String("216.58.201.110"),   // might be deprecated
								DstIpRange: proto.String("104.198.75.23/32"), // IP of the GCP ingress
							},
						},
						ActionBucket: []*apipb.FlowRule_ActionBucket{
							{
								Action: []*apipb.FlowRule_ActionBucket_Action{
									{
										ActionType: &apipb.FlowRule_ActionBucket_Action_Forward_{
											Forward: &apipb.FlowRule_ActionBucket_Action_Forward{
												OutInterfaceId: proto.String("ens224"),           // local OneWeb data plane VLAN iface
												NextHopIp:      proto.String("192.168.200.1/32"), // dummy IP of OneWeb UT
											},
										},
									},
								},
							},
						},
					},
					SequenceNumber: proto.Int64(1),
				},
			},
		},
	})

	printTestState(testName, newState, err)

	testName = "TST8"
	fmt.Printf("\nSpacetime provisions DELETE for zulu1. Because zulu2 and zulu3 still represent the route, the actual route is kept\n")
	updateId = "a97bsdac-d216-45a5-9ed7-129c39ab0438"

	newState, err = eb.Apply(ctx, &apipb.ScheduledControlUpdate{
		UpdateId: &updateId,
		Change: &apipb.ControlPlaneUpdate{
			UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
				FlowUpdate: &apipb.FlowUpdate{
					FlowRuleId:     proto.String("zulu1"),
					Operation:      apipb.FlowUpdate_DELETE.Enum(),
					SequenceNumber: proto.Int64(2),
				},
			},
		},
	})

	printTestState(testName, newState, err)

	testName = "TST9"
	fmt.Printf("\nSpacetime provisions DELETE for zulu2. Because zulu3 still represents the route, the actual route is kept\n")
	updateId = "a97b4dac-d216-45a5-9ed7-129c39ab0438"

	newState, err = eb.Apply(ctx, &apipb.ScheduledControlUpdate{
		UpdateId: &updateId,
		Change: &apipb.ControlPlaneUpdate{
			UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
				FlowUpdate: &apipb.FlowUpdate{
					FlowRuleId:     proto.String("zulu2"),
					Operation:      apipb.FlowUpdate_DELETE.Enum(),
					SequenceNumber: proto.Int64(2),
				},
			},
		},
	})

	printTestState(testName, newState, err)

	testName = "TST10"
	fmt.Printf("\nSpacetime provisions DELETE for zulu3. Since this is the last FlowRuleID representing the route to destination 104.198.75.23, the route too is deleted\n")
	updateId = "a97b4dac-d216-45a5-9ed7-129c39ab0438"

	newState, err = eb.Apply(ctx, &apipb.ScheduledControlUpdate{
		UpdateId: &updateId,
		Change: &apipb.ControlPlaneUpdate{
			UpdateType: &apipb.ControlPlaneUpdate_FlowUpdate{
				FlowUpdate: &apipb.FlowUpdate{
					FlowRuleId:     proto.String("zulu3"),
					Operation:      apipb.FlowUpdate_DELETE.Enum(),
					SequenceNumber: proto.Int64(2),
				},
			},
		},
	})

	printTestState(testName, newState, err)

	cmd = exec.Command("ip", "route", "show", "table", strconv.Itoa(rtTableID))

	output, err = cmd.Output()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("Last implemented route is FlowRuleID dbvr2 to destination 34.135.90.47\n%v", string(output))
}
