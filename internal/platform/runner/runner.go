// Package runner is the one place where the program starts other programs (sd, ffmpeg, piper, blender...). A
// [CommandRunner] runs a command under a context and returns its standard output; the real one puts the program in
// its own process group, so cancelling the context stops it together with everything it started, keeps the end of
// stderr for the error message and refuses a program that writes without end. [FakeRunner] stands for it in tests.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// CommandRunner is the single seam through which every subprocess call in the
// codebase must flow. Implementations must honour context cancellation, capture
// stderr into the returned error on non-zero exit (never discard it), support
// stdin piping, and allow per-invocation environment variable overrides.
type CommandRunner interface {
	// Run executes name with the given args under ctx.
	//
	// On success it returns the captured stdout and a nil error.
	// On non-zero exit (or any other failure) it returns nil stdout and a
	// non-nil error that always includes the captured stderr text.
	//
	// Behavioural scenarios (see openspec/runner):
	//
	//   - "success": exit 0 → (stdout, nil)
	//   - "non-zero-exit-with-stderr": exit ≠ 0 → (nil, err containing stderr)
	//   - "context-cancelled": ctx done before exit → (nil, ctx.Err() wrapped)
	//   - "stdin-piped": child reads stdin → data delivered via WithStdin
	//   - "env-override": child sees overridden vars → via WithEnv
	Run(ctx context.Context, name string, args []string, opts ...Option) (stdout []byte, err error)
}

// Option configures a single Run invocation.
type Option func(*runConfig)

type runConfig struct {
	stderr *bytes.Buffer
	stdin  []byte
	env    []string
	dir    string
}

// WithStdin pipes the supplied bytes into the child process's stdin.
// This is required by Piper and any other tool that reads from stdin.
func WithStdin(data []byte) Option {
	return func(c *runConfig) { c.stdin = data }
}

// WithStderr also copies the child's stderr into buf, so a caller can explain a
// program that exits 0 without having done its job (some write FATAL lines and
// still exit cleanly). On failure the stderr is in the error as always.
func WithStderr(buf *bytes.Buffer) Option {
	return func(c *runConfig) { c.stderr = buf }
}

// WithEnv appends environment variable overrides in KEY=VALUE form.
// Later entries take precedence over earlier ones and over the parent
// environment. This is required for CUDA_VISIBLE_DEVICES and similar.
func WithEnv(pairs ...string) Option {
	return func(c *runConfig) { c.env = append(c.env, pairs...) }
}

// WithDir sets the child process's working directory, so the relative
// paths the pipeline passes as arguments resolve against a project
// directory instead of the orchestrator process's own cwd. An empty dir
// keeps the parent's cwd (the CLI behavior).
func WithDir(dir string) Option {
	return func(c *runConfig) { c.dir = dir }
}

// ---------------------------------------------------------------------------
// execRunner – production implementation backed by os/exec
// ---------------------------------------------------------------------------

// execRunner is the real CommandRunner that spawns OS processes.
type execRunner struct{}

// NewExecRunner returns a CommandRunner backed by os/exec.
func NewExecRunner() CommandRunner {
	return &execRunner{}
}

// Limits of what Run keeps of a program's output and of what it writes into an error message.
const (
	// MaxStdout is the most stdout Run returns (the audio of a long song, a WAV from Piper). A program that writes more is
	// stopped and the call fails, instead of filling the memory.
	MaxStdout = 512 << 20
	// StderrTail is how much of the end of stderr Run keeps; a program that logs a lot keeps its last lines.
	StderrTail = 1 << 20
	// killDelay is how long Run waits, after cancelling a program, for its output pipes to close.
	killDelay = 5 * time.Second
	// maxArgInError is how much of one argument goes into an error message (a prompt can be many KB).
	maxArgInError = 200
)

// ErrOutputTooLarge is the error of a program that wrote more than MaxStdout.
var ErrOutputTooLarge = errors.New("the program wrote more output than allowed")

// limitedWriter collects up to max bytes and, on the first byte beyond, calls over once and stops collecting.
type limitedWriter struct {
	buf  bytes.Buffer
	max  int
	over func()
	hit  bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.hit {
		return len(p), nil
	}
	if w.buf.Len()+len(p) > w.max {
		w.hit = true
		w.over()
		return len(p), nil
	}
	return w.buf.Write(p)
}

// shortArgs joins args for an error message, cutting each one.
func shortArgs(args []string) string {
	cut := make([]string, len(args))
	for i, a := range args {
		if len(a) > maxArgInError {
			a = a[:maxArgInError] + "..."
		}
		cut[i] = a
	}
	return strings.Join(cut, " ")
}

