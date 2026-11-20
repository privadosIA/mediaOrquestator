package pipeline

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseGPUs splits a --gpu value into GPU indexes. The empty string means
// no GPU pinning and yields nil. Entries must be distinct non-negative
// integers; the error names the offending value.
func ParseGPUs(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	var gpus []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid GPU %q in %q: indexes must be non-negative integers", part, s)
		}
		id := strconv.Itoa(n)
		if seen[id] {
			return nil, fmt.Errorf("duplicate GPU %q in %q", id, s)
		}
		seen[id] = true
		gpus = append(gpus, id)
	}
	return gpus, nil
}
