package motion

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// realBrowser skips the test when this machine has no Chrome or Chromium.
func realBrowser(t *testing.T) string {
	t.Helper()
	b, err := Find("")
	if err != nil {
		t.Skip("no Chrome or Chromium on this machine")
	}
	return b.Path
}

func needFFmpeg(t *testing.T) {
	t.Helper()
	for _, n := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(n); err != nil {
			t.Skipf("%s is not installed", n)
		}
	}
}

// slidePage moves a red box across a blue page in 2 seconds with a CSS animation.
const slidePage = `<!doctype html><html><head><style>
html,body{margin:0;background:#0033aa}
#b{position:absolute;top:20px;left:0;width:40px;height:40px;background:#f00;animation:go 2s linear forwards}
@keyframes go{from{left:0}to{left:150px}}
</style></head><body><div id="b"></div></body></html>`

func slide(transparent bool) Page {
	return Page{HTML: slidePage, Width: 200, Height: 80, Duration: 2, Transparent: transparent}
}

func TestSessionSeeksAnimationsByTimeNotByClock(t *testing.T) {
	b := realBrowser(t)
	s, err := Open(context.Background(), b, slide(false), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	f1, err := s.Frame(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	f0, _ := s.Frame(ctx, 0)
	f2, _ := s.Frame(ctx, 2)
	again, _ := s.Frame(ctx, 1) // going back in time gives the same frame
	if bytes.Equal(f0, f1) || bytes.Equal(f1, f2) {
		t.Fatal("the page does not change with time")
	}
	if !bytes.Equal(f1, again) {
		t.Fatal("the same time must give the same frame")
	}
	if p := s.Problems(); len(p) != 0 {
		t.Fatalf("unexpected problems: %v", p)
	}
}

func TestSessionFramesAreIdenticalAcrossSessionsAndUnderLoad(t *testing.T) {
	b := realBrowser(t)
	grab := func() []byte {
		s, err := Open(context.Background(), b, slide(false), Limits{})
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		f, err := s.Frame(context.Background(), 0.7)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	quiet := grab()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					runtime.Gosched()
				}
			}
		}()
	}
	busy := grab()
	close(stop)
	wg.Wait()
	if !bytes.Equal(quiet, busy) {
		t.Fatal("a busy machine changed the frame")
	}
}

