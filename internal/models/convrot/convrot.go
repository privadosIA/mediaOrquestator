// Package convrot turns a ComfyUI "int8_tensorwise + convrot" safetensors checkpoint into a plain BF16 one.
//
// Those checkpoints (the int8 files of Civitai) store every quantised linear layer as an I8 matrix Q, one F32 scale
// per output row and, when "convrot" is on, a weight that was rotated with a normalised regular Hadamard matrix R
// applied to every contiguous group of G input features. The original weight is W = (Q * scale) · R, because R is
// symmetric and orthogonal. stable-diffusion.cpp runs the int8 form directly, but its fast CUDA path exists only for
// G = 256 and the files in the wild use G = 64, which falls back to the CPU (about 26 minutes per picture). Undoing
// the rotation gives an ordinary BF16 file that `sd -M convert` can quantise to Q8_0 or Q4_K and that runs at normal
// speed (docs/int8_convrot.md of stable-diffusion.cpp has the format).
package convrot

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
)

const (
	scaleSuffix = ".weight_scale"
	quantSuffix = ".comfy_quant"
	rowChunk    = 512 // rows converted per batch; bounds the memory of one tensor
)

type tensorInfo struct {
	Dtype   string   `json:"dtype"`
	Shape   []int64  `json:"shape"`
	Offsets [2]int64 `json:"data_offsets"`
}

type layerConfig struct {
	Format    string `json:"format"`
	Convrot   bool   `json:"convrot"`
	GroupSize int    `json:"convrot_groupsize"`
}

// IsInt8Convrot reports whether the safetensors file at path declares int8 tensorwise layers, which is what
// Derotate converts; any other file (an fp8 or BF16 checkpoint) is left to sd as it is.
func IsInt8Convrot(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	_, meta, err := readHeader(f)
	return err == nil && len(meta) > 0
}

// Derotate writes src as a BF16 safetensors file at dst: I8 weights are dequantised and un-rotated, every other
// tensor is copied unchanged and the scale and quantisation-config tensors are dropped. progress, when not nil,
// receives the number of tensors done and the total.
func Derotate(ctx context.Context, src, dst string, progress func(done, total int)) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	hdr, layers, err := readHeader(in)
	if err != nil {
		return err
	}
	if len(layers) == 0 {
		return fmt.Errorf("%s has no int8_tensorwise layers", src)
	}
	base, err := dataStart(in)
	if err != nil {
		return err
	}

	// Output order follows the source offsets so both files are read and written sequentially.
	var names []string
	for k := range hdr {
		if !strings.HasSuffix(k, scaleSuffix) && !strings.HasSuffix(k, quantSuffix) {
			names = append(names, k)
		}
	}
	sort.Slice(names, func(i, j int) bool { return hdr[names[i]].Offsets[0] < hdr[names[j]].Offsets[0] })

	outHdr := make(map[string]tensorInfo, len(names))
	var off int64
	for _, k := range names {
		t := hdr[k]
		n := t.Offsets[1] - t.Offsets[0]
		if t.Dtype == "I8" {
			n *= 2
			t.Dtype = "BF16"
		}
		outHdr[k] = tensorInfo{Dtype: t.Dtype, Shape: t.Shape, Offsets: [2]int64{off, off + n}}
		off += n
	}
	hb, err := json.Marshal(outHdr)
	if err != nil {
		return err
	}
	for len(hb)%8 != 0 {
		hb = append(hb, ' ')
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(dst)
		}
	}()
	var lenBuf [8]byte
	binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(hb)))
	if _, err = out.Write(lenBuf[:]); err != nil {
		return err
	}
	if _, err = out.Write(hb); err != nil {
		return err
	}

	for i, k := range names {
		if err = ctx.Err(); err != nil {
			return err
		}
		t := hdr[k]
		sec := io.NewSectionReader(in, base+t.Offsets[0], t.Offsets[1]-t.Offsets[0])
		if t.Dtype != "I8" {
			if _, err = io.Copy(out, sec); err != nil {
				return err
			}
		} else {
			layer := strings.TrimSuffix(k, ".weight")
			cfg, ok := layers[layer]
			if !ok || len(t.Shape) != 2 {
				return fmt.Errorf("tensor %s is I8 but has no int8_tensorwise configuration", k)
			}
			scale, serr := readScale(in, base, hdr, layer+scaleSuffix, int(t.Shape[0]))
			if serr != nil {
				return serr
			}
			g := 0
			if cfg.Convrot {
				g = cfg.GroupSize
				if g <= 0 || g&(g-1) != 0 || bitsLen(g)%2 == 0 || int(t.Shape[1])%g != 0 {
					return fmt.Errorf("tensor %s: unsupported convrot group size %d for %d columns", k, g, t.Shape[1])
				}
			}
			if err = derotateMatrix(ctx, sec, out, int(t.Shape[0]), int(t.Shape[1]), scale, g); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
		if progress != nil {
			progress(i+1, len(names))
		}
	}
	return nil
}

// bitsLen is the number of bits needed for n; a power of four 4^k has 2k+1 of them.
func bitsLen(n int) int {
	l := 0
	for ; n > 0; n >>= 1 {
		l++
	}
	return l
}

