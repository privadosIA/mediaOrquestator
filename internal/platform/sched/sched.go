// Package sched bounds the memory that concurrent sd processes may use.
// sd copies a whole model into host RAM besides VRAM, so running several at
// once can exhaust memory (the machine froze this way on 2026-10-09); the
// Budget makes each process reserve an estimate before it starts.
package sched

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// GiB is one gibibyte.
const GiB int64 = 1 << 30

// Margin is kept free for the OS and everything else that is running.
const Margin = 4 * GiB

// Factor converts the total size of an sd process's model files into its
// expected resident memory. Measured ratios are 1.07 (Wan) to 1.1 (Krea 2).
const Factor = 1.15

// Estimate returns the memory to reserve for a process loading modelBytes of
// model files.
func Estimate(modelBytes int64) int64 {
	return int64(float64(modelBytes) * Factor)
}

// Budget is a weighted semaphore over a fixed number of bytes. A request
// larger than the whole budget waits until nothing else is reserved and then
// runs alone, so an oversized (or any, when the total is 0) request is
// serialized rather than refused or deadlocked.
type Budget struct {
	mu    sync.Mutex
	total int64
	used  int64
	// waiters are served in arrival order so a big request is not starved by
	// a stream of small ones.
	waiters []*waiter
}

type waiter struct {
	need  int64
	ready chan struct{}
}

// NewBudget returns a budget of total bytes (0 or negative serializes every
// request).
func NewBudget(total int64) *Budget {
	if total < 0 {
		total = 0
	}
	return &Budget{total: total}
}

// Total reports the budget size in bytes.
func (b *Budget) Total() int64 { return b.total }

// clamp is what a request actually holds: never more than the whole budget.
func (b *Budget) clamp(need int64) int64 {
	if need < 0 {
		need = 0
	}
	if need > b.total {
		return b.total
	}
	return need
}

// fits reports whether need (already clamped) can be granted now. A clamped
// oversized request holds the full budget, so it only fits when idle.
func (b *Budget) fits(need int64) bool {
	return b.used+need <= b.total
}

// Acquire reserves need bytes, waiting while they do not fit. It returns a
// release function (safe to call more than once) or ctx's error if the
// context ends first.
func (b *Budget) Acquire(ctx context.Context, need int64) (release func(), err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	need = b.clamp(need)
	// A zero total admits only zero-byte holds, which must still serialize
	// real work: treat any request as exclusive then.
	exclusive := b.total == 0
	if len(b.waiters) == 0 && (exclusive && b.used == 0 || !exclusive && b.fits(need)) {
		b.used += need
		if exclusive {
			b.used = 1 // marker: someone holds the exclusive slot
		}
		b.mu.Unlock()
		return b.releaser(need, exclusive), nil
	}
	w := &waiter{need: need, ready: make(chan struct{})}
	b.waiters = append(b.waiters, w)
	b.mu.Unlock()

	select {
	case <-w.ready:
		return b.releaser(need, exclusive), nil
	case <-ctx.Done():
		b.mu.Lock()
		granted := false
		select {
		case <-w.ready:
			granted = true
		default:
			for i, x := range b.waiters {
				if x == w {
					b.waiters = append(b.waiters[:i], b.waiters[i+1:]...)
					break
				}
			}
			// Others may now fit (this one may have been blocking the head).
			b.grantLocked()
		}
		b.mu.Unlock()
		if granted {
			// Granted at the same moment the context ended: give it back.
			b.releaser(need, exclusive)()
		}
		return nil, ctx.Err()
	}
}

func (b *Budget) releaser(need int64, exclusive bool) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			if exclusive {
				b.used = 0
			} else {
				b.used -= need
			}
			b.grantLocked()
			b.mu.Unlock()
		})
	}
}

// grantLocked wakes waiters from the head of the queue while they fit.
func (b *Budget) grantLocked() {
	exclusive := b.total == 0
	for len(b.waiters) > 0 {
		w := b.waiters[0]
		if exclusive {
			if b.used != 0 {
				return
			}
			b.used = 1
		} else {
			if !b.fits(w.need) {
				return
			}
			b.used += w.need
		}
		b.waiters = b.waiters[1:]
		close(w.ready)
	}
}

// AvailableFunc reads the memory available for new work, in bytes.
type AvailableFunc func() (int64, error)

// MemAvailable reads MemAvailable from /proc/meminfo, which counts the page
// cache that can be reclaimed.
func MemAvailable() (int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			break
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}
		return kb * 1024, nil
	}
	return 0, fmt.Errorf("sched: MemAvailable not found in /proc/meminfo")
}

// MemInfo reads the machine's total and available memory, in bytes.
func MemInfo() (total, available int64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		kb, perr := strconv.ParseInt(fields[1], 10, 64)
		if perr != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = kb * 1024
		case "MemAvailable:":
			available = kb * 1024
		}
	}
	if total == 0 || available == 0 {
		return 0, 0, fmt.Errorf("sched: memory figures not found in /proc/meminfo")
	}
	return total, available, nil
}

// NewBudgetFromAvailable builds the run's budget: available memory minus
// Margin. When the memory cannot be read the budget is 0, which serializes
// every sd process: the safe failure.
func NewBudgetFromAvailable(avail AvailableFunc) *Budget {
	n, err := avail()
	if err != nil {
		return NewBudget(0)
	}
	return NewBudget(n - Margin)
}
