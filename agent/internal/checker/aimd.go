package checker

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	aimdMinLimit  = 8
	aimdWindow    = 16
	aimdSlowRatio = 0.8
	aimdFastRatio = 0.5
)

// pieceLimit caps how many GetPiece calls run at once and moves that cap by AIMD.
// A full window of fast pieces adds one slot. A window where at least half the
// pieces ran longer than 80% of their own timeout cuts the cap in half.
type pieceLimit struct {
	mu       sync.Mutex
	cond     *sync.Cond
	inflight int
	limit    int
	min      int
	max      int
	window   int
	samples  []float64
	onLimit  func(int)
	logger   *slog.Logger
}

func newPieceLimit(min, max, initial, window int, onLimit func(int), logger *slog.Logger) *pieceLimit {
	if min < 1 {
		min = 1
	}
	if max < min {
		max = min
	}
	if initial < min {
		initial = min
	}
	if initial > max {
		initial = max
	}
	if window < 1 {
		window = aimdWindow
	}

	p := &pieceLimit{
		limit:   initial,
		min:     min,
		max:     max,
		window:  window,
		onLimit: onLimit,
		logger:  logger,
	}
	p.cond = sync.NewCond(&p.mu)
	if onLimit != nil {
		onLimit(initial)
	}
	return p
}

func (p *pieceLimit) acquire(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Wake the waiter when the job context ends. The goroutine only locks
	// to broadcast, and acquire never waits for it, so this cannot deadlock.
	wake := make(chan struct{})
	defer close(wake)
	go func() {
		select {
		case <-ctx.Done():
			p.mu.Lock()
			p.cond.Broadcast()
			p.mu.Unlock()
		case <-wake:
		}
	}()

	for p.inflight >= p.limit {
		if err := ctx.Err(); err != nil {
			return err
		}
		p.cond.Wait()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.inflight++
	return nil
}

func (p *pieceLimit) release() {
	p.mu.Lock()
	if p.inflight > 0 {
		p.inflight--
	}
	p.cond.Broadcast()
	p.mu.Unlock()
}

func (p *pieceLimit) observe(elapsed, budget time.Duration) {
	if budget <= 0 || elapsed < 0 {
		return
	}

	ratio := float64(elapsed) / float64(budget)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.samples = append(p.samples, ratio)
	if len(p.samples) < p.window {
		return
	}

	slow := 0
	fast := 0
	for _, sample := range p.samples {
		if sample > aimdSlowRatio {
			slow++
		}
		if sample <= aimdFastRatio {
			fast++
		}
	}
	p.samples = p.samples[:0]

	switch {
	case slow*2 >= p.window:
		next := p.limit / 2
		if next < p.min {
			next = p.min
		}
		p.setLimitLocked(next)
	case fast == p.window:
		next := p.limit + 1
		if next > p.max {
			next = p.max
		}
		p.setLimitLocked(next)
	}
}

func (p *pieceLimit) setLimitLocked(next int) {
	if next == p.limit {
		return
	}
	increased := next > p.limit
	p.limit = next
	if increased {
		p.cond.Broadcast()
	}
	if p.logger != nil {
		p.logger.Info("piece concurrency limit", "limit", next)
	}
	if p.onLimit != nil {
		p.onLimit(next)
	}
}

func (p *pieceLimit) current() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.limit
}

// pieceTimeout is the GetPiece deadline: the request's RLDP timeout plus the
// time the piece needs at minBPS, clamped to cap.
func pieceTimeout(pieceSize uint32, base time.Duration, minBPS uint64, cap time.Duration) time.Duration {
	if minBPS == 0 || pieceSize == 0 || base <= 0 {
		if base < 0 {
			return 0
		}
		return base
	}

	extra := time.Duration(pieceSize) * time.Second / time.Duration(minBPS)
	d := base + extra
	if cap > 0 && d > cap {
		return cap
	}
	return d
}
