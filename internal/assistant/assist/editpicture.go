package assist

import (
	"strings"
)

// TaskEdit turns what someone asks to do to a picture, written in any language, into the short English
// instruction an image-editing model (FLUX.1 Kontext, Qwen-Image-Edit) follows. It is not offered through
// ParseTask: the request planner is its only caller, and it goes through Run for the model's lifecycle.
const TaskEdit Task = "edit"

// keepRest is what an edit instruction ends with so that nothing else changes.
const keepRest = "Keep everything else exactly the same."

// editTask is the prompt of the task that turns a request to change a picture into an instruction.
func editTask() taskSpec {
	return taskSpec{
		system: "You turn a request to change a picture, written in any language, into ONE instruction in English for an AI " +
			"image-editing model. Reply with ONE line and nothing else, at most 30 words, in the imperative. " +
			"Say exactly what changes and who or what it applies to: name the person by where they are in the picture (the man on the left, " +
			"the woman on the right, the child in the middle) or by a feature, never by a colour of their clothes, and do not repeat a colour " +
			"that is not the one to apply. Describe only the change, not the whole picture. " +
			"If the request describes a whole new look for the picture instead of a change to it, write \"Transform the picture into:\" " +
			"followed by a short English description. No preamble, no quotes, no explanation.",
		user:      func(text string) string { return "Request: " + text },
		maxTokens: 100,
	}
}

// EditInstruction finishes the model's line: unwrapped, and ended with the sentence that keeps the rest of the
// picture as it is, unless it already says so or is a transformation of the whole picture.
func EditInstruction(line string) string {
	line = strings.Trim(strings.TrimSpace(line), "\"'«»“”` ")
	if line == "" {
		return ""
	}
	low := strings.ToLower(line)
	if strings.HasPrefix(low, "transform") || strings.Contains(low, "everything else") {
		return line
	}
	if !strings.HasSuffix(line, ".") {
		line += "."
	}
	return line + " " + keepRest
}
