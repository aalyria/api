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

package auth // import "aalyria.com/spacetime/auth"

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// refusedError marks an exchange that the identity provider refused.
type refusedError struct{ err error }

func (e *refusedError) Error() string { return e.err.Error() }
func (e *refusedError) Unwrap() error { return e.err }

// refused marks err as a refusal by the identity provider. The text of err
// does not change.
func refused(err error) error {
	return &refusedError{err: err}
}

// exchangeStatus turns a token exchange error into a gRPC status error, so
// that a gRPC client sees the kind of failure and not a blanket
// Unauthenticated. The message is err.Error(). The code is:
//   - the code of a status that err already holds (found by unwrapping), which
//     is how the caller's own Canceled or DeadlineExceeded survives;
//   - Unauthenticated when err holds a *refusedError, because a retry with the
//     same credential cannot succeed;
//   - Unavailable otherwise, because every other failure is transient from the
//     client's point of view.
//
// A context.DeadlineExceeded that is not inside a status is the exchange's own
// deadline, not the caller's, so it is Unavailable like every other failure to
// reach the provider. A nil err returns nil.
func exchangeStatus(err error) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok {
		return status.Error(st.Code(), err.Error())
	}
	if _, ok := errors.AsType[*refusedError](err); ok {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	return status.Error(codes.Unavailable, err.Error())
}
