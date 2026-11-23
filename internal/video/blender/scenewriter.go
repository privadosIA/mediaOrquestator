package blender

// scenewriter.go: a language model writes the description of a 3D scene from an idea (spec blender-scene-writer). The model
// is told what the project has (the length of the scene, its colours, fonts and tempo) and what the contract accepts, from
// the contract's own tables; its answer is checked with the rules of a hand-written description (Parse) and with a floor
// of quality those rules do not give (an object, and something that moves), and is asked for again with the problems, up
// to MaxAttempts times. The idea is data: only a description that passes the checks is ever returned, whatever it says.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// MaxAttempts is how many times the model is asked for a description (the first time and the repairs).
const MaxAttempts = 3

// MaxIdeaRunes bounds the idea.
const MaxIdeaRunes = 2000

// writerMaxTokens is the room for the answer: a description of 60 keys is about 2500 tokens. The writer of the server
// multiplies it for providers that think.
const writerMaxTokens = 6000

// ErrRejected is returned when the model could not write a description that passes the checks.
var ErrRejected = errors.New("the model could not write a working scene description")

// Writer asks a language model for text. It has the same method as the writer of the motion graphics, so the one the
// server builds for them serves here without the two packages knowing each other.
type Writer interface {
	Write(ctx context.Context, system, user string, maxTokens int) (string, error)
}

// WriteRequest is what the model is asked for.
type WriteRequest struct {
	// Idea is what the scene shows, in words.
	Idea string
	// Seconds is the length of the scene; FPS its frame rate; Width and Height give the aspect of the frame.
	Seconds       float64
	FPS           int
	Width, Height int
	// Env is what the description is checked against (the design's colours and fonts, the tempo, the sources).
	Env Env
	// Families names, for the prompt, the family of each font role the design has ("display": "Playfair Display").
	Families map[string]string
	// Current is the description to change, as stored; nil writes a new one.
	Current []byte
}

// WriteResult is a description that passed.
type WriteResult struct {
	// Raw is the description as the model wrote it (what a person should see in the file), re-indented.
	Raw []byte
	// Spec is the same, resolved (every key in seconds, every colour #rrggbb).
	Spec *Spec
	// Attempts is how many answers it took.
	Attempts int
}

// WriteDescription asks the model for a description and checks it, asking again with the problems up to MaxAttempts times.
// When no attempt passes it returns ErrRejected with the last problems in the message.
func WriteDescription(ctx context.Context, w Writer, req WriteRequest) (WriteResult, error) {
	if strings.TrimSpace(req.Idea) == "" {
		return WriteResult{}, errors.New("the idea is empty")
	}
	system := writerSystemPrompt(ExampleFor(req.Env.BPM, req.Seconds))
	base := writerUserPrompt(req)
	user := base
	var last []string
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		answer, err := w.Write(ctx, system, user, writerMaxTokens)
		if err != nil {
			return WriteResult{}, err
		}
		raw := ExtractDescriptionJSON(answer)
		var problems []string
		if raw == "" {
			problems = []string{"the answer has no JSON object: answer with the description and nothing else"}
		} else if problem := syntaxProblem(raw); problem != "" {
			problems = []string{problem}
		} else {
			spec, err := Parse([]byte(raw), req.Seconds, req.Env)
			var ps Problems
			switch {
			case errors.As(err, &ps):
				problems = ps
			case err != nil:
				problems = []string{err.Error()}
			default:
				if problems = QualityProblems(spec); len(problems) == 0 {
					var indented bytes.Buffer
					if err := json.Indent(&indented, []byte(raw), "", "  "); err != nil {
						return WriteResult{}, err
					}
					return WriteResult{Raw: append(indented.Bytes(), '\n'), Spec: spec, Attempts: attempt}, nil
				}
			}
		}
		last = problems
		user = writerRepairPrompt(base, raw, problems)
	}
	return WriteResult{}, fmt.Errorf("%w after %d attempts: %s", ErrRejected, MaxAttempts, strings.Join(last, "; "))
}

// syntaxProblem says where a JSON syntax error is, with the text around it, so that the model can find a missing quote or a
// comma ("...{\"t:2.3,\"l<<HERE>>ocation..."): the error of the decoder alone ("invalid character 'l' after object key")
// does not tell a model where to look. It returns "" when the text is valid JSON.
func syntaxProblem(raw string) string {
	var v any
	err := json.Unmarshal([]byte(raw), &v)
	if err == nil {
		return ""
	}
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		return "not valid JSON: " + err.Error()
	}
	off := int(se.Offset) - 1 // the decoder counts the bytes read, the one that broke it included: the mark goes before it
	if off > len(raw) {
		off = len(raw)
	}
	if off < 0 {
		off = 0
	}
	lo, hi := max(off-100, 0), min(off+40, len(raw))
	return fmt.Sprintf("not valid JSON (%s) near: ...%s<<HERE>>%s... (check the quotes and commas just before the mark)", se.Error(), raw[lo:off], raw[off:hi])
}

var reDescriptionFence = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")

// ExtractDescriptionJSON takes the JSON object out of a model's answer: inside a markdown fence when there is one, else from
// the first { to the last }. It returns "" when there is none.
func ExtractDescriptionJSON(answer string) string {
	text := strings.TrimSpace(answer)
	if m := reDescriptionFence.FindStringSubmatch(text); m != nil {
		text = strings.TrimSpace(m[1])
	}
	i, j := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if i < 0 || j < i {
		return ""
	}
	return text[i : j+1]
}

// QualityProblems are what a model's description must not have although the contract accepts it: no object at all, and a
// scene in which nothing moves. They are asked of a model because a still scene is a wasted render; a person may write one.
// The description has to be resolved (Parse gives it so).
func QualityProblems(s *Spec) []string {
	var out []string
	if len(s.Objects) == 0 {
		out = append(out, "objects: the scene has no object: add the things the scene shows")
	}
	if !moves(s) {
		out = append(out, "nothing moves: no key of the camera or of an object differs from its previous key; make the camera travel or an object move, turn, scale or flash")
	}
	return out
}

// moves says whether any channel of the camera or of an object is given by two keys, in time order, with different values.
func moves(s *Spec) bool {
	lists := s.allKeys()
	for _, keys := range lists {
		ordered := append([]Key(nil), keys...)
		sort.SliceStable(ordered, func(i, j int) bool { return keyTime(ordered[i]) < keyTime(ordered[j]) })
		var loc, rot, tgt, scale *[3]float64
		var emi *float64
		for _, k := range ordered {
			if differs3(&loc, (*[3]float64)(k.Location)) || differs3(&rot, (*[3]float64)(k.Rotation)) ||
				differs3(&tgt, (*[3]float64)(k.Target)) || differs3(&scale, scaleOf(k.Scale)) || differs1(&emi, k.Emission) {
				return true
			}
		}
	}
	return false
}

func keyTime(k Key) float64 {
	if k.T != nil {
		return *k.T
	}
	return math.Inf(1)
}

func scaleOf(s *Scale) *[3]float64 {
	if s == nil {
		return nil
	}
	return &s.V
}

// differs3 remembers the first value a channel is given and reports whether a later one is not the same.
func differs3(prev **[3]float64, v *[3]float64) bool {
	if v == nil {
		return false
	}
	if *prev == nil {
		c := *v
		*prev = &c
		return false
	}
	return **prev != *v
}

func differs1(prev **float64, v *float64) bool {
	if v == nil {
		return false
	}
	if *prev == nil {
		c := *v
		*prev = &c
		return false
	}
	return **prev != *v
}
