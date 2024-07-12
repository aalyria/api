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

// Package loggable provides some adaptors for logging Spacetime domain objects
// using the zerolog library.
package loggable

import (
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
)

// Returns a zerolog adaptor for the given proto. Since marshalling uses
// reflection (both proto reflection and runtime reflection), this should
// probably only be used for trace or debug logging or in codepaths where
// performance is not a concern.
func Proto(m proto.Message) zerolog.LogObjectMarshaler {
	return protoObject{m}
}

type protoObject struct{ proto.Message }

func (p protoObject) MarshalZerologObject(ev *zerolog.Event) {
	ev.Interface("proto", p.Message.ProtoReflect().Interface())
}
