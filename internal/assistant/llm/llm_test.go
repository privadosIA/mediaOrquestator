package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

var png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

// recorder is a server that records every request and answers with a script.
type recorder struct {
	mu     sync.Mutex
	bodies []map[string]any
	auth   []string
	paths  []string
	answer func(n int, body map[string]any, w http.ResponseWriter)
}

func (r *recorder) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(req.Body).Decode(&body)
		r.mu.Lock()
		r.bodies = append(r.bodies, body)
		r.auth = append(r.auth, req.Header.Get("Authorization"))
		r.paths = append(r.paths, req.URL.Path)
		n := len(r.bodies)
		r.mu.Unlock()
		r.answer(n, body, w)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func reply(w http.ResponseWriter, content, finish string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": content}, "finish_reason": finish}}})
}

func TestOpenAI_TextChat(t *testing.T) {
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) { reply(w, "hello", "stop") }}
	srv := rec.server(t)
	o := NewOpenAI(srv.URL, "sk-secret")
	res, err := o.Chat(context.Background(), Request{Model: "gpt-x", System: "be brief", User: "hi", MaxTokens: 50, Temperature: 0.3})
	if err != nil || res.Text != "hello" || res.Truncated {
		t.Fatalf("unexpected %+v %v", res, err)
	}
	b := rec.bodies[0]
	msgs := b["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || msgs[1].(map[string]any)["content"] != "hi" {
		t.Errorf("messages: %v", msgs)
	}
	if b["model"] != "gpt-x" || b["max_tokens"] != float64(50) || b["temperature"] != 0.3 {
		t.Errorf("body: %v", b)
	}
	if rec.auth[0] != "Bearer sk-secret" || rec.paths[0] != "/v1/chat/completions" {
		t.Errorf("auth %q path %q", rec.auth[0], rec.paths[0])
	}
}

func TestOpenAI_PictureIsADataURI(t *testing.T) {
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) { reply(w, "ok", "stop") }}
	srv := rec.server(t)
	_, err := NewOpenAI(srv.URL, "").Chat(context.Background(), Request{Model: "v", User: "describe", Images: [][]byte{png}})
	if err != nil {
		t.Fatal(err)
	}
	user := rec.bodies[0]["messages"].([]any)[0].(map[string]any)
	parts := user["content"].([]any)
	img := parts[1].(map[string]any)
	url := img["image_url"].(map[string]any)["url"].(string)
	if parts[0].(map[string]any)["type"] != "text" || img["type"] != "image_url" || !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Errorf("parts: %v", parts)
	}
	if rec.auth[0] != "" {
		t.Errorf("no key, no Authorization header, got %q", rec.auth[0])
	}
}

func TestOpenAI_JSONOptionAndRetryWithoutIt(t *testing.T) {
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) {
		if _, has := b["response_format"]; has {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"response_format is not supported"}}`))
			return
		}
		reply(w, `{"a":1}`, "stop")
	}}
	srv := rec.server(t)
	res, err := NewOpenAI(srv.URL, "").Chat(context.Background(), Request{Model: "m", User: "json please", JSON: true})
	if err != nil || res.Text != `{"a":1}` {
		t.Fatalf("%+v %v", res, err)
	}
	if len(rec.bodies) != 2 {
		t.Fatalf("one retry expected, got %d requests", len(rec.bodies))
	}
	if _, has := rec.bodies[0]["response_format"]; !has {
		t.Error("the first request must ask for json_object")
	}
	if _, has := rec.bodies[1]["response_format"]; has {
		t.Error("the retry must not carry response_format")
	}
}

func TestOpenAI_JSONSupportedIsOneRequest(t *testing.T) {
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) { reply(w, "{}", "stop") }}
	srv := rec.server(t)
	if _, err := NewOpenAI(srv.URL, "").Chat(context.Background(), Request{Model: "m", User: "x", JSON: true}); err != nil {
		t.Fatal(err)
	}
	rf := rec.bodies[0]["response_format"].(map[string]any)
	if len(rec.bodies) != 1 || rf["type"] != "json_object" {
		t.Errorf("one request with json_object, got %d %v", len(rec.bodies), rf)
	}
}

func TestOpenAI_Truncated(t *testing.T) {
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) { reply(w, "cut off", "length") }}
	srv := rec.server(t)
	res, _ := NewOpenAI(srv.URL, "").Chat(context.Background(), Request{Model: "m", User: "x"})
	if !res.Truncated {
		t.Error("finish_reason length must be truncated")
	}
}

func TestOpenAI_FailuresSayWhatToDo(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusUnauthorized:    ErrKeyRejected,
		http.StatusForbidden:       ErrKeyRejected,
		http.StatusTooManyRequests: ErrRateLimited,
		http.StatusNotFound:        ErrModelNotFound,
	} {
		rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
		}}
		srv := rec.server(t)
		_, err := NewOpenAI(srv.URL, "k").Chat(context.Background(), Request{Model: "m", User: "x"})
		if !errors.Is(err, want) {
			t.Errorf("status %d: want %v, got %v", status, want, err)
		}
	}
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) { w.WriteHeader(500) }}
	srv := rec.server(t)
	if _, err := NewOpenAI(srv.URL, "").Chat(context.Background(), Request{Model: "m", User: "x"}); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("a 500 must say the status: %v", err)
	}
}

func TestOpenAI_ErrorsRedactTheKeyAndCutTheBody(t *testing.T) {
	const key = "sk-super-secret-key"
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("bad key " + key + " " + strings.Repeat("x", 1000)))
	}}
	srv := rec.server(t)
	_, err := NewOpenAI(srv.URL, key).Chat(context.Background(), Request{Model: "m", User: "x"})
	if err == nil || strings.Contains(err.Error(), key) {
		t.Fatalf("the key must never be in an error: %v", err)
	}
	if !strings.Contains(err.Error(), "***") || len(err.Error()) > 400 {
		t.Errorf("the body must be redacted and cut: %q", err.Error())
	}
	// an error object inside a 200
	rec2 := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"error":{"message":"quota for ` + key + ` exceeded"}}`))
	}}
	srv2 := rec2.server(t)
	_, err = NewOpenAI(srv2.URL, key).Chat(context.Background(), Request{Model: "m", User: "x"})
	if err == nil || strings.Contains(err.Error(), key) {
		t.Fatalf("the key must never be in an error: %v", err)
	}
}