func (r *execRunner) Run(ctx context.Context, name string, args []string, opts ...Option) ([]byte, error) {
	cfg := &runConfig{}
	for _, o := range opts {
		o(cfg)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	inOwnGroup(cmd)
	cmd.WaitDelay = killDelay
	if cfg.dir != "" {
		cmd.Dir = cfg.dir
	}

	// Environment: inherit parent, then apply overrides.
	if len(cfg.env) > 0 {
		cmd.Env = append(os.Environ(), cfg.env...)
	}

	// Stdin piping.
	if cfg.stdin != nil {
		cmd.Stdin = bytes.NewReader(cfg.stdin)
	}

	stdout := &limitedWriter{max: MaxStdout, over: cancel}
	stderr := &TailBuffer{Max: StderrTail}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if cfg.stderr != nil {
		cfg.stderr.Write(stderr.Bytes())
	}
	if stdout.hit {
		return nil, fmt.Errorf("runner: %s %s: %w (more than %d MB)", name, shortArgs(args), ErrOutputTooLarge, MaxStdout>>20)
	}
	if err != nil {
		// Always include captured stderr in the error; never return nil.
		stderrText := stderr.String()
		if stderrText != "" {
			return nil, fmt.Errorf("runner: %s %s: %w (stderr: %s)",
				name, shortArgs(args), err, stderrText)
		}
		return nil, fmt.Errorf("runner: %s %s: %w",
			name, shortArgs(args), err)
	}

	return stdout.buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// FakeRunner – testutil implementation for config-free unit tests
// ---------------------------------------------------------------------------

// Invocation records the details of a single Run call made through a
// FakeRunner, for later assertion in tests.
type Invocation struct {
	Name  string
	Args  []string
	Env   []string
	Stdin []byte
	Dir   string
}

// ScriptedResult is a canned response returned by a FakeRunner.
type ScriptedResult struct {
	Stdout []byte
	Err    error
}

// FakeRunner implements CommandRunner for unit tests. It records every
// invocation and returns pre-scripted results without spawning any real
// process, requiring no binaries, GPU, or configuration.
//
// Usage:
//
//	fr := runner.NewFakeRunner()
//	fr.Script(
//	    runner.ScriptedResult{Stdout: []byte("ok\n")},
//	    runner.ScriptedResult{Err: fmt.Errorf("exit status 1 (stderr: boom)")},
//	)
//	stdout, err := fr.Run(ctx, "nvidia-smi", nil)
//
// The scripts are consumed in FIFO order. Alternatively, set a Handler
// function for dynamic dispatch.
type FakeRunner struct {
	mu          sync.Mutex
	invocations []Invocation
	scripts     []ScriptedResult
	handler     func(Invocation) ScriptedResult
}

// NewFakeRunner returns a FakeRunner with no scripts queued.
// If Run is called with no script and no handler, it returns (nil, nil).
func NewFakeRunner() *FakeRunner {
	return &FakeRunner{}
}

// Script queues one or more results to be returned by successive Run calls,
// in FIFO order.
func (f *FakeRunner) Script(results ...ScriptedResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scripts = append(f.scripts, results...)
}

// SetHandler installs a dynamic dispatch function. When set, it takes
// precedence over the script queue.
func (f *FakeRunner) SetHandler(fn func(Invocation) ScriptedResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handler = fn
}

// Calls returns a copy of all recorded invocations in call order.
func (f *FakeRunner) Calls() []Invocation {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Invocation, len(f.invocations))
	copy(out, f.invocations)
	return out
}

// Reset clears all recorded invocations and queued scripts.
func (f *FakeRunner) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invocations = nil
	f.scripts = nil
	f.handler = nil
}

// Run implements CommandRunner.
func (f *FakeRunner) Run(_ context.Context, name string, args []string, opts ...Option) ([]byte, error) {
	cfg := &runConfig{}
	for _, o := range opts {
		o(cfg)
	}

	inv := Invocation{
		Name:  name,
		Args:  append([]string(nil), args...),
		Env:   append([]string(nil), cfg.env...),
		Stdin: append([]byte(nil), cfg.stdin...),
		Dir:   cfg.dir,
	}

	f.mu.Lock()
	f.invocations = append(f.invocations, inv)

	var result ScriptedResult
	handler := f.handler
	if handler == nil && len(f.scripts) > 0 {
		result = f.scripts[0]
		f.scripts = f.scripts[1:]
	}
	f.mu.Unlock()

	// The handler runs outside the lock so concurrent invocations (scenes on
	// different GPUs) can overlap, as real subprocesses do.
	if handler != nil {
		result = handler(inv)
	}

	return result.Stdout, result.Err
}
