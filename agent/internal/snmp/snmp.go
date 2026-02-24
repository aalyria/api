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

// Package agentcli provides an SBI agent that is configured using a
// protobuf-based manifest.
package snmp

import (
	"context"

	snmp "github.com/gosnmp/gosnmp"

	"aalyria.com/spacetime/agent/internal/configpb"
)

// TODO : Decide how to mappings: SnmpGo data types (e.g. OctetString) to internal to metric (e.g. counter, gauge, etc).
// For now, just use a simple type in the Metric even though it's not really useful at this point. We might
// even decide to remove the type (since we can just switch on the returned value).  However, it's possible we might need to
// distinguish between 2 int types, for example (like TimerTicks vs general Integer). If not, we probably don't even need to
// specify a type (here or in proto).
type MetricType int

const (
	TypeInt MetricType = iota
	TypeString
)

type Metric struct {
	Oid             string
	DisplayName     string
	MetricType      MetricType
	AlertConstraint *MetricAlertConstraint
}

type MetricAlertConstraint struct {
	InterfaceID        string
	NodeID             string
	Min                *float64
	Max                *float64
	TriggerIfUnchanged bool
	ShiftMetric        *float64
}

type MetricValue struct {
	Metric
	Value float64
}

func CreateSnmpStruct(ctx context.Context, config *configpb.SnmpConfig) *snmp.GoSNMP {
	snmpStruct := snmp.Default
	snmpStruct.Target = config.TargetUrl
	snmpStruct.Retries = int(config.Retries)
	// commenting this out turns off SNMP logging
	// snmpStruct.Logger = snmp.NewLogger(nlog.New(os.Stdout, "", 0)) // TODO : see how to use with zerolog

	if config.TargetPort != 0 {
		snmpStruct.Port = uint16(config.TargetPort)
	}

	timeout := config.Timeout.AsDuration()
	if timeout != 0 {
		snmpStruct.Timeout = timeout
	}

	if config.Community != "" {
		snmpStruct.Community = config.Community
	}

	if config.MaxOids != 0 {
		snmpStruct.MaxOids = int(config.MaxOids)
	}

	switch config.MsgFlags {
	case configpb.SnmpConfig_MSG_FLAGS_UNSPECIFIED:
		snmpStruct.MsgFlags = snmp.NoAuthNoPriv
	case configpb.SnmpConfig_NO_AUTH_NO_PRIV:
		snmpStruct.MsgFlags = snmp.NoAuthNoPriv
	case configpb.SnmpConfig_AUTH_NO_PRIV:
		snmpStruct.MsgFlags = snmp.AuthNoPriv
	case configpb.SnmpConfig_AUTH_PRIV:
		snmpStruct.MsgFlags = snmp.AuthPriv
	case configpb.SnmpConfig_REPORTABLE:
		snmpStruct.MsgFlags = snmp.Reportable
	}

	switch config.Version {
	case configpb.SnmpConfig_VERSION_UNSPECIFIED:
		snmpStruct.Version = snmp.Version1
	case configpb.SnmpConfig_VERSION_1:
		snmpStruct.Version = snmp.Version1
	case configpb.SnmpConfig_VERSION_2:
		snmpStruct.Version = snmp.Version2c
	case configpb.SnmpConfig_VERSION_3:
		snmpStruct.Version = snmp.Version3
		snmpStruct.SecurityModel = snmp.UserSecurityModel
	}

	// UserName is required if including SecurityParameters.
	if config.SecurityParameters != nil && config.SecurityParameters.UserName != "" {
		securityParameters := snmp.UsmSecurityParameters{
			UserName: config.SecurityParameters.UserName,
		}

		switch config.SecurityParameters.AuthProtocol {
		case configpb.SnmpUsmSecurityParameters_MD5:
			securityParameters.AuthenticationProtocol = snmp.MD5
		case configpb.SnmpUsmSecurityParameters_SHA:
			securityParameters.AuthenticationProtocol = snmp.SHA
		case configpb.SnmpUsmSecurityParameters_SHA224:
			securityParameters.AuthenticationProtocol = snmp.SHA224
		case configpb.SnmpUsmSecurityParameters_SHA256:
			securityParameters.AuthenticationProtocol = snmp.SHA256
		case configpb.SnmpUsmSecurityParameters_SHA384:
			securityParameters.AuthenticationProtocol = snmp.SHA384
		case configpb.SnmpUsmSecurityParameters_SHA512:
			securityParameters.AuthenticationProtocol = snmp.SHA512
		}

		if config.SecurityParameters.AuthPassphrase != "" {
			securityParameters.AuthenticationPassphrase = config.SecurityParameters.AuthPassphrase
		}

		switch config.SecurityParameters.PrivProtocol {
		case configpb.SnmpUsmSecurityParameters_DES:
			securityParameters.PrivacyProtocol = snmp.DES
		case configpb.SnmpUsmSecurityParameters_AES:
			securityParameters.PrivacyProtocol = snmp.AES
		case configpb.SnmpUsmSecurityParameters_AES192:
			securityParameters.PrivacyProtocol = snmp.AES192
		case configpb.SnmpUsmSecurityParameters_AES256:
			securityParameters.PrivacyProtocol = snmp.AES256
		case configpb.SnmpUsmSecurityParameters_AES192C:
			securityParameters.PrivacyProtocol = snmp.AES192C
		case configpb.SnmpUsmSecurityParameters_AES256C:
			securityParameters.PrivacyProtocol = snmp.AES256C
		}

		if config.SecurityParameters.PrivPassphrase != "" {
			securityParameters.PrivacyPassphrase = config.SecurityParameters.PrivPassphrase
		}
		snmpStruct.SecurityParameters = &securityParameters
	}

	return snmpStruct
}

func CreateMetrics(ctx context.Context, snmpObjects []*configpb.SnmpObject) []Metric {
	metricValues := []Metric{}

	// Using range causes a lock copy.
	for i := 0; i < len(snmpObjects); i++ {
		snmpObject := snmpObjects[i]
		snmpMetric := Metric{
			Oid:         snmpObject.Oid,
			DisplayName: snmpObject.DisplayName,
		}

		switch snmpObject.DataType {
		case configpb.SnmpObject_DATA_TYPE_UNSPECIFIED:
			snmpMetric.MetricType = TypeString
		case configpb.SnmpObject_STRING:
			snmpMetric.MetricType = TypeString
		case configpb.SnmpObject_INT:
			snmpMetric.MetricType = TypeInt
		}

		if snmpObject.AlertConstraint != nil {
			snmpMetric.AlertConstraint = &MetricAlertConstraint{
				InterfaceID:        snmpObject.AlertConstraint.InterfaceId,
				NodeID:             snmpObject.AlertConstraint.NodeId,
				Min:                snmpObject.AlertConstraint.Min,
				Max:                snmpObject.AlertConstraint.Max,
				TriggerIfUnchanged: snmpObject.AlertConstraint.TriggerIfUnchanged,
				ShiftMetric:        snmpObject.AlertConstraint.ShiftMetric,
			}
		}

		metricValues = append(metricValues, snmpMetric)
	}

	return metricValues
}
