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
	"fmt"

	apipb "aalyria.com/spacetime/api/common"
	"google.golang.org/protobuf/proto"
)

// NoChangeSpecifiedError indicates that there is no Change supplied in the apipb.SCU
type NoChangeSpecifiedError struct {
	req *apipb.ScheduledControlUpdate
}

func (e *NoChangeSpecifiedError) Error() string {
	return fmt.Sprintf("ScheduledControlUpdate received with no Change specified: %v", e.req)
}

func (e *NoChangeSpecifiedError) Is(err error) bool {
	if typedErr, ok := err.(*NoChangeSpecifiedError); ok {
		return proto.Equal(typedErr.req, e.req)
	}
	return false
}

///////////////////////////////////////////////////////////////////////////////////////////////

// UnsupportedUpdateError indicates the UpdateType specified in the apipb.SCU is unsupported
// Dec7: apipb.ControlPlaneUpdate_FlowUpdate is supported
type UnsupportedUpdateError struct {
	req *apipb.ScheduledControlUpdate
}

func (e *UnsupportedUpdateError) Error() string {
	return fmt.Sprintf("Attempted to implement unsupported update type: %v on update id: %s", e.req.Change.UpdateType, *e.req.UpdateId)
}

func (e *UnsupportedUpdateError) Is(err error) bool {
	if typedErr, ok := err.(*UnsupportedUpdateError); ok {
		return proto.Equal(e.req, typedErr.req)
	}
	return false
}

///////////////////////////////////////////////////////////////////////////////////////////////

// UnknownFlowRuleDeleteError indicates an unknown FlowRule is attempted to be deleted
type UnknownFlowRuleDeleteError struct {
	flowRuleId string
}

func (e *UnknownFlowRuleDeleteError) Error() string {
	return fmt.Sprintf("Attempted to DELETE unknown flowRuleID: %s", e.flowRuleId)
}

func (e *UnknownFlowRuleDeleteError) Is(err error) bool {
	if typedErr, ok := err.(*UnknownFlowRuleDeleteError); ok {
		return typedErr.flowRuleId == e.flowRuleId
	}
	return false
}

///////////////////////////////////////////////////////////////////////////////////////////////

// UnrecognizedFlowUpdateOperationError indicates an unrecognized FlowUpdate Operation is attempted to be applied
// Dec7: apipb.FlowUpdate_ADD and apipb.FlowUpdate_DELETE are supported
type UnrecognizedFlowUpdateOperationError struct {
	operation apipb.FlowUpdate_Operation
}

func (e *UnrecognizedFlowUpdateOperationError) Error() string {
	return fmt.Sprintf("Attempted unrecognized FlowUpdate operation (%s)", e.operation)
}

func (e *UnrecognizedFlowUpdateOperationError) Is(err error) bool {
	if typedErr, ok := err.(*UnrecognizedFlowUpdateOperationError); ok {
		return e.operation == typedErr.operation
	}
	return false
}

///////////////////////////////////////////////////////////////////////////////////////////////

// UnsupportedActionTypeError indicates an unsupported ActionType is attempted to be applied
// Dec7: FlowRule_ActionBucket_Action_Forward_ is supported
//
// TO DO: Expand error and code in netlink.go to return errors if unsupported Action Types
// such as PopHeader are passed
type UnsupportedActionTypeError struct{}

func (e *UnsupportedActionTypeError) Error() string {
	return fmt.Sprintf("Supplied no supported Actions of Forward Action Types")
}

func (e *UnsupportedActionTypeError) Is(err error) bool {
	_, ok := err.(*UnsupportedActionTypeError)
	return ok
}

///////////////////////////////////////////////////////////////////////////////////////////////

// IPv4FormattingError indicates an erroneously formatted IPv4 address was passed
type IPv4Entry string

const (
	SrcIpRange_Ip IPv4Entry = "SrcIpRange"
	DstIpRange_Ip IPv4Entry = "DstIpRange"
	NextHopIp_Ip  IPv4Entry = "NextHopIp"
)

type IPv4FormattingError struct {
	ipv4        string
	sourceField IPv4Entry
}

func (e IPv4FormattingError) Error() string {
	return fmt.Sprintf("Attempted using wrongly formatted IPv4 address/range (%s) for %s field", e.ipv4, e.sourceField)
}

func (e IPv4FormattingError) Is(err error) bool {
	if typedErr, ok := err.(IPv4FormattingError); ok {
		return typedErr.sourceField == e.sourceField && typedErr.ipv4 == e.ipv4
	}
	return false
}

///////////////////////////////////////////////////////////////////////////////////////////////

// FlowRuleMatchError indicates a case where the supplied FlowRule src/dstIpRange and those implemented by Netlink don't match
type FlowRuleMatchError struct {
	ipv4        string
	sourceField IPv4Entry
}

func (e FlowRuleMatchError) Error() string {
	return fmt.Sprintf("Error occurred when attempting to match FLowRule and installed %v, with IP %v", e.sourceField, e.ipv4)
}

func (e FlowRuleMatchError) Is(err error) bool {
	if typedErr, ok := err.(FlowRuleMatchError); ok {
		return typedErr.sourceField == e.sourceField && typedErr.ipv4 == e.ipv4
	}
	return false
}

///////////////////////////////////////////////////////////////////////////////////////////////

// OutInterfaceIdxError indicates an erroneously supplied outbound network interface
type OutInterfaceIdxError struct {
	wrongIface  string
	sourceError error
}

func (e OutInterfaceIdxError) Error() string {
	return fmt.Sprintf("Attempted using erroneous OutInterfaceId (%s) field with error (%v)", e.wrongIface, e.sourceError)
}

func (e OutInterfaceIdxError) Unwrap() error {
	return e.sourceError
}

func (e OutInterfaceIdxError) Is(err error) bool {
	if typedErr, ok := err.(OutInterfaceIdxError); ok {
		return typedErr.wrongIface == e.wrongIface
	}
	return false
}

///////////////////////////////////////////////////////////////////////////////////////////////

// ClassifierError indicates an erroneously supplied outbound network interface
type ClassifierField string

const (
	IpHeader_Field   ClassifierField = "IpHeader"
	Protocol_Field   ClassifierField = "Protocol"
	SrcIpRange_Field ClassifierField = "SrcIpRange"
	DstIpRange_Field ClassifierField = "DstIpRange"
)

type ClassifierError struct {
	missingField ClassifierField
}

func (e ClassifierError) Error() string {
	return fmt.Sprintf("Attempted using PacketClassifier with missing field (%v)", e.missingField)
}

func (e ClassifierError) Is(err error) bool {
	if typedErr, ok := err.(ClassifierError); ok {
		return typedErr.missingField == e.missingField
	}
	return false
}