func derotateMatrix(ctx context.Context, r io.Reader, w io.Writer, rows, cols int, scale []float32, group int) error {
	buf := make([]byte, rowChunk*cols)
	res := make([]byte, rowChunk*cols*2)
	workers := runtime.NumCPU()
	for row := 0; row < rows; row += rowChunk {
		if err := ctx.Err(); err != nil {
			return err
		}
		n := min(rowChunk, rows-row)
		if _, err := io.ReadFull(r, buf[:n*cols]); err != nil {
			return err
		}
		var wg sync.WaitGroup
		per := (n + workers - 1) / workers
		for lo := 0; lo < n; lo += per {
			hi := min(lo+per, n)
			wg.Add(1)
			go func(lo, hi int) {
				defer wg.Done()
				tmp := make([]float32, cols)
				for i := lo; i < hi; i++ {
					s := scale[row+i]
					for j, q := range buf[i*cols : (i+1)*cols] {
						tmp[j] = float32(int8(q)) * s
					}
					if group > 0 {
						for g := 0; g < cols; g += group {
							hadamard(tmp[g : g+group])
						}
					}
					o := res[i*cols*2 : (i+1)*cols*2]
					for j, v := range tmp {
						binary.LittleEndian.PutUint16(o[j*2:], bf16(v))
					}
				}
			}(lo, hi)
		}
		wg.Wait()
		if _, err := w.Write(res[:n*cols*2]); err != nil {
			return err
		}
	}
	return nil
}

// hadamard applies, in place, the normalised Kronecker power of
//
//	H4 = 1/2 · [ 1  1  1 -1 ; 1  1 -1  1 ; 1 -1  1  1 ; -1  1  1  1 ]
//
// that fits len(v) (a power of four): H4 along each base-4 digit of the index, 1/2 each time.
func hadamard(v []float32) {
	n := len(v)
	for s := 1; s < n; s *= 4 {
		for b := 0; b < n; b += 4 * s {
			for i := b; i < b+s; i++ {
				a, c, d, e := v[i], v[i+s], v[i+2*s], v[i+3*s]
				v[i] = (a + c + d - e) / 2
				v[i+s] = (a + c - d + e) / 2
				v[i+2*s] = (a - c + d + e) / 2
				v[i+3*s] = (-a + c + d + e) / 2
			}
		}
	}
}

// bf16 rounds a float32 to the nearest BF16 (ties to even).
func bf16(f float32) uint16 {
	u := math.Float32bits(f)
	if u&0x7fffffff > 0x7f800000 { // NaN
		return uint16(u>>16) | 0x40
	}
	return uint16((u + 0x7fff + (u >> 16 & 1)) >> 16)
}

func readScale(f *os.File, base int64, hdr map[string]tensorInfo, name string, rows int) ([]float32, error) {
	t, ok := hdr[name]
	if !ok || t.Dtype != "F32" || (t.Offsets[1]-t.Offsets[0]) != int64(rows)*4 {
		return nil, fmt.Errorf("missing or malformed %s", name)
	}
	b := make([]byte, rows*4)
	if _, err := f.ReadAt(b, base+t.Offsets[0]); err != nil {
		return nil, err
	}
	s := make([]float32, rows)
	for i := range s {
		s[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return s, nil
}

func dataStart(f *os.File) (int64, error) {
	var b [8]byte
	if _, err := f.ReadAt(b[:], 0); err != nil {
		return 0, err
	}
	return 8 + int64(binary.LittleEndian.Uint64(b[:])), nil
}

// readHeader parses the safetensors header and the per-layer int8 configuration of its metadata (empty when the
// file declares none).
func readHeader(f *os.File) (map[string]tensorInfo, map[string]layerConfig, error) {
	var b [8]byte
	if _, err := f.ReadAt(b[:], 0); err != nil {
		return nil, nil, err
	}
	n := binary.LittleEndian.Uint64(b[:])
	if n == 0 || n > 1<<28 {
		return nil, nil, fmt.Errorf("not a safetensors file")
	}
	raw := make([]byte, n)
	if _, err := f.ReadAt(raw, 8); err != nil {
		return nil, nil, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, nil, fmt.Errorf("not a safetensors file: %w", err)
	}
	hdr := map[string]tensorInfo{}
	var layers map[string]layerConfig
	for k, v := range all {
		if k == "__metadata__" {
			var m map[string]string
			if json.Unmarshal(v, &m) == nil {
				var q struct {
					Layers map[string]layerConfig `json:"layers"`
				}
				if json.Unmarshal([]byte(m["_quantization_metadata"]), &q) == nil {
					layers = map[string]layerConfig{}
					for name, c := range q.Layers {
						if c.Format == "int8_tensorwise" {
							layers[name] = c
						}
					}
				}
			}
			continue
		}
		var t tensorInfo
		if err := json.Unmarshal(v, &t); err != nil {
			return nil, nil, fmt.Errorf("tensor %s: %w", k, err)
		}
		hdr[k] = t
	}
	return hdr, layers, nil
}
