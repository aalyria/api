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

package task

import (
	"context"
	"errors"
	"reflect"
	"testing"

	otelsdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestContextGetters_Tracer(t *testing.T) {
	want := oteltracenoop.NewTracerProvider().Tracer("task")

	ctx := InjectTracer(context.Background(), want)
	got, ok := ExtractTracer(ctx)
	if !ok {
		t.Errorf("wasn't able to retrieve injected tracer")
		return
	}

	if !reflect.DeepEqual(want, got) {
		t.Errorf("got tracer %#v, but wanted tracer %#v", got, want)
	}
}

func TestContextGetters_TracerProvider(t *testing.T) {
	want := otelsdktrace.NewTracerProvider()

	ctx := InjectTracerProvider(context.Background(), want)
	got, ok := ExtractTracerProvider(ctx)
	if !ok {
		t.Errorf("wasn't able to retrieve injected tracer")
		return
	}

	if want != got {
		t.Errorf("got tracer %#v, but wanted tracer %#v", got, want)
	}
}

func TestAsThunk(t *testing.T) {
	want := context.DeadlineExceeded

	var got error
	Task(func(_ context.Context) error { return want }).
		AsThunk(context.Background(), &got)()

	if got != want {
		t.Errorf("AsThunk didn't set err correctly, got %v but wanted %v", got, want)
	}
}

func TestAsThunk_disregardsPreviousErr(t *testing.T) {
	want := context.DeadlineExceeded

	got := context.Canceled
	Task(func(_ context.Context) error { return want }).
		AsThunk(context.Background(), &got)()

	if got != want {
		t.Errorf("AsThunk didn't set err correctly, got %v but wanted %v", got, want)
	}
}

func TestGroup_emptyGroup(t *testing.T) {
	err := Group()(context.Background())
	if err != errNoTasks {
		t.Errorf("unexpected result from empty group, got %v but wanted %v", err, errNoTasks)
	}
}

func TestWithPanicCatcher(t *testing.T) {
	badTask := func(_ context.Context) error { panic("bad") }

	got := Task(badTask).WithPanicCatcher()(context.Background())
	want := errors.New("panic: bad")
	if got.Error() != want.Error() {
		t.Errorf("expected result from WithPanicCatcher to be %v, but got %v", want, got)
	}
}
