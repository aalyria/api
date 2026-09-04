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

package protofmt

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

type Format int

const (
	JSON Format = iota
	Wire
	Text
)

func FromString(s string) (Format, error) {
	switch strings.ToLower(s) {
	case "text":
		return Text, nil
	case "json":
		return JSON, nil
	case "wire":
		return Wire, nil
	default:
		return 0, fmt.Errorf("unknown proto format: %q", s)
	}
}

func (pf Format) String() string {
	switch pf {
	case JSON:
		return "JSON"
	case Text:
		return "text"
	case Wire:
		return "wire"
	default:
		return "unknown"
	}
}

func (pf Format) Marshal(m proto.Message) ([]byte, error) {
	switch pf {
	case JSON:
		return protojson.Marshal(m)
	case Text:
		return prototext.Marshal(m)
	case Wire:
		return proto.Marshal(m)
	default:
		return nil, fmt.Errorf("marshal: unknown format: %v", pf)
	}
}

func (pf Format) Unmarshal(data []byte, m proto.Message) error {
	switch pf {
	case JSON:
		return protojson.Unmarshal(data, m)
	case Text:
		return prototext.Unmarshal(data, m)
	case Wire:
		return proto.Unmarshal(data, m)
	default:
		return fmt.Errorf("unmarshal: unknown format: %v", pf)
	}
}
