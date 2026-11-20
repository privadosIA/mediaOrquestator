package convrot

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// kron4 builds the normalised Kronecker power of H4 of size 4^n as an explicit matrix, independent of hadamard().
func kron4(n int) [][]float64 {
	h4 := [][]float64{{1, 1, 1, -1}, {1, 1, -1, 1}, {1, -1, 1, 1}, {-1, 1, 1, 1}}
	r := [][]float64{{1}}
	for k := 0; k < n; k++ {
		m := len(r)
		next := make([][]float64, m*4)
		for i := range next {
			next[i] = make([]float64, m*4)
			for j := range next[i] {
				next[i][j] = r[i/4][j/4] * h4[i%4][j%4] / 2
			}
		}
		r = next
	}
	return r
}

func TestHadamardMatchesTheExplicitKroneckerPower(t *testing.T) {
	for n := 1; n <= 3; n++ { // groups of 4, 16 and 64
		r := kron4(n)
		size := len(r)
		v := make([]float32, size)
		for i := range v {
			v[i] = rand.Float32()*2 - 1
		}
		want := make([]float64, size)
		for i := range want {
			for j := range v {
				want[i] += r[i][j] * float64(v[j])
			}
		}
		hadamard(v)
		for i := range v {
			if math.Abs(float64(v[i])-want[i]) > 1e-5 {
				t.Fatalf("group %d, index %d: got %v want %v", size, i, v[i], want[i])
			}
		}
	}
}

func TestBF16RoundsToNearestEven(t *testing.T) {
	cases := map[float32]uint16{1: 0x3f80, -2: 0xc000, 0: 0, 1.00390625: 0x3f80 /* tie, even */, 1.01171875: 0x3f82 /* tie, to even */}
	for in, want := range cases {
		if got := bf16(in); got != want {
			t.Errorf("bf16(%v) = %#x, want %#x", in, got, want)
		}
	}
}

func writeST(t *testing.T, path string, meta map[string]string, tensors []struct {
	name, dtype string
	shape       []int64
	data        []byte
}) {
	t.Helper()
	hdr := map[string]any{}
	if meta != nil {
		hdr["__metadata__"] = meta
	}
	var off int64
	for _, x := range tensors {
		hdr[x.name] = tensorInfo{Dtype: x.dtype, Shape: x.shape, Offsets: [2]int64{off, off + int64(len(x.data))}}
		off += int64(len(x.data))
	}
	hb, _ := json.Marshal(hdr)
	for len(hb)%8 != 0 {
		hb = append(hb, ' ')
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var l [8]byte
	binary.LittleEndian.PutUint64(l[:], uint64(len(hb)))
	f.Write(l[:])
	f.Write(hb)
	for _, x := range tensors {
		f.Write(x.data)
	}
}

func TestDerotateRecoversTheOriginalWeights(t *testing.T) {
	const rows, cols, g = 600, 128, 64 // more rows than rowChunk to cross a batch boundary
	r := kron4(3)
	w := make([][]float64, rows)
	rotated := make([][]float64, rows)
	for i := range w {
		w[i] = make([]float64, cols)
		rotated[i] = make([]float64, cols)
		for j := range w[i] {
			w[i][j] = rand.NormFloat64()
		}
		for b := 0; b < cols; b += g {
			for o := 0; o < g; o++ {
				for k := 0; k < g; k++ {
					rotated[i][b+o] += w[i][b+k] * r[k][o] // W_rot = W R
				}
			}
		}
	}
	q := make([]byte, rows*cols)
	scale := make([]byte, rows*4)
	for i := range rotated {
		m := 0.0
		for _, v := range rotated[i] {
			m = math.Max(m, math.Abs(v))
		}
		s := float32(m / 127)
		binary.LittleEndian.PutUint32(scale[i*4:], math.Float32bits(s))
		for j, v := range rotated[i] {
			q[i*cols+j] = byte(int8(math.Round(v / float64(s))))
		}
	}
	norm := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "in.safetensors"), filepath.Join(dir, "out.safetensors")
	writeST(t, src, map[string]string{"_quantization_metadata": `{"layers":{"blk.lin":{"format":"int8_tensorwise","convrot":true,"convrot_groupsize":64}}}`},
		[]struct {
			name, dtype string
			shape       []int64
			data        []byte
		}{
			{"blk.lin.weight", "I8", []int64{rows, cols}, q},
			{"blk.lin.weight_scale", "F32", []int64{rows, 1}, scale},
			{"blk.lin.comfy_quant", "U8", []int64{2}, []byte{0, 0}},
			{"blk.norm.scale", "BF16", []int64{4}, norm},
		})
	if !IsInt8Convrot(src) {
		t.Fatal("the source must be detected as int8 convrot")
	}
	if err := Derotate(context.Background(), src, dst, nil); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(dst)
	defer f.Close()
	hdr, _, err := readHeader(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(hdr) != 2 || hdr["blk.lin.weight"].Dtype != "BF16" || hdr["blk.norm.scale"].Dtype != "BF16" {
		t.Fatalf("unexpected output tensors: %+v", hdr)
	}
	base, _ := dataStart(f)
	lin := hdr["blk.lin.weight"]
	raw := make([]byte, lin.Offsets[1]-lin.Offsets[0])
	f.ReadAt(raw, base+lin.Offsets[0])
	var errSum, refSum float64
	for i := 0; i < rows*cols; i++ {
		got := float64(math.Float32frombits(uint32(binary.LittleEndian.Uint16(raw[i*2:])) << 16))
		want := w[i/cols][i%cols]
		errSum += (got - want) * (got - want)
		refSum += want * want
	}
	if rel := math.Sqrt(errSum / refSum); rel > 0.02 { // int8 + bf16 rounding
		t.Errorf("relative error %.4f, want under 0.02", rel)
	}
	nt := hdr["blk.norm.scale"]
	got := make([]byte, 8)
	f.ReadAt(got, base+nt.Offsets[0])
	for i := range norm {
		if got[i] != norm[i] {
			t.Fatalf("a non-quantised tensor must be copied as it is: %v", got)
		}
	}
	if IsInt8Convrot(dst) {
		t.Error("the output must not look like an int8 file")
	}
}

func TestDerotateRejectsAFileWithoutInt8Layers(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plain.safetensors")
	writeST(t, src, nil, []struct {
		name, dtype string
		shape       []int64
		data        []byte
	}{{"a", "BF16", []int64{1}, []byte{0, 0}}})
	if IsInt8Convrot(src) {
		t.Error("a plain file is not int8 convrot")
	}
	if err := Derotate(context.Background(), src, filepath.Join(dir, "o"), nil); err == nil {
		t.Error("expected an error")
	}
}