func TestSessionBlocksTheNetworkAndFiles(t *testing.T) {
	b := realBrowser(t)
	page := Page{Width: 100, Height: 60, Duration: 1, HTML: `<!doctype html><html><body>
<img src="http://example.com/x.png"><script>fetch("file:///etc/passwd").catch(function(){});</script></body></html>`}
	s, err := Open(context.Background(), b, page, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Frame(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	p := strings.Join(s.Problems(), "\n")
	if !strings.Contains(p, "example.com") {
		t.Fatalf("the remote request must be reported as blocked, got %q", p)
	}
}

func TestSessionReportsScriptErrors(t *testing.T) {
	b := realBrowser(t)
	page := Page{Width: 100, Height: 60, Duration: 1, HTML: `<!doctype html><html><body><script>window.__seek=function(t){ if(t>0.5) throw new Error("boom at "+t); };</script></body></html>`}
	s, err := Open(context.Background(), b, page, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Frame(context.Background(), 0.9); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("a throwing page must fail the frame with its message, got %v", err)
	}
}

func TestSessionTimesOutAnEndlessLoopAndLeavesNoProcess(t *testing.T) {
	b := realBrowser(t)
	page := Page{Width: 100, Height: 60, Duration: 1, HTML: `<!doctype html><html><body><script>window.__seek=function(){ while(true){} };</script></body></html>`}
	s, err := Open(context.Background(), b, page, Limits{Frame: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	pid := s.cmd.Process.Pid
	_, err = s.Frame(context.Background(), 0.5)
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("an endless loop must time out with a clear message, got %v", err)
	}
	s.Close()
	profile := s.profile
	if _, err := os.Stat(profile); err == nil {
		t.Fatal("the profile must be deleted")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(-pid, 0) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the browser's process group is still running")
}

func probe(t *testing.T, file, entries string) string {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-count_frames", "-select_streams", "v:0", "-show_entries", entries, "-of", "csv=p=0", file).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestRenderWritesTheExpectedFramesAndDuration(t *testing.T) {
	b := realBrowser(t)
	needFFmpeg(t)
	out := filepath.Join(t.TempDir(), "scene.mp4")
	page := slide(false)
	page.Duration = 6.4
	n, problems, err := Render(context.Background(), Job{Page: page, FPS: 30, Browser: b, Out: out})
	if err != nil || len(problems) != 0 {
		t.Fatalf("render: %v %v", err, problems)
	}
	if n != 192 {
		t.Fatalf("6.4 s at 30 fps is 192 frames, got %d", n)
	}
	if got := probe(t, out, "stream=nb_read_frames"); got != "192" {
		t.Fatalf("the file has %s frames, want 192", got)
	}
	d, _ := strconv.ParseFloat(probe(t, out, "format=duration"), 64)
	if d < 6.3 || d > 6.5 {
		t.Fatalf("duration %v, want about 6.4", d)
	}
}

func TestRenderTransparentKeepsTheAlphaChannel(t *testing.T) {
	b := realBrowser(t)
	needFFmpeg(t)
	// A page whose background is transparent: only a red box is drawn.
	page := Page{Width: 200, Height: 80, Duration: 1, Transparent: true,
		HTML: `<!doctype html><html><body style="margin:0"><div style="width:40px;height:40px;background:#f00"></div></body></html>`}
	for _, ext := range []string{"webm", "mov"} {
		out := filepath.Join(t.TempDir(), "o."+ext)
		if _, _, err := Render(context.Background(), Job{Page: page, FPS: 10, Browser: b, Out: out}); err != nil {
			t.Fatalf("%s: %v", ext, err)
		}
		// Decode a pixel outside the box to a raw RGBA frame: it must be transparent, the box opaque.
		args := []string{"-v", "error"}
		if ext == "webm" {
			args = append(args, "-c:v", "libvpx-vp9") // the native decoder drops the alpha plane
		}
		args = append(args, "-i", out, "-frames:v", "1", "-pix_fmt", "rgba", "-f", "rawvideo", "-")
		raw, err := exec.Command("ffmpeg", args...).Output()
		if err != nil || len(raw) != 200*80*4 {
			t.Fatalf("%s: decode: %v (%d bytes)", ext, err, len(raw))
		}
		at := func(x, y int) byte { return raw[(y*200+x)*4+3] }
		if at(150, 60) != 0 || at(10, 10) != 255 {
			t.Fatalf("%s: alpha outside the box %d (want 0), inside %d (want 255)", ext, at(150, 60), at(10, 10))
		}
	}
}

func TestRenderRejectsATransparentFileWithoutAlpha(t *testing.T) {
	_, _, err := Render(context.Background(), Job{Page: slide(true), FPS: 10, Browser: "x", Out: "o.mp4"})
	if err == nil || !strings.Contains(err.Error(), ".webm") {
		t.Fatalf("got %v", err)
	}
}

// What the editor previews is what the renderer draws: a page prepared for the preview, moved
// by a postMessage like the editor's slider does, shows the same picture the renderer takes at
// that second.
func TestPreviewPageDrawsTheSameFrameAsTheRenderer(t *testing.T) {
	b := realBrowser(t)
	ctx := context.Background()
	render, err := Open(ctx, b, slide(false), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer render.Close()
	want, err := render.Frame(ctx, 1.3)
	if err != nil {
		t.Fatal(err)
	}

	prev := slide(false)
	prev.HTML = PreviewHTML(slidePage, 2)
	s, err := Open(ctx, b, prev, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// the slider's message, then a screenshot without the renderer's own seek
	if err := s.c.call(ctx, "Runtime.evaluate", map[string]any{
		"expression": `window.postMessage({type:"seek", t:1.3}, "*"); new Promise(function(r){ setTimeout(r, 50); })`, "awaitPromise": true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	var shot struct {
		Data string `json:"data"`
	}
	if err := s.c.call(ctx, "Page.captureScreenshot", map[string]any{"format": "png", "fromSurface": true}, &shot); err != nil {
		t.Fatal(err)
	}
	got, _ := base64.StdEncoding.DecodeString(shot.Data)
	if !bytes.Equal(got, want) {
		t.Fatal("the preview must draw the same frame the renderer takes at the same second")
	}
	if p := s.Problems(); len(p) != 0 {
		t.Fatalf("the preview page runs under its policy without problems: %v", p)
	}
}

// A desktop whose keyring is broken made Chrome wait for it before loading any page (every page timed out): the
// browser must be started with no keyring, and still with no network.
func TestBrowserArgsUseNoKeyringAndNoNetwork(t *testing.T) {
	args := strings.Join(browserArgs("/tmp/profile"), " ")
	for _, want := range []string{"--password-store=basic", "--user-data-dir=/tmp/profile", "--host-resolver-rules=MAP * ~NOTFOUND", "--disable-background-networking"} {
		if !strings.Contains(args, want) {
			t.Errorf("the browser arguments lack %q: %s", want, args)
		}
	}
}
