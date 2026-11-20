// Package sdlog reads what the stable-diffusion.cpp CLI (bin/sd, pinned in scripts/build-sd.sh) says about its own
// work. sd writes its log to standard output, not to standard error, and loads each component (text encoder, VAE,
// diffusion model...) just before it uses it, printing one line per load:
//
//	[I] loading tensors completed, taking 0.20s (read: 0.04s, memcpy: 0.00s, ...) --- model_loader.cpp:1381
//
// The model load of one invocation is the sum of those lines. The output of sd is not a stable API: when no such line
// is there (another version, another format) the load is simply unknown, which is not an error.
package sdlog

import (
	"math"
	"regexp"
	"strconv"
)

var loadLine = regexp.MustCompile(`loading tensors completed, taking ([0-9]+(?:\.[0-9]+)?) ?(ms|s)\b`)

// LoadMillis returns the milliseconds sd spent loading model weights in one invocation, summed over every component
// it loaded; ok is false when the output holds no readable load time.
func LoadMillis(out []byte) (ms int, ok bool) {
	var total float64
	for _, m := range loadLine.FindAllSubmatch(out, -1) {
		v, err := strconv.ParseFloat(string(m[1]), 64)
		if err != nil {
			continue
		}
		if string(m[2]) == "s" {
			v *= 1000
		}
		total += v
		ok = true
	}
	return int(math.Round(total)), ok
}