func TestOpenAI_BaseAddressForms(t *testing.T) {
	for base, want := range map[string]string{
		"https://openrouter.ai/api/v1":    "https://openrouter.ai/api/v1/chat/completions",
		"https://api.groq.com/openai/v1/": "https://api.groq.com/openai/v1/chat/completions",
		"http://127.0.0.1:8000":           "http://127.0.0.1:8000/v1/chat/completions",
		"https://api.openai.com/v1":       "https://api.openai.com/v1/chat/completions",
	} {
		if got := NewOpenAI(base, "").URL("/chat/completions"); got != want {
			t.Errorf("%q: %q, want %q", base, got, want)
		}
	}
}

func TestOpenAI_DoesNotFollowRedirectsNorSendTheKeyThere(t *testing.T) {
	var gotKey string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Authorization")
		reply(w, "leaked", "stop")
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	_, err := NewOpenAI(redirector.URL, "sk-key").Chat(context.Background(), Request{Model: "m", User: "x"})
	if err == nil || gotKey != "" {
		t.Fatalf("a redirect must fail without reaching the target: err=%v key=%q", err, gotKey)
	}
}

func TestOpenAI_NoOllamaOnlyOptions(t *testing.T) {
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) { reply(w, "ok", "stop") }}
	srv := rec.server(t)
	_, err := NewOpenAI(srv.URL, "").Chat(context.Background(), Request{Model: "m", User: "x", Local: &LocalOptions{CPUOnly: true, KeepAlive: "30s"}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(rec.bodies[0])
	for _, bad := range []string{"num_gpu", "keep_alive", "options", "think"} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("%s must not be sent to an OpenAI-compatible provider: %s", bad, raw)
		}
	}
}

func TestOpenAI_Models(t *testing.T) {
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"},{"id":"gpt-4o-mini"}]}`))
	}}
	srv := rec.server(t)
	models, err := NewOpenAI(srv.URL, "sk-k").Models(context.Background())
	if err != nil || len(models) != 2 || models[0].Name != "gpt-4o" || models[0].Vision != nil {
		t.Fatalf("%+v %v", models, err)
	}
	if rec.auth[0] != "Bearer sk-k" || rec.paths[0] != "/v1/models" {
		t.Errorf("auth %q path %q", rec.auth[0], rec.paths[0])
	}
	// a rejected key
	bad := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) { w.WriteHeader(401) }}
	srv2 := bad.server(t)
	if _, err := NewOpenAI(srv2.URL, "x").Models(context.Background()); !errors.Is(err, ErrKeyRejected) {
		t.Errorf("want ErrKeyRejected, got %v", err)
	}
}

func TestOpenAI_Unreachable(t *testing.T) {
	_, err := NewOpenAI("http://127.0.0.1:1", "k").Chat(context.Background(), Request{Model: "m", User: "x"})
	if !errors.Is(err, ErrUnreachable) {
		t.Errorf("want ErrUnreachable, got %v", err)
	}
}

// --- Ollama ---

