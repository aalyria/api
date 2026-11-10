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
	"cmp"
	"context"
	"sync/atomic"
	"time"

	"github.com/jonboulle/clockwork"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc/status"
)

// exchangeTimeout bounds one token exchange. The exchange is detached from
// its caller's context, so it needs a deadline of its own or an unresponsive
// provider would hold every waiting caller forever. Both OIDC credential
// types exchange under it.
const exchangeTimeout = 30 * time.Second

// exchangeKey is the singleflight key of the one exchange a cache runs.
const exchangeKey = "token"

// tokenCache holds one access token and collapses concurrent exchanges into
// one.
type tokenCache struct {
	clock clockwork.Clock
	// timeout bounds one exchange. Zero means exchangeTimeout. Tests set it.
	timeout time.Duration
	token   atomic.Pointer[expiringToken]
	sf      singleflight.Group
}

// get returns a fresh cached token, or runs exchange to get one.
//
// Concurrent callers share one exchange. The exchange is detached from the
// context of whichever caller started it and given a deadline of its own,
// because every caller waits on the same call: at cold start every RPC
// misses the cache at once, and one of them giving up would otherwise fail
// all the others with a misleading "context canceled".
//
// Each caller still waits under its own context, so giving up returns. The
// exchange it abandons carries on and fills the cache for whoever is still
// waiting. When ctx ends first, get returns
// status.FromContextError(ctx.Err()).Err(), so that the error keeps the
// context's gRPC code instead of becoming Unauthenticated.
//
// A stale token that has not expired is still valid at the server. For such
// a token, get starts the exchange and returns the token at once, without
// waiting: the exchange refreshes the cache in the background, and when it
// fails, the next call tries again. Only a caller with no token or an expired
// one waits, and it gets the exchange error unchanged.
func (c *tokenCache) get(ctx context.Context, exchange func(context.Context) (*expiringToken, error)) (string, error) {
	cached := c.token.Load()
	if cached != nil && !cached.isStale(c.clock) {
		return cached.tok, nil
	}

	ch := c.sf.DoChan(exchangeKey, func() (any, error) {
		if token := c.token.Load(); token != nil && !token.isStale(c.clock) {
			return token.tok, nil
		}
		exchangeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cmp.Or(c.timeout, exchangeTimeout))
		defer cancel()
		token, err := exchange(exchangeCtx)
		if err != nil {
			return nil, err
		}
		c.token.Store(token)
		return token.tok, nil
	})

	if cached != nil && c.clock.Now().Before(cached.expiresAt) {
		return cached.tok, nil
	}

	select {
	case <-ctx.Done():
		return "", status.FromContextError(ctx.Err()).Err()
	case result := <-ch:
		if result.Err != nil {
			return "", result.Err
		}
		return result.Val.(string), nil
	}
}
