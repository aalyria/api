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

	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	"google.golang.org/protobuf/proto"
)

// NoChangeSpecifiedError indicates that there is no Change supplied in the
// [schedpb.CreateEntryRequest].
type NoChangeSpecifiedError struct {
	req *schedpb.CreateEntryRequest
}

func (e *NoChangeSpecifiedError) Error() string {
	return fmt.Sprintf("CreateEntryRequest received with no ConfigurationChange specified: %v", e.req)
}

func (e *NoChangeSpecifiedError) Is(err error) bool {
	if typedErr, ok := err.(*NoChangeSpecifiedError); ok {
		return proto.Equal(typedErr.req, e.req)
	}
	return false
}

///////////////////////////////////////////////////////////////////////////////////////////////

// UnsupportedUpdateError indicates the UpdateType specified in the
// [schedpb.CreateEntryRequest] is unsupported.
type UnsupportedUpdateError struct {
	req *schedpb.CreateEntryRequest
}

func (e *UnsupportedUpdateError) Error() string {
	return fmt.Sprintf(
		"unsupported update type %T on update id %s",
		e.req.GetConfigurationChange(), e.req.GetId())
}

func (e *UnsupportedUpdateError) Is(err error) bool {
	if typedErr, ok := err.(*UnsupportedUpdateError); ok {
		return proto.Equal(e.req, typedErr.req)
	}
	return false
}

///////////////////////////////////////////////////////////////////////////////////////////////

// UnknownRouteDeleteError indicates an unknown FlowRule is attempted to be deleted
type UnknownRouteDeleteError struct {
	changeID string
}

func (e *UnknownRouteDeleteError) Error() string {
	return fmt.Sprintf("attempted to DELETE unknown route: %s", e.changeID)
}

func (e *UnknownRouteDeleteError) Is(err error) bool {
	if typedErr, ok := err.(*UnknownRouteDeleteError); ok {
		return typedErr.changeID == e.changeID
	}
	return false
}

///////////////////////////////////////////////////////////////////////////////////////////////

// IPFormattingError indicates an erroneously formatted IP address or prefix was passed
type IPField string

const (
	Dst_IPField IPField = "To"
	Via_IPField IPField = "Via"
	Src_IPField IPField = "From"
)

type IPFormattingError struct {
	ip          string
	sourceField IPField
}

func (e IPFormattingError) Error() string {
	return fmt.Sprintf("attempted using wrongly formatted IP address/range (%s) for %s field", e.ip, e.sourceField)
}

func (e IPFormattingError) Is(err error) bool {
	if typedErr, ok := err.(IPFormattingError); ok {
		return typedErr.sourceField == e.sourceField && typedErr.ip == e.ip
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
	return fmt.Sprintf("attempted using erroneous interface (%s): %v", e.wrongIface, e.sourceError)
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

type MismatchedAddressFamilyError struct {
	srcFamily, dstFamily int
}

func (m MismatchedAddressFamilyError) Error() string {
	return fmt.Sprintf("got mismatched address families: from=%v to=%v", m.srcFamily, m.dstFamily)
}

func (m MismatchedAddressFamilyError) Is(err error) bool {
	if typed, ok := err.(MismatchedAddressFamilyError); ok {
		return typed.srcFamily == m.srcFamily && typed.dstFamily == m.dstFamily
	}
	return false
}
