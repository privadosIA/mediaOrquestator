package describe

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const goodText = "A photorealistic portrait of one adult woman in her late twenties with long wavy auburn hair, wearing a black lace bra and matching panties, sitting on the edge of an unmade bed in a dim bedroom, soft warm window light from the left, shallow depth of field, medium shot, confident smile, editorial boudoir style."

// fakeOllama answers the calls describe makes. age is the JSON the age check
// returns; describeReply the description.
type fakeOllama struct {
	mu            sync.Mutex
	age           string
	describeReply string
	mergeReply    string
	installed     map[string]bool // name -> vision
	unloaded      []string
	chats         []map[string]any
}

func newFake() *fakeOllama {
	return &fakeOllama{
		age:           `{"people":1,"minor":false,"unclear":false,"medium":"photo"}`,
		describeReply: goodText,
		installed:     map[string]bool{"gemma4:26b": true, "muse-glimmer:30b": true, "qwen3:8b": false},
	}
}

func (f *fakeOllama) server(t *testing.T) *Describer {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		var ms []map[string]any
		for n := range f.installed {
			ms = append(ms, map[string]any{"name": n, "size": int64(10 << 30)})
		}
		json.NewEncoder(w).Encode(map[string]any{"models": ms})
	})
	mux.HandleFunc("/api/show", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Model string }
		json.NewDecoder(r.Body).Decode(&req)
		caps := []string{"completion"}
		if f.installed[req.Model] {
			caps = append(caps, "vision")
		}
		json.NewEncoder(w).Encode(map[string]any{"capabilities": caps})
	})
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.chats = append(f.chats, req)
		f.mu.Unlock()
		msgs := req["messages"].([]any)
		system := msgs[0].(map[string]any)["content"].(string)
		reply := f.describeReply
		switch {
		case strings.HasPrefix(system, "You check a picture"):
			reply = f.age
		case strings.HasPrefix(system, "You merge"):
			reply = f.mergeReply
		case strings.HasPrefix(system, "You write motion prompts"):
			reply = "slow push in while she smiles"
		}
		json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": reply}})
	})
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model     string
			KeepAlive any `json:"keep_alive"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.unloaded = append(f.unloaded, req.Model)
		f.mu.Unlock()
		w.Write([]byte("{}"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Describer{
		Host:         srv.URL,
		HTTP:         srv.Client(),
		MemAvailable: func() (int64, error) { return 64 << 30, nil },
		Frames: func(ctx context.Context, path string, kind Kind) ([][]byte, error) {
			if kind == KindVideo {
				return [][]byte{{1}, {2}, {3}, {4}}, nil
			}
			return [][]byte{{1}}, nil
		},
	}
}

func file(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDescribeImage(t *testing.T) {
	f := newFake()
	d := f.server(t)
	res, err := d.Describe(context.Background(), file(t, "a.png"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Prompt != goodText || res.Model != DefaultModel || res.Kind != KindImage || res.Frames != 1 {
		t.Fatalf("unexpected result %+v", res)
	}
	if res.Motion != "" {
		t.Fatalf("an image has no motion prompt, got %q", res.Motion)
	}
	// The instructions must not ask the model to soften anything.
	last := f.chats[len(f.chats)-1]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if privateEdition && !strings.Contains(last, "never omit, soften or censor") {
		t.Fatalf("the description instructions lost the literal rule: %q", last)
	}
	if !privateEdition && (strings.Contains(last, "censor") || strings.Contains(last, "soften") || strings.Contains(last, "literally")) {
		t.Fatalf("the public instructions must not ask for a literal description: %q", last)
	}
	if len(f.unloaded) == 0 || f.unloaded[len(f.unloaded)-1] != DefaultModel {
		t.Fatalf("the model was not unloaded: %v", f.unloaded)
	}
}

func TestDescribeUnloadsOnFailure(t *testing.T) {
	f := newFake()
	f.age = `{"people":1,"minor":true,"unclear":false}`
	d := f.server(t)
	if _, err := d.Describe(context.Background(), file(t, "a.png"), Options{}); !errors.Is(err, ErrMinor) {
		t.Fatalf("want ErrMinor, got %v", err)
	}
	if len(f.unloaded) == 0 {
		t.Fatal("the model was not unloaded after a refusal")
	}
}

func TestDescribeMinorKinds(t *testing.T) {
	for name, age := range map[string]string{
		"minor":   `{"people":2,"minor":true,"unclear":false,"medium":"anime"}`,
		"unclear": `{"people":1,"minor":false,"unclear":true}`,
		"garbage": `not json at all`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			f.age = age
			d := f.server(t)
			res, err := d.Describe(context.Background(), file(t, "a.png"), Options{})
			if !errors.Is(err, ErrMinor) {
				t.Fatalf("want ErrMinor, got %v", err)
			}
			if res.Prompt != "" {
				t.Fatal("a refused file must return no prompt")
			}
			// The description must never be requested for a refused file.
			for _, c := range f.chats {
				sys := c["messages"].([]any)[0].(map[string]any)["content"].(string)
				if strings.HasPrefix(sys, "You write prompts") {
					t.Fatal("a description was requested for a refused file")
				}
			}
		})
	}
}

func TestDescribeNoPeopleIsFine(t *testing.T) {
	f := newFake()
	f.age = `{"people":0,"minor":false,"unclear":false,"medium":"photo"}`
	d := f.server(t)
	if _, err := d.Describe(context.Background(), file(t, "a.jpg"), Options{}); err != nil {
		t.Fatal(err)
	}
}

func TestDescribeAgeCheckRefused(t *testing.T) {
	f := newFake()
	f.age = "I'm sorry, I can't help with that."
	d := f.server(t)
	_, err := d.Describe(context.Background(), file(t, "a.png"), Options{})
	var re *RefusedError
	if !errors.Is(err, ErrRefused) || !errors.As(err, &re) {
		t.Fatalf("want a RefusedError, got %v", err)
	}
	if re.Model != DefaultModel {
		t.Fatalf("the refusal must name the model: %+v", re)
	}
	if privateEdition && (len(re.Alternatives) != 1 || re.Alternatives[0] != detailModel) {
		t.Fatalf("the private refusal lists the other vision models: %+v", re)
	}
	if !privateEdition && (len(re.Alternatives) != 0 || len(re.Choices) != 0) {
		t.Fatalf("the public refusal suggests nothing: %+v", re)
	}
}

func TestDescribeRefusal(t *testing.T) {
	for name, reply := range map[string]string{
		"sorry": "I'm sorry, but I can't describe this image.",
		"short": "A woman.",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			f.describeReply = reply
			d := f.server(t)
			res, err := d.Describe(context.Background(), file(t, "a.png"), Options{})
			var re *RefusedError
			if !errors.As(err, &re) || res.Prompt != "" {
				t.Fatalf("want a RefusedError and no prompt, got %v / %q", err, res.Prompt)
			}
			if privateEdition && !strings.Contains(err.Error(), detailModel) {
				t.Fatalf("the message must list the alternatives: %v", err)
			}
			if !privateEdition && strings.Contains(err.Error(), "try another") {
				t.Fatalf("the public message must not suggest another model: %v", err)
			}
		})
	}
}

func TestDescribeEmptyAnswer(t *testing.T) {
	f := newFake()
	f.describeReply = "   "
	d := f.server(t)
	if _, err := d.Describe(context.Background(), file(t, "a.png"), Options{}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("want ErrEmpty, got %v", err)
	}
}

func TestModelChoice(t *testing.T) {
	f := newFake()
	d := f.server(t)

	res, err := d.Describe(context.Background(), file(t, "a.png"), Options{Detail: true})
	if privateEdition {
		if err != nil || res.Model != detailModel {
			t.Fatalf("detail should pick %s, got %q (%v)", detailModel, res.Model, err)
		}
		res, err = d.Describe(context.Background(), file(t, "a.png"), Options{Model: "muse-glimmer:30b", Detail: false})
		if err != nil || res.Model != "muse-glimmer:30b" {
			t.Fatalf("named model not used: %q (%v)", res.Model, err)
		}
	} else if err != nil || res.Model != DefaultModel {
		t.Fatalf("the public edition ignores detail and uses %s, got %q (%v)", DefaultModel, res.Model, err)
	}

	delete(f.installed, "gemma4:26b")
	f.installed["qwen3.8:27b"] = true
	res, err = d.Describe(context.Background(), file(t, "a.png"), Options{})
	if err != nil || (res.Model != "qwen3.8:27b" && res.Model != "muse-glimmer:30b") {
		t.Fatalf("with the default missing the first vision model should be used, got %q (%v)", res.Model, err)
	}
}

func TestModelNotInstalled(t *testing.T) {
	f := newFake()
	d := f.server(t)
	_, err := d.Describe(context.Background(), file(t, "a.png"), Options{Model: "llava:7b"})
	if !errors.Is(err, ErrNotInstalled) || !strings.Contains(err.Error(), "ollama pull llava:7b") {
		t.Fatalf("want a not-installed error naming ollama pull, got %v", err)
	}
	if len(f.chats) != 0 {
		t.Fatal("no request may be sent for a missing model")
	}
	// A model that cannot read pictures counts as missing.
	if _, err := d.Describe(context.Background(), file(t, "a.png"), Options{Model: "qwen3:8b"}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("a text-only model must be refused, got %v", err)
	}
}

func TestNoVisionModelAtAll(t *testing.T) {
	f := newFake()
	f.installed = map[string]bool{"qwen3:8b": false}
	d := f.server(t)
	if _, err := d.Describe(context.Background(), file(t, "a.png"), Options{}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("want ErrNotInstalled, got %v", err)
	}
}

func TestMemoryCheck(t *testing.T) {
	f := newFake()
	d := f.server(t)
	d.MemAvailable = func() (int64, error) { return 2 << 30, nil }
	if _, err := d.Describe(context.Background(), file(t, "a.png"), Options{}); !errors.Is(err, ErrNoMemory) {
		t.Fatalf("want ErrNoMemory, got %v", err)
	}
	if len(f.chats) != 0 {
		t.Fatal("nothing may be sent when memory is short")
	}
}

func TestMemoryRuleDependsOnWhereTheModelRuns(t *testing.T) {
	f := newFake()
	d := f.server(t)
	d.MemAvailable = func() (int64, error) { return 8 << 30, nil }
	if _, err := d.Describe(context.Background(), file(t, "a.png"), Options{}); err != nil {
		t.Fatalf("on a GPU only the margin must be free: %v", err)
	}
	d.CPU = true
	if _, err := d.Describe(context.Background(), file(t, "a.png"), Options{}); !errors.Is(err, ErrNoMemory) {
		t.Fatalf("on the CPU the model needs size x 1.3 + margin, got %v", err)
	}
}

func TestDescribeVideo(t *testing.T) {
	f := newFake()
	f.mergeReply = `{"prompt":"` + goodText + `","motion":"slow push in, hair moves slightly in the breeze"}`
	d := f.server(t)
	res, err := d.Describe(context.Background(), file(t, "c.mp4"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindVideo || res.Frames != 4 || res.Motion == "" || res.Prompt != goodText {
		t.Fatalf("unexpected result %+v", res)
	}
	ages := 0
	for _, c := range f.chats {
		if strings.HasPrefix(c["messages"].([]any)[0].(map[string]any)["content"].(string), "You check a picture") {
			ages++
		}
	}
	if ages != 4 {
		t.Fatalf("every frame needs an age check, got %d", ages)
	}
}

func TestVideoOneMinorFrameRefusesAll(t *testing.T) {
	f := newFake()
	d := f.server(t)
	calls := 0
	// The age answer changes on the third check.
	srvAge := f.age
	_ = srvAge
	d.Frames = func(ctx context.Context, path string, kind Kind) ([][]byte, error) {
		return [][]byte{{1}, {2}, {3}, {4}, {5}, {6}}, nil
	}
	orig := d.HTTP.Transport
	d.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/chat" {
			calls++
			if calls == 3 {
				f.mu.Lock()
				f.age = `{"people":1,"minor":true,"unclear":false}`
				f.mu.Unlock()
			}
		}
		if orig == nil {
			return http.DefaultTransport.RoundTrip(r)
		}
		return orig.RoundTrip(r)
	})}
	_, err := d.Describe(context.Background(), file(t, "c.mp4"), Options{})
	if !errors.Is(err, ErrMinor) || !strings.Contains(err.Error(), "frame 3 of 6") {
		t.Fatalf("one minor frame must refuse the clip and say which: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVideoMergeFallback(t *testing.T) {
	f := newFake()
	f.mergeReply = "not json"
	d := f.server(t)
	res, err := d.Describe(context.Background(), file(t, "c.webm"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Prompt != goodText || res.Motion != "" {
		t.Fatalf("without a merge the first frame's own description stands in: %+v", res)
	}
}

func TestInvalidFiles(t *testing.T) {
	d := newFake().server(t)
	if _, err := d.Describe(context.Background(), file(t, "notes.txt"), Options{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("a text file must be refused, got %v", err)
	}
	if _, err := d.Describe(context.Background(), filepath.Join(t.TempDir(), "gone.png"), Options{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("a missing file must be refused, got %v", err)
	}
}

func TestFrameCount(t *testing.T) {
	for secs, want := range map[float64]int{1: 4, 20: 5, 31: 7, 200: 8} {
		if got := frameCount(secs); got != want {
			t.Errorf("frameCount(%v) = %d, want %d", secs, got, want)
		}
	}
}

func TestCPUOption(t *testing.T) {
	f := newFake()
	d := f.server(t)
	d.CPU = true
	if _, err := d.Describe(context.Background(), file(t, "a.png"), Options{}); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.chats {
		if c["options"].(map[string]any)["num_gpu"] != float64(0) {
			t.Fatalf("CPU mode must send num_gpu 0: %v", c["options"])
		}
	}
}

func TestListModels(t *testing.T) {
	d := newFake().server(t)
	def, detail, models, err := d.ListModels(context.Background())
	want := 1
	if privateEdition {
		want = 2
	}
	if err != nil || def != DefaultModel || detail != detailModel || len(models) != want {
		t.Fatalf("unexpected list: %q %q %v %v", def, detail, models, err)
	}
}

func TestUnreachable(t *testing.T) {
	d := &Describer{Host: "http://127.0.0.1:1", MemAvailable: func() (int64, error) { return 64 << 30, nil }}
	if _, err := d.Describe(context.Background(), file(t, "a.png"), Options{}); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("want ErrUnreachable, got %v", err)
	}
}

// The exact request Ollama receives must not change when the transport moves
// behind internal/assistant/llm: age check as JSON, description as free text, both with the
// picture, temperature 0.2, kept loaded 5 minutes, and no num_gpu unless CPU is asked.
func TestRequestBodiesAreUnchanged(t *testing.T) {
	f := newFake()
	d := f.server(t)
	if _, err := d.Describe(context.Background(), file(t, "a.png"), Options{}); err != nil {
		t.Fatal(err)
	}
	var age, desc map[string]any
	for _, c := range f.chats {
		sys := c["messages"].([]any)[0].(map[string]any)["content"].(string)
		switch {
		case strings.HasPrefix(sys, "You check a picture"):
			age = c
		case strings.HasPrefix(sys, "You write prompts"):
			desc = c
		}
	}
	if age == nil || desc == nil {
		t.Fatalf("both requests expected: %v", f.chats)
	}
	if age["format"] != "json" || desc["format"] != nil {
		t.Errorf("only the age check asks for JSON: age=%v desc=%v", age["format"], desc["format"])
	}
	for name, c := range map[string]map[string]any{"age": age, "desc": desc} {
		opts := c["options"].(map[string]any)
		if c["model"] != DefaultModel || c["stream"] != false || c["think"] != false || c["keep_alive"] != "5m" {
			t.Errorf("%s request: %v", name, c)
		}
		if opts["temperature"] != 0.2 {
			t.Errorf("%s options: %v", name, opts)
		}
		if _, has := opts["num_gpu"]; has {
			t.Errorf("%s: num_gpu only when the CPU is asked", name)
		}
		user := c["messages"].([]any)[1].(map[string]any)
		if len(user["images"].([]any)) != 1 {
			t.Errorf("%s: the picture must travel with the user message", name)
		}
	}
	if age["options"].(map[string]any)["num_predict"] != float64(120) || desc["options"].(map[string]any)["num_predict"] != float64(400) {
		t.Errorf("token limits changed: %v %v", age["options"], desc["options"])
	}
}
