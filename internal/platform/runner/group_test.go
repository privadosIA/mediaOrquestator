//go:build unix

package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// alive says whether the process runs. A process that was killed and that nobody has reaped yet is a zombie: kill(pid, 0) still
// finds it, but it is dead. In a container whose first process does not reap orphans (no init) that is what a killed grandchild
// stays as, so the state is read where the system offers it (/proc on Linux).
func alive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return true
	}
	// "pid (name) S ...": the name may hold spaces and brackets, so the state is the letter after the last ")".
	end := bytes.LastIndexByte(stat, ')')
	return end < 0 || end+2 >= len(stat) || stat[end+2] != 'Z'
}

// A program that starts another and waits for it: cancelling the context must leave neither, and Run must return soon.
func TestRun_CancellingKillsTheWholeGroup(t *testing.T) {
	pidFile := t.TempDir() + "/pid"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := NewExecRunner().Run(ctx, "sh", []string{"-c", "sleep 60 & echo $! > " + pidFile + "; wait"})
		done <- err
	}()
	var child int
	for i := 0; i < 100 && child == 0; i++ {
		time.Sleep(20 * time.Millisecond)
		if b, err := os.ReadFile(pidFile); err == nil {
			child, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
	}
	if child == 0 {
		t.Fatal("the grandchild never started")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled run must fail")
		}
	case <-time.After(killDelay + 3*time.Second):
		t.Fatal("Run did not return after the cancel: a grandchild keeps its pipes open")
	}
	if time.Since(start) > killDelay {
		t.Errorf("Run took %v to return, want well under the %v delay", time.Since(start), killDelay)
	}
	time.Sleep(100 * time.Millisecond)
	if alive(child) {
		syscall.Kill(child, syscall.SIGKILL)
		t.Fatal("the grandchild survived the cancel")
	}
}

func TestRun_AChattyStderrKeepsOnlyItsTail(t *testing.T) {
	_, err := NewExecRunner().Run(context.Background(), "sh", []string{"-c", "head -c 20000000 /dev/zero | tr '\\0' 'x' >&2; echo THE-END >&2; exit 3"})
	if err == nil {
		t.Fatal("exit 3 must fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "THE-END") {
		t.Error("the error must carry the end of stderr")
	}
	if len(msg) > StderrTail+4096 {
		t.Errorf("the error is %d bytes long, want about the %d of the tail", len(msg), StderrTail)
	}
}

func TestRun_TooMuchStdoutStopsTheProgram(t *testing.T) {
	start := time.Now()
	out, err := NewExecRunner().Run(context.Background(), "sh", []string{"-c", "head -c 700000000 /dev/zero"})
	if !errors.Is(err, ErrOutputTooLarge) || out != nil {
		t.Fatalf("got %d bytes and %v, want ErrOutputTooLarge", len(out), err)
	}
	if time.Since(start) > 20*time.Second {
		t.Errorf("took %v", time.Since(start))
	}
}

func TestRun_TheErrorCutsLongArguments(t *testing.T) {
	prompt := strings.Repeat("p", 20000)
	_, err := NewExecRunner().Run(context.Background(), "sh", []string{"-c", "exit 1", prompt})
	if err == nil || len(err.Error()) > 1000 {
		t.Fatalf("the error must not carry the whole prompt: %d bytes", len(err.Error()))
	}
}

func TestRun_StillReturnsStdoutAndStderr(t *testing.T) {
	out, err := NewExecRunner().Run(context.Background(), "sh", []string{"-c", "printf hello; echo oops >&2"})
	if err != nil || string(out) != "hello" {
		t.Fatalf("got %q, %v", out, err)
	}
}
