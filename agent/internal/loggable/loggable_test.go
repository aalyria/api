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

package loggable

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
)

func encode(key string, m proto.Message) []byte {
	buf := &bytes.Buffer{}
	logger := zerolog.New(buf)
	logger.Info().
		Object("msg", Proto(m)).
		Msg("testing proto marshaller")
	return buf.Bytes()
}

func TestSimpleProto(t *testing.T) {
	dur := &durationpb.Duration{Seconds: 12}

	bytes := encode("msg", dur)

	var got struct {
		LogMsg struct {
			Proto struct {
				Seconds int64 `json:"seconds"`
			} `json:"proto"`
		} `json:"msg"`
	}
	if err := json.Unmarshal(bytes, &got); err != nil {
		t.Error("unmarshalling error", err)
	}
	if got.LogMsg.Proto.Seconds != dur.GetSeconds() {
		t.Errorf("(%s) got %#v, wanted %v", string(bytes), got, dur)
	}
}

func TestStructProto(t *testing.T) {
	orig := map[string]interface{}{
		"firstName": "John",
		"lastName":  "Smith",
		"isAlive":   true,
		"age":       float64(27),
		"address": map[string]interface{}{
			"streetAddress": "21 2nd Street",
			"city":          "New York",
			"state":         "NY",
			"postalCode":    "10021-3100",
		},
		"phoneNumbers": []interface{}{
			map[string]interface{}{
				"type":   "home",
				"number": "212 555-1234",
			},
			map[string]interface{}{
				"type":   "office",
				"number": "646 555-4567",
			},
		},
		"children": []interface{}{},
		"spouse":   nil,
	}

	m, err := structpb.NewStruct(orig)
	if err != nil {
		t.Fatal("error creating struct from map[string]interface:", err)
	}

	gotPayload := map[string]interface{}{}
	if err := json.Unmarshal(encode("msg", m), &gotPayload); err != nil {
		t.Error("unmarshalling error", err)
	}
	if diff := cmp.Diff(orig, gotPayload["msg"].(map[string]interface{})["proto"]); diff != "" {
		t.Errorf("mismatch: (-want +got):\n%s", diff)
		t.FailNow()
	}
}

func TestStructProtoWithExtraSpaces(t *testing.T) {
	orig := map[string]interface{}{
		"a field with carriage returns": "Hello\rWorld",
	}

	m, err := structpb.NewStruct(orig)
	if err != nil {
		t.Fatal("error creating struct from map[string]interface:", err)
	}
	payload := map[string]interface{}{}
	if err := json.Unmarshal(encode("msg", m), &payload); err != nil {
		t.Error("unmarshalling error", err)
	}
	got := payload["msg"].(map[string]interface{})["proto"]
	if diff := cmp.Diff(orig, got); diff != "" {
		t.Errorf("mismatch: (-want +got):\n%s", diff)
		t.FailNow()
	}
}
