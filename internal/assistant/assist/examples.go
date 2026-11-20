package assist

import (
	"context"
	"fmt"
	"strings"
)

// Limits of the examples a caller can send with an assistant call.
const (
	MaxExamples      = 5
	MaxExampleLength = 2000
)

// Example is one prompt of the house style, shown to the model to follow and never to copy.
type Example struct {
	Prompt   string `json:"prompt"`
	Negative string `json:"negative,omitempty"`
	Tags     string `json:"tags,omitempty"`
}

// ValidateExamples checks what a client sent: at most MaxExamples, each with a prompt, every field within MaxExampleLength.
func ValidateExamples(ex []Example) error {
	if len(ex) > MaxExamples {
		return fmt.Errorf("%w: examples has %d items: the most is %d", ErrInvalidInput, len(ex), MaxExamples)
	}
	for i, e := range ex {
		if strings.TrimSpace(e.Prompt) == "" {
			return fmt.Errorf("%w: examples[%d] has no prompt", ErrInvalidInput, i)
		}
		for name, v := range map[string]string{"prompt": e.Prompt, "negative": e.Negative, "tags": e.Tags} {
			if n := len([]rune(v)); n > MaxExampleLength {
				return fmt.Errorf("%w: examples[%d].%s has %d characters: the most is %d", ErrInvalidInput, i, name, n, MaxExampleLength)
			}
		}
	}
	return nil
}

type examplesKey struct{}

// WithExamples carries the examples of one call to whichever task runs, so every task takes them the same way.
func WithExamples(ctx context.Context, ex []Example) context.Context {
	if len(ex) == 0 {
		return ctx
	}
	return context.WithValue(ctx, examplesKey{}, ex)
}

// ExamplesOf are the examples a call carries (nil when none), for the seams of tests.
func ExamplesOf(ctx context.Context) []Example {
	ex, _ := ctx.Value(examplesKey{}).([]Example)
	return ex
}

// examplesBlock is the text added after a task's own request ("" when there are none).
func examplesBlock(ctx context.Context) string {
	ex := ExamplesOf(ctx)
	if len(ex) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nExamples of the style to follow. Learn their wording, length and level of detail; never copy them or their subjects into the answer:\n")
	for i, e := range ex {
		fmt.Fprintf(&b, "Example %d: %s\n", i+1, strings.TrimSpace(e.Prompt))
		if n := strings.TrimSpace(e.Negative); n != "" {
			b.WriteString("  Negative: " + n + "\n")
		}
		if t := strings.TrimSpace(e.Tags); t != "" {
			b.WriteString("  Tags: " + t + "\n")
		}
	}
	return b.String()
}
