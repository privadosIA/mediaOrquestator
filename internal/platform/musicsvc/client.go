// Package musicsvc is what the optional song generators that run as a small HTTP service in a container (HeartMuLa,
// DiffRhythm2) have in common: the client of the service, the container on this machine, and the engine that
// decides where songs are made and whether they can be. A generator is described by a Spec; nothing here starts
// unless a project asks for that generator.
package musicsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GPU is what the service says about its card.
type GPU struct {
	Name    string `json:"name"`
	FreeMB  int64  `json:"free_mb"`
	TotalMB int64  `json:"total_mb"`
}

// Health is the answer of GET /health.
type Health struct {
	Ready   bool     `json:"ready"`
	Version string   `json:"version"`
	Busy    bool     `json:"busy"`
	Queued  int      `json:"queued"`
	GPU     *GPU     `json:"gpu"`
	Missing []string `json:"missing"`
}

// Job is the answer of GET /jobs/{id}.
type Job struct {
	ID       string `json:"id"`
	State    string `json:"state"` // queued, running, done, failed
	Progress struct {
		Done  int `json:"done"`
		Total int `json:"total"`
	} `json:"progress"`
	Error   string  `json:"error"`
	Seconds float64 `json:"seconds"`
}

// Errors the caller tells apart.
var (
	ErrUnauthorized = errors.New("the service needs a token, or the token is wrong")
	ErrUnreachable  = errors.New("the service does not answer")
)

// namedError is an error with the text a person reads (it names the service) that still matches its kind.
type namedError struct {
	msg  string
	kind error
}

func (e *namedError) Error() string { return e.msg }
func (e *namedError) Unwrap() error { return e.kind }

// Client is a client of one service.
type Client struct {
	BaseURL string // http://host:port, no trailing slash
	Token   string
	HTTP    *http.Client
	// Service is the name of the generator in messages ("HeartMuLa"); empty says "music".
	Service string
}

// NewClient returns a client of the service of a generator for an address: a host:port or a URL.
func NewClient(service, address, token string) *Client {
	return &Client{Service: service, BaseURL: BaseURL(address), Token: token}
}

func (c *Client) name() string {
	if c.Service == "" {
		return "music"
	}
	return c.Service
}

// BaseURL turns a host, a host:port or a URL into a URL without a trailing slash.
func BaseURL(address string) string {
	a := strings.TrimRight(strings.TrimSpace(address), "/")
	if a != "" && !strings.Contains(a, "://") {
		a = "http://" + a
	}
	return a
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{} // every call carries a context with its own limit
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, ctx.Err()
		}
		return nil, &namedError{msg: fmt.Sprintf("the %s service does not answer (%s): %v", c.name(), c.BaseURL, err), kind: ErrUnreachable}
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		return nil, &namedError{msg: fmt.Sprintf("the %s service needs a token, or the token is wrong", c.name()), kind: ErrUnauthorized}
	}
	return resp, nil
}

func (c *Client) json(ctx context.Context, method, path string, body, out any, limit time.Duration) (int, error) {
	if limit > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, limit)
		defer cancel()
	}
	resp, err := c.do(ctx, method, path, body)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(data))
		}
		return resp.StatusCode, fmt.Errorf("the %s service answered %d: %s", c.name(), resp.StatusCode, e.Error)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("the %s service answered something that is not JSON: %w", c.name(), err)
		}
	}
	return resp.StatusCode, nil
}

// Call sends one JSON request to a path of the service and decodes the answer into out (nil to ignore it); limit
// bounds the call (zero leaves it to ctx). Errors name the service, as every call of the client does.
func (c *Client) Call(ctx context.Context, method, path string, body, out any, limit time.Duration) (int, error) {
	return c.json(ctx, method, path, body, out, limit)
}

// Health asks GET /health within three seconds.
func (c *Client) Health(ctx context.Context) (Health, error) {
	var h Health
	_, err := c.json(ctx, http.MethodGet, "/health", nil, &h, 3*time.Second)
	return h, err
}

// Generate queues a song and returns its job id.
func (c *Client) Generate(ctx context.Context, r any) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if _, err := c.json(ctx, http.MethodPost, "/generate", r, &out, 30*time.Second); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", fmt.Errorf("the %s service gave no job id", c.name())
	}
	return out.ID, nil
}

// Job asks for the state of a job.
func (c *Client) Job(ctx context.Context, id string) (Job, error) {
	var j Job
	_, err := c.json(ctx, http.MethodGet, "/jobs/"+id, nil, &j, 15*time.Second)
	return j, err
}

// Audio copies the WAV of a finished job into w.
func (c *Client) Audio(ctx context.Context, id string, w io.Writer) error {
	resp, err := c.do(ctx, http.MethodGet, "/jobs/"+id+"/audio", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("the %s service answered %d for the audio: %s", c.name(), resp.StatusCode, strings.TrimSpace(string(data)))
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

// Forget deletes a job and its audio on the service; failing to do it is not worth reporting.
func (c *Client) Forget(ctx context.Context, id string) {
	_, _ = c.json(ctx, http.MethodDelete, "/jobs/"+id, nil, nil, 5*time.Second)
}
