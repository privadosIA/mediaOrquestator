package sched

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEstimate(t *testing.T) {
	eight := 8 * GiB
	if got, want := Estimate(eight), int64(float64(eight)*1.15); got != want {
		t.Errorf("Estimate = %d, want %d", got, want)
	}
}

// acquireAsync starts an Acquire and reports when it is granted.
func acquireAsync(b *Budget, ctx context.Context, need int64) (granted chan func(), failed chan error) {
	granted, failed = make(chan func(), 1), make(chan error, 1)
	go func() {
		rel, err := b.Acquire(ctx, need)
		if err != nil {
			failed <- err
			return
		}
		granted <- rel
	}()
	return
}

func expectGranted(t *testing.T, g chan func(), what string) func() {
	t.Helper()
	select {
	case rel := <-g:
		return rel
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: not granted in time", what)
		return nil
	}
}

func expectWaiting(t *testing.T, g chan func(), what string) {
	t.Helper()
	select {
	case <-g:
		t.Fatalf("%s: granted but should wait", what)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBudget_TwoSmallOverlap(t *testing.T) {
	b := NewBudget(23 * GiB)
	r1, err := b.Acquire(context.Background(), 9*GiB)
	if err != nil {
		t.Fatal(err)
	}
	g2, _ := acquireAsync(b, context.Background(), 9*GiB)
	r2 := expectGranted(t, g2, "second 9 GiB request")
	r1()
	r2()
}

func TestBudget_TwoLargeSerialize(t *testing.T) {
	b := NewBudget(23 * GiB)
	r1, _ := b.Acquire(context.Background(), 18*GiB)
	g2, _ := acquireAsync(b, context.Background(), 18*GiB)
	expectWaiting(t, g2, "second 18 GiB request")
	r1()
	expectGranted(t, g2, "after release")()
}

func TestBudget_OversizedRunsAloneAndDoesNotDeadlock(t *testing.T) {
	b := NewBudget(10 * GiB)
	small, _ := b.Acquire(context.Background(), 2*GiB)
	g, _ := acquireAsync(b, context.Background(), 50*GiB)
	expectWaiting(t, g, "oversized request while another holds memory")
	// While the oversized one waits at the head, a later small one queues
	// behind it (no starvation).
	gs, _ := acquireAsync(b, context.Background(), 1*GiB)
	expectWaiting(t, gs, "small request behind the oversized one")
	small()
	big := expectGranted(t, g, "oversized request once alone")
	expectWaiting(t, gs, "small request while the oversized one runs")
	big()
	expectGranted(t, gs, "small request after the oversized one")()
}

func TestBudget_ZeroTotalSerializesEverything(t *testing.T) {
	b := NewBudget(0)
	r1, _ := b.Acquire(context.Background(), 1*GiB)
	g2, _ := acquireAsync(b, context.Background(), 0)
	expectWaiting(t, g2, "second request with a zero budget")
	r1()
	expectGranted(t, g2, "after release")()
}

func TestBudget_ContextCancelWhileWaiting(t *testing.T) {
	b := NewBudget(10 * GiB)
	r1, _ := b.Acquire(context.Background(), 10*GiB)
	ctx, cancel := context.WithCancel(context.Background())
	_, failed := acquireAsync(b, ctx, 5*GiB)
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-failed:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled waiter did not return")
	}
	r1()
	// The cancelled waiter left no reservation behind.
	rel, err := b.Acquire(context.Background(), 10*GiB)
	if err != nil {
		t.Fatal(err)
	}
	rel()
}

func TestBudget_ReleaseIsIdempotent(t *testing.T) {
	b := NewBudget(10 * GiB)
	r, _ := b.Acquire(context.Background(), 6*GiB)
	r()
	r()
	// A double release must not hand out more than the budget.
	r1, _ := b.Acquire(context.Background(), 6*GiB)
	g, _ := acquireAsync(b, context.Background(), 6*GiB)
	expectWaiting(t, g, "second 6 GiB after a double release")
	r1()
	expectGranted(t, g, "after release")()
}

func TestBudget_NeverExceedsTotalUnderLoad(t *testing.T) {
	b := NewBudget(10 * GiB)
	var inUse, peak int64
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		need := int64(1+i%4) * 2 * GiB // 2, 4, 6, 8 GiB
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, err := b.Acquire(context.Background(), need)
			if err != nil {
				t.Error(err)
				return
			}
			cur := atomic.AddInt64(&inUse, need)
			for {
				p := atomic.LoadInt64(&peak)
				if cur <= p || atomic.CompareAndSwapInt64(&peak, p, cur) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt64(&inUse, -need)
			rel()
		}()
	}
	wg.Wait()
	if peak > 10*GiB {
		t.Errorf("peak %d GiB exceeded the 10 GiB budget", peak/GiB)
	}
}

func TestNewBudgetFromAvailable(t *testing.T) {
	if got := NewBudgetFromAvailable(func() (int64, error) { return 27 * GiB, nil }).Total(); got != 23*GiB {
		t.Errorf("total = %d GiB, want 23", got/GiB)
	}
	if got := NewBudgetFromAvailable(func() (int64, error) { return 0, errors.New("unreadable") }).Total(); got != 0 {
		t.Errorf("unreadable memory must give a zero budget, got %d", got)
	}
	if got := NewBudgetFromAvailable(func() (int64, error) { return 2 * GiB, nil }).Total(); got != 0 {
		t.Errorf("less than the margin must clamp to 0, got %d", got)
	}
}

func TestMemAvailable_ReadsProcMeminfo(t *testing.T) {
	n, err := MemAvailable()
	if err != nil {
		t.Skipf("no /proc/meminfo: %v", err)
	}
	if n <= 0 {
		t.Errorf("MemAvailable = %d", n)
	}
}
