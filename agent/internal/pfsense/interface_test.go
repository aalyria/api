// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
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

package pfsense

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPfSenseAPIResponse_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := PfSenseAPIResponse{
		Status:  "ok",
		Code:    200,
		Message: "success",
		Data:    map[string]any{"key": "value"},
	}

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}

	var decoded PfSenseAPIResponse
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() returned error: %v", err)
	}

	if diff := cmp.Diff(original, decoded); diff != "" {
		t.Errorf("PfSenseAPIResponse round trip mismatch (-want +got):\n%s", diff)
	}
}

func TestPfSenseAPIResponse_OmitsNilData(t *testing.T) {
	t.Parallel()

	// Data is tagged omitempty, so a response without a payload must not emit a
	// "data" field.
	encoded, err := json.Marshal(PfSenseAPIResponse{Status: "error", Code: 500, Message: "boom"})
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}

	if strings.Contains(string(encoded), "data") {
		t.Errorf("marshalled response = %s, want no \"data\" field when Data is nil", encoded)
	}
}
