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

package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestExchangeStatus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want codes.Code
	}{
		{
			name: "a plain error is unavailable",
			err:  errors.New("provider down"),
			want: codes.Unavailable,
		},
		{
			name: "a refusal is unauthenticated",
			err:  refused(errors.New("token endpoint returned status 400")),
			want: codes.Unauthenticated,
		},
		{
			name: "a wrapped refusal is still unauthenticated",
			err:  fmt.Errorf("failed to get the token: %w", refused(errors.New("token endpoint returned status 401"))),
			want: codes.Unauthenticated,
		},
		{
			name: "a status keeps its code",
			err:  status.Error(codes.DeadlineExceeded, "context deadline exceeded"),
			want: codes.DeadlineExceeded,
		},
		{
			name: "a wrapped status keeps its code",
			err:  fmt.Errorf("failed to get the token: %w", status.Error(codes.Canceled, "context canceled")),
			want: codes.Canceled,
		},
		{
			// The exchange's own deadline surfaces as context.DeadlineExceeded
			// inside an HTTP error. That is the provider not answering, which
			// is a transient failure and not the caller's deadline.
			name: "a bare context deadline is unavailable",
			err:  fmt.Errorf("executing token request: %w", context.DeadlineExceeded),
			want: codes.Unavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := exchangeStatus(tc.err)
			if code := status.Code(got); code != tc.want {
				t.Errorf("exchangeStatus(%v) has code %s, want %s", tc.err, code, tc.want)
			}
		})
	}
}

// TestExchangeStatusKeepsTheMessage pins that the status message is the whole
// error text. The operator reads the message to find the cause, so the
// prefixes that the callers add must survive.
func TestExchangeStatusKeepsTheMessage(t *testing.T) {
	t.Parallel()

	for _, err := range []error{
		errors.New("provider down"),
		fmt.Errorf("failed to get the token: %w", refused(errors.New("token endpoint returned status 400"))),
		fmt.Errorf("failed to get the token: %w", status.Error(codes.DeadlineExceeded, "context deadline exceeded")),
	} {
		st, ok := status.FromError(exchangeStatus(err))
		if !ok {
			t.Fatalf("exchangeStatus(%v) is not a status error", err)
		}
		if st.Message() != err.Error() {
			t.Errorf("exchangeStatus(%v) has message %q, want %q", err, st.Message(), err.Error())
		}
	}
}

func TestExchangeStatusOfNilIsNil(t *testing.T) {
	t.Parallel()

	if got := exchangeStatus(nil); got != nil {
		t.Errorf("exchangeStatus(nil) = %v, want nil", got)
	}
}

func TestRefusedKeepsTheErrorText(t *testing.T) {
	t.Parallel()

	cause := errors.New("the discovery document reports issuer \"https://impostor.example.com\"")
	got := refused(cause)
	if got.Error() != cause.Error() {
		t.Errorf("refused(%v).Error() = %q, want %q", cause, got.Error(), cause.Error())
	}
	if !errors.Is(got, cause) {
		t.Errorf("refused(%v) does not unwrap to its cause", cause)
	}
}
