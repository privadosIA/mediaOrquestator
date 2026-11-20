package assist

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ShotBrief is what the assistant is told of a shot: the section of the song it belongs to and the lines sung there.
type ShotBrief struct {
	Section string
	Lines   []string
}

// Storyboard is what the assistant writes for a music video: the look shared by every shot, and a picture prompt
// for each shot (empty where the model wrote none, so the caller can fall back).
type Storyboard struct {
	Subject string
	Prompts []string
}

const (
	storyboardBatch = 8
	// per shot a prompt of 30 to 45 words takes about 70 tokens; the rest is the subject line.
	storyboardTokensPerShot = 110
	storyboardSystem        = "You are a music video director who writes prompts for an image generator. Reply in exactly this format and nothing else:\n" +
		"SUBJECT: <ONE line in English, 20 to 35 words: the main character or characters and the look of the whole video (setting, palette, era); it is the same for every shot>\n" +
		"1: <ONE line in English, 30 to 45 words: the picture of shot 1: subject, setting, light, camera angle and lens, mood>\n" +
		"2: <the same for shot 2>\n" +
		"...and so on, one numbered line per shot. Show what the lyrics tell, with images and no text on the picture. " +
		"No preamble, no explanation, no code fences."
	storyboardFollowSystem = "You are a music video director who writes prompts for an image generator. The look of the video is already decided (it is given). Reply in exactly this format and nothing else:\n" +
		"1: <ONE line in English, 30 to 45 words: the picture of shot 1: subject, setting, light, camera angle and lens, mood>\n" +
		"2: <the same for shot 2>\n" +
		"...and so on, one numbered line per shot. Keep the same character and look. Show what the lyrics tell, with images and no text on the picture. " +
		"No preamble, no explanation, no code fences."
)

// Storyboard asks the model for the look and the picture of every shot of a music video, in batches of a few
// shots (a CPU model answers slowly, and a short answer is more reliable). The first batch also decides the
// look, which the later ones are given so the character stays the same. It shares the lifecycle of the other
// tasks: CPU only, memory check, unload when done.
func (a *Assistant) Storyboard(ctx context.Context, style, title string, shots []ShotBrief) (Storyboard, error) {
	out := Storyboard{Prompts: make([]string, len(shots))}
	for lo := 0; lo < len(shots); lo += storyboardBatch {
		hi := lo + storyboardBatch
		if hi > len(shots) {
			hi = len(shots)
		}
		first := lo == 0
		system := storyboardFollowSystem
		if first {
			system = storyboardSystem
		}
		reply, truncated, err := a.generateOnCPU(ctx, system, storyboardUser(style, title, out.Subject, shots[lo:hi]), 120+storyboardTokensPerShot*(hi-lo))
		if err != nil {
			return out, err
		}
		if IsRefusal(Clean(reply, false)) && !strings.Contains(strings.ToUpper(reply), "SUBJECT:") {
			return out, refused(a.Model)
		}
		subject, prompts := ParseStoryboard(reply, hi-lo, truncated)
		if first {
			out.Subject = subject
		}
		copy(out.Prompts[lo:hi], prompts)
	}
	return out, nil
}

func storyboardUser(style, title, subject string, shots []ShotBrief) string {
	var b strings.Builder
	if title != "" {
		fmt.Fprintf(&b, "Song: %s\n", title)
	}
	if style != "" {
		fmt.Fprintf(&b, "Music style: %s\n", style)
	}
	if subject != "" {
		fmt.Fprintf(&b, "Look of the video (already decided): %s\n", subject)
	}
	b.WriteString("Shots:\n")
	for i, s := range shots {
		tag := s.Section
		if tag == "" {
			tag = "song"
		}
		if len(s.Lines) == 0 {
			fmt.Fprintf(&b, "%d. [%s] (no lyrics: the music alone)\n", i+1, tag)
		} else {
			fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, tag, strings.Join(s.Lines, " / "))
		}
	}
	b.WriteString("(Answer in English.)")
	return b.String()
}

var (
	subjectLine = regexp.MustCompile(`(?i)^[\s*#_>-]*subject[\s*_]*:[\s*_]*(.*)$`)
	shotLine    = regexp.MustCompile(`(?i)^[\s*#_>-]*(?:shot\s*)?(\d+)\s*[:.)\-]+\s*(.+)$`)
)

// ParseStoryboard reads a reply in the SUBJECT:/numbered format: the look (empty when the reply has none) and
// the prompt of each of the count shots (empty where it has none). A prompt that stopped at the token limit is
// kept only when it is the last one and long enough to describe a picture.
func ParseStoryboard(reply string, count int, truncated bool) (subject string, prompts []string) {
	prompts = make([]string, count)
	last := -1
	s := thinkRe.ReplaceAllString(reply, " ")
	if i := strings.Index(s, "<think>"); i >= 0 { // reasoning that never closed
		s = s[:i]
	}
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") { // fences around the answer
			continue
		}
		if m := subjectLine.FindStringSubmatch(line); m != nil {
			subject = clean1(m[1])
			continue
		}
		if m := shotLine.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			if n >= 1 && n <= count {
				prompts[n-1] = clean1(m[2])
				last = n - 1
			}
		}
	}
	if truncated && last >= 0 && len(strings.Fields(prompts[last])) < 12 {
		prompts[last] = ""
	}
	return subject, prompts
}

func clean1(s string) string {
	return strings.Trim(strings.TrimSpace(s), "\"*_ ")
}
