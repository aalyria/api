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
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errProviderDown = errors.New("provider down")

// waitForRefresh waits for the exchange that a call with a stale token
// started in the background. The call registered the exchange before it
// returned, so joining it blocks until it finishes. When it has finished
// already, Do runs the empty function instead. Do not call it while another
// goroutine is in get: a get that joins the empty function gets no token.
func waitForRefresh(c *tokenCache) {
	c.sf.Do(exchangeKey, func() (any, error) { return nil, nil })
}

func newTestTokenCache() (*tokenCache, *clockwork.FakeClock) {
	clock := clockwork.NewFakeClockAt(time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC))
	return &tokenCache{clock: clock}, clock
}

// countingExchange is a fake exchange that counts its calls. When block is
// not nil, each call waits until block is closed before it returns token.
type countingExchange struct {
	calls atomic.Int32
	token *expiringToken
	err   error
	block <-chan struct{}
}

func (e *countingExchange) exchange(ctx context.Context) (*expiringToken, error) {
	e.calls.Add(1)
	if e.block != nil {
		<-e.block
	}
	if e.err != nil {
		return nil, e.err
	}
	return e.token, nil
}

func TestTokenCacheReturnsAFreshTokenWithoutAnExchange(t *testing.T) {
	t.Parallel()

	cache, clock := newTestTokenCache()
	ex := &countingExchange{token: &expiringToken{tok: "first", expiresAt: clock.Now().Add(time.Hour)}}

	for i := range 2 {
		got, err := cache.get(t.Context(), ex.exchange)
		if err != nil {
			t.Fatalf("get call %d returned an unexpected error: %v", i, err)
		}
		if got != "first" {
			t.Errorf("get call %d: got %q, want %q", i, got, "first")
		}
	}
	if n := ex.calls.Load(); n != 1 {
		t.Errorf("the exchange ran %d times for a fresh token, want 1", n)
	}
}

func TestTokenCacheCollapsesConcurrentExchanges(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cache, clock := newTestTokenCache()
		release := make(chan struct{})
		ex := &countingExchange{
			token: &expiringToken{tok: "shared", expiresAt: clock.Now().Add(time.Hour)},
			block: release,
		}

		const callers = 8
		results := make(chan string, callers)
		for range callers {
			go func() {
				got, err := cache.get(t.Context(), ex.exchange)
				if err != nil {
					t.Errorf("get returned an unexpected error: %v", err)
				}
				results <- got
			}()
		}

		// Every caller is now blocked on the one exchange.
		synctest.Wait()
		if n := ex.calls.Load(); n != 1 {
			t.Errorf("%d callers started %d exchanges, want 1", callers, n)
		}

		close(release)
		for range callers {
			if got := <-results; got != "shared" {
				t.Errorf("get: got %q, want %q", got, "shared")
			}
		}
	})
}

// TestTokenCacheSurvivesACallerThatGivesUp is one half of the detachment
// rule: a caller that cancels while the exchange is in flight must not
// abort the exchange, which has to finish and fill the cache for the next
// caller.
func TestTokenCacheSurvivesACallerThatGivesUp(t *testing.T) {
	t.Parallel()

	cache, clock := newTestTokenCache()
	release := make(chan struct{})
	ex := &countingExchange{
		token: &expiringToken{tok: "survivor", expiresAt: clock.Now().Add(time.Hour)},
		block: release,
	}

	leaderCtx, cancelLeader := context.WithCancel(t.Context())
	leaderErr := make(chan error, 1)
	go func() {
		_, err := cache.get(leaderCtx, ex.exchange)
		leaderErr <- err
	}()

	cancelLeader()
	if err := <-leaderErr; status.Code(err) != codes.Canceled {
		t.Errorf("the canceled caller got %v, want code %s", err, codes.Canceled)
	}
	close(release)

	got, err := cache.get(t.Context(), ex.exchange)
	if err != nil {
		t.Fatalf("the exchange the leader started did not survive its cancellation: %v", err)
	}
	if got != "survivor" {
		t.Errorf("get: got %q, want %q", got, "survivor")
	}
	if n := ex.calls.Load(); n != 1 {
		t.Errorf("the exchange ran %d times, want 1; the leader's cancellation threw away the exchange", n)
	}
}

