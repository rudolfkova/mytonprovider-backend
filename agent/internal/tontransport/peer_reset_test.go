package tontransport

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallWithPeerReset_RetriesOnNewClient(t *testing.T) {
	var resets atomic.Int32
	err := callWithPeerReset(context.Background(), time.Second,
		func() (string, uint64) { return "old", 0 },
		func(uint64) (string, uint64, error) {
			resets.Add(1)
			return "new", 1, nil
		},
		func(_ context.Context, client string) error {
			if client == "old" {
				return errors.New("stuck")
			}
			if client != "new" {
				t.Fatalf("retry client = %q", client)
			}
			return nil
		},
		func(error) {},
	)
	if err != nil {
		t.Fatal(err)
	}
	if resets.Load() != 1 {
		t.Fatalf("resets = %d", resets.Load())
	}
}

func TestCallWithPeerReset_ResetErrorSkipsRetry(t *testing.T) {
	calls := 0
	err := callWithPeerReset(context.Background(), time.Second,
		func() (int, uint64) { return 1, 0 },
		func(uint64) (int, uint64, error) {
			return 0, 0, errors.New("reset failed")
		},
		func(context.Context, int) error {
			calls++
			return errors.New("stuck")
		},
		func(error) {
			t.Fatal("onRetry called")
		},
	)
	if err == nil || err.Error() != "stuck" {
		t.Fatalf("err = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestCallWithPeerReset_CancelledContextDoesNotRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := callWithPeerReset(ctx, time.Second,
		func() (int, uint64) { return 1, 0 },
		func(uint64) (int, uint64, error) {
			t.Fatal("reset called")
			return 0, 0, nil
		},
		func(context.Context, int) error {
			calls++
			cancel()
			return errors.New("stuck")
		},
		func(error) {
			t.Fatal("onRetry called")
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}

	calls = 0
	err = callWithPeerReset(ctx, time.Second,
		func() (int, uint64) { return 1, 0 },
		func(uint64) (int, uint64, error) {
			t.Fatal("reset called")
			return 0, 0, nil
		},
		func(context.Context, int) error {
			calls++
			return nil
		},
		nil,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if calls != 0 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestPeerSessions_ParallelFailuresResetOnce(t *testing.T) {
	var resets atomic.Int32
	sessions := newPeerSessions(providerPeerResetCooldown, func() (int, error) {
		resets.Add(1)
		time.Sleep(20 * time.Millisecond)
		return 2, nil
	}, nil)
	sessions.setInitial(1)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, gen, err := sessions.resetIfDue(0)
			if err != nil {
				t.Errorf("reset: %v", err)
				return
			}
			if gen != 1 || client != 2 {
				t.Errorf("client=%d gen=%d", client, gen)
			}
		}()
	}
	wg.Wait()
	if resets.Load() != 1 {
		t.Fatalf("resets = %d", resets.Load())
	}
}

func TestPeerSessions_CooldownSkipsRecreate(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	var resets atomic.Int32
	sessions := newPeerSessions(providerPeerResetCooldown, func() (int, error) {
		resets.Add(1)
		return int(resets.Load()) + 1, nil
	}, nil)
	sessions.now = func() time.Time { return now }
	sessions.setInitial(1)

	client, gen, err := sessions.resetIfDue(0)
	if err != nil {
		t.Fatal(err)
	}
	if gen != 1 || client != 2 {
		t.Fatalf("after first reset client=%d gen=%d", client, gen)
	}

	client, gen, err = sessions.resetIfDue(1)
	if err != nil {
		t.Fatal(err)
	}
	if resets.Load() != 1 {
		t.Fatalf("resets during cooldown = %d", resets.Load())
	}
	if gen != 1 || client != 2 {
		t.Fatalf("during cooldown client=%d gen=%d", client, gen)
	}

	now = now.Add(providerPeerResetCooldown)
	client, gen, err = sessions.resetIfDue(1)
	if err != nil {
		t.Fatal(err)
	}
	if resets.Load() != 2 {
		t.Fatalf("resets after cooldown = %d", resets.Load())
	}
	if gen != 2 || client != 3 {
		t.Fatalf("after cooldown client=%d gen=%d", client, gen)
	}
}

func TestCallWithPeerReset_ParallelFailuresShareOneReset(t *testing.T) {
	var resets atomic.Int32
	sessions := newPeerSessions(providerPeerResetCooldown, func() (int, error) {
		resets.Add(1)
		time.Sleep(20 * time.Millisecond)
		return 2, nil
	}, nil)
	sessions.setInitial(1)

	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := callWithPeerReset(context.Background(), time.Second,
				sessions.snapshot,
				sessions.resetIfDue,
				func(_ context.Context, client int) error {
					if client == 1 {
						return errors.New("stuck")
					}
					return nil
				},
				nil,
			)
			if err != nil {
				t.Errorf("call: %v", err)
			}
		}()
	}
	wg.Wait()
	if resets.Load() != 1 {
		t.Fatalf("resets = %d", resets.Load())
	}
}
