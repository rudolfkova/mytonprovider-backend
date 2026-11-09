package tontransport

import (
	"context"
	"sync"
	"time"
)

const providerPeerResetCooldown = 20 * time.Second

// peerSessions tracks the live provider client generation.
// A reset closes every cached ADNL peer, so it is coalesced: one recreate
// for a burst of failures, and no second recreate inside the cooldown.
type peerSessions[T any] struct {
	mu          sync.Mutex
	cond        *sync.Cond
	cur         T
	gen         uint64
	lastReset   time.Time
	minInterval time.Duration
	resetting   bool
	now         func() time.Time
	recreate    func() (T, error)
	onRecreate  func()
}

func newPeerSessions[T any](minInterval time.Duration, recreate func() (T, error), onRecreate func()) *peerSessions[T] {
	s := &peerSessions[T]{
		minInterval: minInterval,
		now:         time.Now,
		recreate:    recreate,
		onRecreate:  onRecreate,
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

func (s *peerSessions[T]) setInitial(v T) {
	s.mu.Lock()
	s.cur = v
	s.mu.Unlock()
}

func (s *peerSessions[T]) snapshot() (T, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.resetting {
		s.cond.Wait()
	}
	return s.cur, s.gen
}

// resetIfDue recreates the session when seenGen is still current and the cooldown has elapsed.
// Concurrent callers that observed the same generation share one recreate.
func (s *peerSessions[T]) resetIfDue(seenGen uint64) (T, uint64, error) {
	s.mu.Lock()
	for s.resetting {
		s.cond.Wait()
	}
	now := s.now()
	if seenGen != s.gen || s.coolingDownLocked(now) {
		cur, gen := s.cur, s.gen
		s.mu.Unlock()
		return cur, gen, nil
	}
	s.resetting = true
	s.mu.Unlock()

	next, err := s.recreate()

	s.mu.Lock()
	if err == nil {
		s.cur = next
		s.gen++
		s.lastReset = s.now()
		if s.onRecreate != nil {
			s.onRecreate()
		}
	}
	s.resetting = false
	s.cond.Broadcast()
	cur, gen := s.cur, s.gen
	s.mu.Unlock()
	return cur, gen, err
}

func (s *peerSessions[T]) coolingDownLocked(now time.Time) bool {
	if s.lastReset.IsZero() {
		return false
	}
	return now.Sub(s.lastReset) < s.minInterval
}

// callWithPeerReset runs fn once. On failure with a live parent context it resets the session
// and retries exactly once with a fresh perQuery timeout.
func callWithPeerReset[T any](
	ctx context.Context,
	perQuery time.Duration,
	snapshot func() (T, uint64),
	reset func(seenGen uint64) (T, uint64, error),
	fn func(context.Context, T) error,
	onRetry func(error),
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	client, gen := snapshot()
	err := runWithTimeout(ctx, perQuery, client, fn)
	if err == nil || ctx.Err() != nil {
		return err
	}

	client, _, resetErr := reset(gen)
	if resetErr != nil || ctx.Err() != nil {
		return err
	}
	if onRetry != nil {
		onRetry(err)
	}
	return runWithTimeout(ctx, perQuery, client, fn)
}

func runWithTimeout[T any](ctx context.Context, perQuery time.Duration, client T, fn func(context.Context, T) error) error {
	qctx, cancel := context.WithTimeout(ctx, perQuery)
	defer cancel()
	return fn(qctx, client)
}