func TestOllama_ChatBody(t *testing.T) {
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"content": "hi"}, "done_reason": "length"})
	}}
	srv := rec.server(t)
	res, err := NewOllama(srv.URL).Chat(context.Background(), Request{
		Model: "gemma4:26b", System: "sys", User: "usr", Images: [][]byte{png}, JSON: true,
		MaxTokens: 120, Temperature: 0.2, Local: &LocalOptions{CPUOnly: true, KeepAlive: "30s"},
	})
	if err != nil || res.Text != "hi" || !res.Truncated {
		t.Fatalf("%+v %v", res, err)
	}
	b := rec.bodies[0]
	opts := b["options"].(map[string]any)
	if b["model"] != "gemma4:26b" || b["stream"] != false || b["think"] != false || b["keep_alive"] != "30s" || b["format"] != "json" {
		t.Errorf("body: %v", b)
	}
	if opts["num_gpu"] != float64(0) || opts["num_predict"] != float64(120) || opts["temperature"] != 0.2 {
		t.Errorf("options: %v", opts)
	}
	msgs := b["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || len(msgs[1].(map[string]any)["images"].([]any)) != 1 {
		t.Errorf("messages: %v", msgs)
	}
	if rec.paths[0] != "/api/chat" {
		t.Errorf("path %q", rec.paths[0])
	}
}

func TestOllama_NoSystemNoOptionsWhenNotAsked(t *testing.T) {
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"content": "x"}})
	}}
	srv := rec.server(t)
	if _, err := NewOllama(srv.URL).Chat(context.Background(), Request{Model: "m", User: "u"}); err != nil {
		t.Fatal(err)
	}
	b := rec.bodies[0]
	if len(b["messages"].([]any)) != 1 {
		t.Errorf("no system message when empty: %v", b["messages"])
	}
	for _, k := range []string{"keep_alive", "format"} {
		if _, has := b[k]; has {
			t.Errorf("%s must be absent: %v", k, b)
		}
	}
	if _, has := b["options"].(map[string]any)["num_gpu"]; has {
		t.Error("num_gpu only with CPUOnly")
	}
}

func TestOllama_ModelsAndLoadedAndUnreachable(t *testing.T) {
	var unloaded []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"b:1","size":20},{"name":"a:1","size":10}]}`))
		case "/api/show":
			var req struct{ Model string }
			_ = json.NewDecoder(r.Body).Decode(&req)
			caps := `["completion"]`
			if req.Model == "a:1" {
				caps = `["completion","vision"]`
			}
			_, _ = w.Write([]byte(`{"capabilities":` + caps + `}`))
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[{"name":"a:1","size_vram":1048576}]}`))
		case "/api/generate":
			var req struct{ Model string }
			_ = json.NewDecoder(r.Body).Decode(&req)
			unloaded = append(unloaded, req.Model)
			_, _ = w.Write([]byte("{}"))
		}
	}))
	defer srv.Close()
	o := NewOllama(srv.URL)
	models, err := o.Models(context.Background())
	if err != nil || len(models) != 2 || models[0].Name != "a:1" || models[0].Vision == nil || !*models[0].Vision || *models[1].Vision {
		t.Fatalf("smallest first with vision flags: %+v %v", models, err)
	}
	loaded, err := o.Loaded(context.Background())
	if err != nil || len(loaded) != 1 || loaded[0].SizeVRAM != 1048576 {
		t.Fatalf("%+v %v", loaded, err)
	}
	o.Unload(context.Background(), "a:1")
	if len(unloaded) != 1 || unloaded[0] != "a:1" {
		t.Errorf("unloaded %v", unloaded)
	}
	if _, err := NewOllama("http://127.0.0.1:1").Chat(context.Background(), Request{Model: "m", User: "x"}); !errors.Is(err, ErrUnreachable) {
		t.Errorf("want ErrUnreachable, got %v", err)
	}
	if !SameModel("a:latest", "a") || SameModel("a:1", "a") || !SameModel("a:1", "a:1") {
		t.Error("SameModel")
	}
}

func TestOpenAI_AModelThatCannotReadPicturesSaysSo(t *testing.T) {
	rec := &recorder{answer: func(n int, b map[string]any, w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"This model does not support image input"}}`))
	}}
	srv := rec.server(t)
	_, err := NewOpenAI(srv.URL, "").Chat(context.Background(), Request{Model: "text-only", User: "describe", Images: [][]byte{png}})
	if !errors.Is(err, ErrNoVision) || !strings.Contains(err.Error(), "reads pictures") {
		t.Fatalf("want ErrNoVision, got %v", err)
	}
	// the same 400 without a picture is a plain failure, not a vision error
	_, err = NewOpenAI(srv.URL, "").Chat(context.Background(), Request{Model: "m", User: "hi"})
	if errors.Is(err, ErrNoVision) {
		t.Errorf("without pictures it is not a vision error: %v", err)
	}
}

func TestOpenAIURLKeepsAGeminiStyleBase(t *testing.T) {
	cases := map[string]string{
		"https://generativelanguage.googleapis.com/v1beta/openai/": "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
		"https://api.openai.com/v1":                                "https://api.openai.com/v1/chat/completions",
		"http://127.0.0.1:8000":                                    "http://127.0.0.1:8000/v1/chat/completions",
	}
	for base, want := range cases {
		if got := NewOpenAI(base, "").URL("/chat/completions"); got != want {
			t.Errorf("%s: %s, want %s", base, got, want)
		}
	}
}