// TestTokenCacheReturnsWhenTheCallerDeadlinePasses is the other half of the
// detachment rule: detaching the exchange must not detach the caller from
// it. A caller whose own context ends returns at once, with a status error
// that carries the context's code.
func TestTokenCacheReturnsWhenTheCallerDeadlinePasses(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cache, clock := newTestTokenCache()
		release := make(chan struct{})
		ex := &countingExchange{
			token: &expiringToken{tok: "late", expiresAt: clock.Now().Add(time.Hour)},
			block: release,
		}

		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()

		returned := make(chan error, 1)
		go func() {
			_, err := cache.get(ctx, ex.exchange)
			returned <- err
		}()

		// The exchange timeout is 30 seconds. Fake time only passes while
		// every goroutine is blocked, so a caller that waits on the exchange
		// instead of its own deadline returns after 30 seconds, not 50 ms.
		start := time.Now()
		err := <-returned
		if waited := time.Since(start); waited > time.Second {
			t.Errorf("get returned after %v, want about 50ms", waited)
		}
		if status.Code(err) != codes.DeadlineExceeded {
			t.Errorf("get returned %v, want code %s", err, codes.DeadlineExceeded)
		}
		close(release)
	})
}

func TestTokenCacheBoundsTheExchange(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cache, _ := newTestTokenCache()
		cache.timeout = 50 * time.Millisecond

		var hadDeadline atomic.Bool
		_, err := cache.get(t.Context(), func(ctx context.Context) (*expiringToken, error) {
			_, ok := ctx.Deadline()
			hadDeadline.Store(ok)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		if !hadDeadline.Load() {
			t.Error("the exchange context has no deadline")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("get returned %v, want %v from the exchange", err, context.DeadlineExceeded)
		}
	})
}

func TestTokenCacheDetachesTheExchangeFromTheCaller(t *testing.T) {
	t.Parallel()

	cache, clock := newTestTokenCache()
	release := make(chan struct{})
	seen := make(chan error, 1)
	exchange := func(ctx context.Context) (*expiringToken, error) {
		<-release
		// The caller has canceled by now. A detached context is still live.
		seen <- ctx.Err()
		return &expiringToken{tok: "detached", expiresAt: clock.Now().Add(time.Hour)}, nil
	}

	leaderCtx, cancelLeader := context.WithCancel(t.Context())
	leaderReturned := make(chan struct{})
	go func() {
		defer close(leaderReturned)
		_, _ = cache.get(leaderCtx, exchange)
	}()

	cancelLeader()
	<-leaderReturned
	close(release)

	if err := <-seen; err != nil {
		t.Errorf("the exchange context ended with the caller: %v", err)
	}
	got, err := cache.get(t.Context(), exchange)
	if err != nil {
		t.Fatalf("get returned an unexpected error: %v", err)
	}
	if got != "detached" {
		t.Errorf("get: got %q, want %q", got, "detached")
	}
}

// TestTokenCacheServesAStaleTokenWithoutWaiting makes sure that a stale
// token that has not expired goes back to the caller at once, while one
// exchange refreshes the cache in the background. A caller that waited for
// the exchange would hang for as long as an unresponsive provider does.
func TestTokenCacheServesAStaleTokenWithoutWaiting(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cache, clock := newTestTokenCache()
		cache.token.Store(&expiringToken{tok: "old", expiresAt: clock.Now().Add(time.Hour)})
		// Inside the 5-minute window, so the token is stale but not expired.
		clock.Advance(57 * time.Minute)
		release := make(chan struct{})
		ex := &countingExchange{
			token: &expiringToken{tok: "new", expiresAt: clock.Now().Add(time.Hour)},
			block: release,
		}

		// The exchange is held open, so a get that waits for it never
		// returns, and synctest reports the deadlock.
		for i := range 2 {
			got, err := cache.get(t.Context(), ex.exchange)
			if err != nil {
				t.Fatalf("get call %d returned an error, want the stale token: %v", i, err)
			}
			if got != "old" {
				t.Errorf("get call %d: got %q, want %q", i, got, "old")
			}
		}
		synctest.Wait()
		if n := ex.calls.Load(); n != 1 {
			t.Errorf("two calls with a stale token started %d exchanges, want 1", n)
		}

		close(release)
		synctest.Wait()
		got, err := cache.get(t.Context(), ex.exchange)
		if err != nil {
			t.Fatalf("get returned an unexpected error: %v", err)
		}
		if got != "new" {
			t.Errorf("get after the refresh: got %q, want %q", got, "new")
		}
	})
}

