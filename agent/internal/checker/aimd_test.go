package checker

import (
	"context"
	"testing"
	"time"
)

func TestPieceLimitIncreaseWhenWindowIsFast(t *testing.T) {
	p := newPieceLimit(8, 40, 10, 16, nil, nil)
	for i := 0; i < 16; i++ {
		p.observe(100*time.Millisecond, time.Second)
	}
	if got := p.current(); got != 11 {
		t.Fatalf("limit = %d, want 11", got)
	}
}

func TestPieceLimitDecreaseWhenHalfTheWindowIsSlow(t *testing.T) {
	p := newPieceLimit(8, 40, 20, 16, nil, nil)
	for i := 0; i < 8; i++ {
		p.observe(900*time.Millisecond, time.Second)
	}
	for i := 0; i < 8; i++ {
		p.observe(100*time.Millisecond, time.Second)
	}
	if got := p.current(); got != 10 {
		t.Fatalf("limit = %d, want 10", got)
	}
}

func TestPieceLimitOneSlowPieceDoesNotDecrease(t *testing.T) {
	p := newPieceLimit(8, 40, 20, 16, nil, nil)
	p.observe(900*time.Millisecond, time.Second)
	for i := 0; i < 15; i++ {
		p.observe(100*time.Millisecond, time.Second)
	}
	if got := p.current(); got != 20 {
		t.Fatalf("limit = %d, want 20", got)
	}
}

func TestPieceLimitRespectsFloorAndCeiling(t *testing.T) {
	floor := newPieceLimit(8, 40, 8, 16, nil, nil)
	for i := 0; i < 16; i++ {
		floor.observe(time.Second, time.Second)
	}
	if got := floor.current(); got != 8 {
		t.Fatalf("floor limit = %d, want 8", got)
	}

	ceiling := newPieceLimit(8, 40, 40, 16, nil, nil)
	for i := 0; i < 16; i++ {
		ceiling.observe(100*time.Millisecond, time.Second)
	}
	if got := ceiling.current(); got != 40 {
		t.Fatalf("ceiling limit = %d, want 40", got)
	}
}

func TestPieceLimitAcquireWaitsForRelease(t *testing.T) {
	p := newPieceLimit(1, 1, 1, 16, nil, nil)
	ctx := context.Background()
	if err := p.acquire(ctx); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		time.Sleep(30 * time.Millisecond)
		p.release()
	}()
	go func() {
		done <- p.acquire(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("acquire did not proceed after release")
	}
	p.release()
}

func TestPieceLimitAcquireHonorsContext(t *testing.T) {
	p := newPieceLimit(1, 1, 1, 16, nil, nil)
	if err := p.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := p.acquire(ctx)
	if err == nil {
		t.Fatal("acquire succeeded while the only slot was held")
	}
	p.release()
}

func TestPieceTimeoutAddsTransferTimeAndCaps(t *testing.T) {
	base := 5 * time.Second
	got := pieceTimeout(312500, base, 312500, 15*time.Second)
	if got != 6*time.Second {
		t.Fatalf("timeout = %s, want 6s", got)
	}

	capped := pieceTimeout(10_000_000, base, 312500, 15*time.Second)
	if capped != 15*time.Second {
		t.Fatalf("capped timeout = %s, want 15s", capped)
	}
}

func TestPieceTimeoutKeepsBaseWithoutSizeOrRate(t *testing.T) {
	base := 5 * time.Second
	if got := pieceTimeout(0, base, 312500, 15*time.Second); got != base {
		t.Fatalf("zero piece = %s, want %s", got, base)
	}
	if got := pieceTimeout(312500, base, 0, 15*time.Second); got != base {
		t.Fatalf("zero rate = %s, want %s", got, base)
	}
}
