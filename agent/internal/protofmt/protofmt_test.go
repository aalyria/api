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
	"testing"
)

func TestString_roundTripping(t *testing.T) {
	for _, n := range []Format{JSON, Wire, Text} {
		t.Run(n.String(), func(t *testing.T) {
			pf, err := FromString(n.String())
			if err != nil {
				t.Errorf("got unexpected error from FromString(%q): %v", n.String(), err)
				return
			}

			if n != pf {
				t.Errorf("expected round-tripping %v to be the same, but got %v", n, pf)
			}
		})
	}
}

func TestFromString(t *testing.T) {
	for s, f := range map[string]Format{"json": JSON, "JSON": JSON, "text": Text, "wire": Wire} {
		t.Run(fmt.Sprintf("%q -> %v", s, f), func(t *testing.T) {
			pf, err := FromString(s)
			if err != nil {
				t.Errorf("got unexpected error from FromString(%q): %v", s, err)
				return
			}

			if f != pf {
				t.Errorf("expected FromString(%q) to return %v, but got %v", s, f, pf)
			}
		})
	}
}