func TestTokenCacheServesAStaleTokenWhenTheExchangeFails(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cache, clock := newTestTokenCache()
		cache.token.Store(&expiringToken{tok: "old", expiresAt: clock.Now().Add(time.Hour)})
		// Inside the 5-minute window, so the token is stale but not expired.
		clock.Advance(57 * time.Minute)
		ex := &countingExchange{err: errProviderDown}

		got, err := cache.get(t.Context(), ex.exchange)
		if err != nil {
			t.Fatalf("get returned an error, want the stale token: %v", err)
		}
		if got != "old" {
			t.Errorf("get: got %q, want %q", got, "old")
		}
		synctest.Wait()
		if n := ex.calls.Load(); n != 1 {
			t.Errorf("the exchange ran %d times for a stale token, want 1", n)
		}
	})
}

func TestTokenCacheDoesNotServeAnExpiredToken(t *testing.T) {
	t.Parallel()

	cache, clock := newTestTokenCache()
	cache.token.Store(&expiringToken{tok: "old", expiresAt: clock.Now().Add(time.Hour)})
	clock.Advance(61 * time.Minute)
	ex := &countingExchange{err: errProviderDown}

	got, err := cache.get(t.Context(), ex.exchange)
	if !errors.Is(err, errProviderDown) {
		t.Errorf("get returned %v, want %v", err, errProviderDown)
	}
	if got != "" {
		t.Errorf("get returned the expired token %q", got)
	}
}

func TestTokenCacheRetriesTheExchangeAfterAStaleToken(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cache, clock := newTestTokenCache()
		cache.token.Store(&expiringToken{tok: "old", expiresAt: clock.Now().Add(time.Hour)})
		clock.Advance(57 * time.Minute)
		ex := &countingExchange{err: errProviderDown}

		for i := range 2 {
			if _, err := cache.get(t.Context(), ex.exchange); err != nil {
				t.Fatalf("get call %d returned an error, want the stale token: %v", i, err)
			}
			// Let the failed exchange finish, so the next call starts its own.
			synctest.Wait()
		}
		if n := ex.calls.Load(); n != 2 {
			t.Errorf("the exchange ran %d times for two calls with a stale token, want 2", n)
		}
	})
}

func TestTokenCacheReplacesTheTokenAfterASuccessfulExchange(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cache, clock := newTestTokenCache()
		cache.token.Store(&expiringToken{tok: "old", expiresAt: clock.Now().Add(time.Hour)})
		clock.Advance(57 * time.Minute)
		ex := &countingExchange{token: &expiringToken{tok: "new", expiresAt: clock.Now().Add(time.Hour)}}

		if _, err := cache.get(t.Context(), ex.exchange); err != nil {
			t.Fatalf("the first get returned an unexpected error: %v", err)
		}
		synctest.Wait()
		for i := range 2 {
			got, err := cache.get(t.Context(), ex.exchange)
			if err != nil {
				t.Fatalf("get call %d returned an unexpected error: %v", i, err)
			}
			if got != "new" {
				t.Errorf("get call %d: got %q, want %q", i, got, "new")
			}
		}
		if n := ex.calls.Load(); n != 1 {
			t.Errorf("the exchange ran %d times, want 1; the new token was not cached", n)
		}
	})
}
