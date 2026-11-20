package promptfetch

import "sort"

// rng is a small seeded generator (mulberry32): the same seed gives the same sequence.
type rng struct{ a uint32 }

func (r *rng) next() float64 {
	r.a += 0x6d2b79f5
	t := r.a
	t = (t ^ (t >> 15)) * (t | 1)
	t ^= t + (t^(t>>7))*(t|61)
	return float64(t^(t>>14)) / 4294967296
}

// Sample chooses n items with the seed and keeps the order they had (a partial Fisher-Yates over the indices): the
// same items and the same seed give the same list.
func Sample[T any](items []T, n int, seed int) []T {
	if len(items) <= n {
		return items
	}
	r := &rng{a: uint32(seed)}
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	for i := 0; i < n; i++ {
		j := i + int(r.next()*float64(len(idx)-i))
		idx[i], idx[j] = idx[j], idx[i]
	}
	chosen := idx[:n]
	sort.Ints(chosen)
	out := make([]T, n)
	for i, k := range chosen {
		out[i] = items[k]
	}
	return out
}
