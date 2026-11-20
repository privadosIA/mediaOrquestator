package assist

import (
	"strings"
	"testing"
)

func TestImproveIsOfferedAndKeepsTheLanguage(t *testing.T) {
	task, err := ParseTask("improve")
	if err != nil || task != TaskImprove {
		t.Fatalf("ParseTask(improve) = %q, %v", task, err)
	}
	spec := tasks[TaskImprove]
	if !strings.Contains(spec.system, "SAME language") || !strings.Contains(spec.system, "quotes exactly") {
		t.Errorf("the prompt must keep the language and the quoted words: %q", spec.system)
	}
	if got := spec.user("un gato"); !strings.Contains(got, "un gato") {
		t.Errorf("the request must reach the model: %q", got)
	}
}
