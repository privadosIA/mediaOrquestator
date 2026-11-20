package promptfetch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"
)

type diffusionRow struct {
	ImageName    string  `parquet:"image_name"`
	Prompt       string  `parquet:"prompt"`
	Lyrics       string  `parquet:"lyrics"`
	PromptNSFW   float32 `parquet:"prompt_nsfw"`
	ImageNSFW    float32 `parquet:"image_nsfw"`
	Instrumental bool    `parquet:"is_instrumental"`
}

// writeParquet writes rows with LZ4_RAW (what DiffusionDB uses), as one row group, and returns the path.
func writeParquet(t *testing.T, rows []diffusionRow) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.parquet")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := parquet.NewGenericWriter[diffusionRow](f, parquet.Compression(&parquet.Lz4Raw))
	if _, err := w.Write(rows); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadParquet_NamedColumnsOnlyAcrossManyPages(t *testing.T) {
	var rows []diffusionRow
	for i := 0; i < 20000; i++ { // enough for several pages per column
		rows = append(rows, diffusionRow{ImageName: fmt.Sprintf("img-%d.png", i), Prompt: fmt.Sprintf("a castle number %d, oil painting", i),
			Lyrics: "LYRICS-MARKER " + strings.Repeat("la ", 20), PromptNSFW: float32(i%10) / 10, Instrumental: i%2 == 0})
	}
	path := writeParquet(t, rows)
	n := 0
	err := ReadParquet(path, []string{"prompt", "image_name", "prompt_nsfw", "is_instrumental"}, func(r Row) error {
		if len(r) != 4 {
			t.Fatalf("only the named columns: %v", r)
		}
		for _, v := range r {
			if s, ok := v.(string); ok && strings.Contains(s, "LYRICS-MARKER") {
				t.Fatal("a column that was not named was read")
			}
		}
		if r["prompt"] != fmt.Sprintf("a castle number %d, oil painting", n) || r["image_name"] != fmt.Sprintf("img-%d.png", n) ||
			r["prompt_nsfw"] != float64(float32(n%10)/10) || r["is_instrumental"] != (n%2 == 0) {
			t.Fatalf("row %d out of step: %v", n, r)
		}
		n++
		return nil
	})
	if err != nil || n != len(rows) {
		t.Fatalf("%d rows, %v", n, err)
	}
}

func TestReadParquet_ErrorsAndStopping(t *testing.T) {
	path := writeParquet(t, []diffusionRow{{Prompt: "a"}, {Prompt: "b"}})
	if err := ReadParquet(path, []string{"nope"}, func(Row) error { return nil }); err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("%v", err)
	}
	stop := fmt.Errorf("stop")
	if err := ReadParquet(path, []string{"prompt"}, func(Row) error { return stop }); err != stop {
		t.Fatalf("%v", err)
	}
	if err := ReadParquet(filepath.Join(t.TempDir(), "missing"), []string{"prompt"}, func(Row) error { return nil }); err == nil {
		t.Fatal("a missing file is an error")
	}
}
