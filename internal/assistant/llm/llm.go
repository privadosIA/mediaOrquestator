// Package llm is the one interface the application uses to talk to a language or
// vision model, whoever serves it: a local or remote Ollama, or any server that
// speaks the OpenAI chat completions protocol (OpenAI, OpenRouter, Together,
// Groq, vLLM, LM Studio, llama.cpp). What exists only for Ollama (staying on the
// CPU, keep-alive, unloading a model, asking where it is loaded) lives in the
// Ollama implementation and is never sent to another provider.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// Errors every provider can return; wrap them with %w so callers can tell them
// apart with errors.Is.
var (
	// ErrUnreachable means nobody answers at the address.
	ErrUnreachable = errors.New("ollama unreachable")
	// ErrKeyRejected means the provider refused the API key (401 or 403).
	ErrKeyRejected = errors.New("the provider rejected the API key")
	// ErrRateLimited means the provider asked to slow down (429).
	ErrRateLimited = errors.New("the provider asked to slow down")
	// ErrModelNotFound means the provider does not know the model (404 on a chat call).
	ErrModelNotFound = errors.New("the model was not found")
	// ErrNoVision means the model cannot read pictures.
	ErrNoVision = errors.New("the model cannot read pictures")
)

// Request is one chat call. Model names the model to use at that provider.
type Request struct {
	Model  string
	System string
	User   string
	// Images are PNG or JPEG bytes sent with the user message.
	Images [][]byte
	// JSON asks for a JSON answer.
	JSON        bool
	MaxTokens   int
	Temperature float64
	// Local holds options that only an Ollama understands; other providers ignore it.
	Local *LocalOptions
}

// LocalOptions are the Ollama-only options of a request.
type LocalOptions struct {
	// CPUOnly keeps the model off the GPUs (num_gpu 0).
	CPUOnly bool
	// KeepAlive says how long Ollama keeps the model loaded ("30s", "5m"); empty leaves Ollama's default.
	KeepAlive string
}

// Response is the answer of a chat call.
type Response struct {
	Text string
	// Truncated is true when generation stopped at the token limit.
	Truncated bool
}

// Model is a model a provider serves. Vision is nil when the provider cannot say.
type Model struct {
	Name      string
	SizeBytes int64
	Vision    *bool
}

// Provider is something that answers chat calls and lists its models.
type Provider interface {
	Chat(ctx context.Context, req Request) (Response, error)
	Models(ctx context.Context) ([]Model, error)
}

// Unloader is implemented by providers that hold models in memory and can drop them.
type Unloader interface {
	Unload(ctx context.Context, model string)
}

// maxErrBody is how much of a provider's body an error message may repeat.
const maxErrBody = 200

// redact replaces every occurrence of the secret in s.
func redact(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "***")
}

// clip cuts s to n runes and removes line breaks.
func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// redactNet drops the operation and address a network error repeats.
func redactNet(err error) error {
	var ne *net.OpError
	if errors.As(err, &ne) && ne.Err != nil {
		return ne.Err
	}
	return err
}

// doJSON sends a JSON body and decodes the JSON answer, returning the status and the
// raw body for the caller to classify a failure.
func doJSON(ctx context.Context, client *http.Client, method, url string, hdr map[string]string, body any, limit int64) (status int, data []byte, err error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, nil, ctxErr
		}
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ = io.ReadAll(io.LimitReader(resp.Body, limit))
	return resp.StatusCode, data, nil
}

func unreachable(host string, err error) error {
	return fmt.Errorf("%w: cannot reach Ollama at %s: %w", ErrUnreachable, host, redactNet(err))
}
