package assist

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ErrBadSVG marks an answer of the svg task that is not a usable SVG: it does not parse, its root is not
// <svg>, it has no viewBox or it carries a script, an event attribute or an outside reference.
var ErrBadSVG = errors.New("the model did not write a usable SVG")

// MaxSVGBytes bounds what the svg task accepts back: a few dozen elements are a few KB, so a reply this long
// is a model that ran on.
const MaxSVGBytes = 200 << 10

// svgMaxTokens is the generation limit of the svg task: a drawing of a few dozen elements fits well inside it.
const svgMaxTokens = 3000

const svgSystem = "You draw vector graphics. Reply with ONE complete SVG document and nothing else: no explanation, no code fence.\n" +
	"Rules: a single <svg> element with xmlns=\"http://www.w3.org/2000/svg\" and a viewBox (use 0 0 1920 1080 unless the request asks for another shape). " +
	"Draw with shapes, paths and groups (<g>), and give an id to every element worth selecting or animating separately (for example ring, bar-1, bar-2, logo-mark). " +
	"Put fills and strokes as attributes on each element; do not use a <style> block, gradients, filters, images or fonts. " +
	"No <text> elements. No <script>, no event attributes (onclick and the like), no links or references to anything outside the document. " +
	"At most 40 elements; keep the drawing simple, flat and readable, away from the middle of the frame being covered by anything but shapes."

// TaskSVG writes an SVG from a description. Its answer is a document, not a line, so it goes through SVG, not Run.
const TaskSVG Task = "svg"

// SVG writes one SVG document from a description with the same lifecycle as the other tasks (memory rule,
// CPU only, unload), and checks it cheaply: it parses as XML, its root is <svg>, it has a viewBox and it
// holds no script, event attribute or outside reference. A reply that fails the checks is ErrBadSVG; one that
// is a refusal is ErrRefused.
func (a *Assistant) SVG(ctx context.Context, text string) (string, error) {
	if err := ValidateText(text); err != nil {
		return "", err
	}
	user := "Drawing: " + strings.TrimSpace(text) + "\n(Reply with the SVG only.)"
	reply, truncated, err := a.generateOnCPU(ctx, svgSystem, user, svgMaxTokens)
	if err != nil {
		return "", err
	}
	if IsRefusal(Clean(reply, false)) {
		return "", refused(a.Model)
	}
	if truncated {
		return "", fmt.Errorf("%w: the drawing was cut off before it ended; try a simpler description or another model", ErrBadSVG)
	}
	return ExtractSVG(reply)
}

var (
	svgThink = regexp.MustCompile(`(?s)<think>.*?</think>`)
	svgOpen  = regexp.MustCompile(`(?i)<svg[\s>]`)
	// onAttr is an event attribute such as onclick or onload.
	onAttr = regexp.MustCompile(`(?i)^on[a-z]+$`)
)

// ExtractSVG takes the SVG document out of a model's reply (which may carry reasoning, a code fence or words
// around it) and checks it. It returns the document as the model wrote it, from <svg to </svg>.
func ExtractSVG(reply string) (string, error) {
	s := svgThink.ReplaceAllString(reply, " ")
	if i := strings.Index(s, "<think>"); i >= 0 {
		s = s[:i]
	}
	loc := svgOpen.FindStringIndex(s)
	if loc == nil {
		return "", fmt.Errorf("%w: the reply has no <svg> element", ErrBadSVG)
	}
	s = s[loc[0]:]
	end := strings.LastIndex(strings.ToLower(s), "</svg>")
	if end < 0 {
		return "", fmt.Errorf("%w: the drawing does not end with </svg>", ErrBadSVG)
	}
	s = strings.TrimSpace(s[:end+len("</svg>")])
	if len(s) > MaxSVGBytes {
		return "", fmt.Errorf("%w: the drawing is longer than %d KB", ErrBadSVG, MaxSVGBytes>>10)
	}
	if err := checkSVG(s); err != nil {
		return "", fmt.Errorf("%w: %w", ErrBadSVG, err)
	}
	return s, nil
}

// checkSVG parses the document and refuses what the editor and a browser should never receive from a model.
func checkSVG(doc string) error {
	dec := xml.NewDecoder(bytes.NewReader([]byte(doc)))
	dec.Strict = true
	root := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("it is not well-formed XML (%w)", err)
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		name := strings.ToLower(el.Name.Local)
		if !root {
			root = true
			if name != "svg" {
				return fmt.Errorf("the root element is <%s>, not <svg>", el.Name.Local)
			}
			has := false
			for _, at := range el.Attr {
				has = has || strings.EqualFold(at.Name.Local, "viewBox") && strings.TrimSpace(at.Value) != ""
			}
			if !has {
				return errors.New("the <svg> has no viewBox")
			}
		}
		if name == "script" || name == "foreignobject" {
			return fmt.Errorf("it holds a <%s> element", el.Name.Local)
		}
		for _, at := range el.Attr {
			local := strings.ToLower(at.Name.Local)
			if onAttr.MatchString(local) {
				return fmt.Errorf("it holds the event attribute %q", at.Name.Local)
			}
			if local == "href" {
				v := strings.TrimSpace(strings.ToLower(at.Value))
				if !strings.HasPrefix(v, "#") {
					return errors.New("it refers to something outside the document")
				}
			}
		}
	}
	if !root {
		return errors.New("it is empty")
	}
	return nil
}
