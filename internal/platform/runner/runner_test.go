package runner

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecRunner_Success(t *testing.T) {
	r := NewExecRunner()
	out, err := r.Run(context.Background(), "echo", []string{"hello"})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Fatalf("stdout = %q, want %q", out, "hello")
	}
}

func TestExecRunner_NonZeroExitIncludesStderr(t *testing.T) {
	r := NewExecRunner()
	_, err := r.Run(context.Background(), "sh", []string{"-c", "echo boom >&2; exit 1"})
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error %q should contain captured stderr %q", err.Error(), "boom")
	}
}

func TestExecRunner_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	r := NewExecRunner()
	_, err := r.Run(ctx, "sleep", []string{"5"})
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil for a cancelled context")
	}
}

func TestExecRunner_StdinPiped(t *testing.T) {
	r := NewExecRunner()
	out, err := r.Run(context.Background(), "cat", nil, WithStdin([]byte("piped data")))
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if string(out) != "piped data" {
		t.Fatalf("stdout = %q, want %q", out, "piped data")
	}
}

func TestExecRunner_EnvOverride(t *testing.T) {
	r := NewExecRunner()
	out, err := r.Run(context.Background(), "sh", []string{"-c", "echo $FOO"}, WithEnv("FOO=bar"))
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if strings.TrimSpace(string(out)) != "bar" {
		t.Fatalf("stdout = %q, want %q", out, "bar")
	}
}

func TestFakeRunner_RecordsInvocationsAndReturnsScriptedResults(t *testing.T) {
	fr := NewFakeRunner()
	fr.Script(
		ScriptedResult{Stdout: []byte("first")},
		ScriptedResult{Err: errBoom},
	)

	out, err := fr.Run(context.Background(), "whatever", []string{"-x"}, WithStdin([]byte("in")), WithEnv("K=V"))
	if err != nil || string(out) != "first" {
		t.Fatalf("first call = (%q, %v), want (%q, nil)", out, err, "first")
	}

	_, err = fr.Run(context.Background(), "whatever", nil)
	if err != errBoom {
		t.Fatalf("second call error = %v, want %v", err, errBoom)
	}

	calls := fr.Calls()
	if len(calls) != 2 {
		t.Fatalf("got %d recorded invocations, want 2", len(calls))
	}
	if calls[0].Name != "whatever" || calls[0].Stdin[0] == 0 {
		t.Fatalf("first invocation not recorded correctly: %+v", calls[0])
	}
	if len(calls[0].Env) != 1 || calls[0].Env[0] != "K=V" {
		t.Fatalf("env not recorded correctly: %+v", calls[0].Env)
	}
}

func TestFakeRunner_HandlerTakesPrecedenceOverScripts(t *testing.T) {
	fr := NewFakeRunner()
	fr.Script(ScriptedResult{Stdout: []byte("ignored")})
	fr.SetHandler(func(inv Invocation) ScriptedResult {
		return ScriptedResult{Stdout: []byte("from handler: " + inv.Name)}
	})

	out, err := fr.Run(context.Background(), "x", nil)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if string(out) != "from handler: x" {
		t.Fatalf("stdout = %q, want %q", out, "from handler: x")
	}
}

func TestFakeRunner_Reset(t *testing.T) {
	fr := NewFakeRunner()
	fr.Script(ScriptedResult{Stdout: []byte("x")})
	fr.Run(context.Background(), "x", nil)
	fr.Reset()

	if len(fr.Calls()) != 0 {
		t.Fatal("Reset() should clear recorded invocations")
	}
	out, err := fr.Run(context.Background(), "y", nil)
	if err != nil || len(out) != 0 {
		t.Fatalf("after Reset(), unscripted call should return (nil, nil), got (%q, %v)", out, err)
	}
}

var errBoom = &boomErr{}

type boomErr struct{}

func (*boomErr) Error() string { return "boom" }

func TestWithStderrKeepsTheStderrOfASuccessfulRun(t *testing.T) {
	var buf bytes.Buffer
	if _, err := NewExecRunner().Run(context.Background(), "sh", []string{"-c", "echo FATAL oops >&2"}, WithStderr(&buf)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "FATAL oops") {
		t.Errorf("stderr = %q", buf.String())
	}
}
