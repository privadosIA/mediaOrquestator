package motion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// TextBox is where a piece of visible text is drawn, in frame pixels.
type TextBox struct {
	Text                     string
	Left, Top, Right, Bottom float64
	// House says the text is in one of the page's own layers (#verse, #hit, #hitbg: what the program draws for a song), not in a part.
	House bool
}

func (b TextBox) area() float64 {
	w, h := b.Right-b.Left, b.Bottom-b.Top
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

// textBoxesScript measures every text node that is drawn: the element is shown (display, visibility and the
// opacity of it and of its ancestors above 0.05) and the text takes room. It answers {total, boxes}: how many
// text nodes the page has and where the drawn ones are.
const textBoxesScript = `(function(){
  var out = [], total = 0, walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  while (walker.nextNode()) {
    var n = walker.currentNode, txt = n.nodeValue.replace(/\s+/g, ' ').trim();
    if (!txt) continue;
    var el = n.parentElement;
    if (!el || /^(SCRIPT|STYLE|NOSCRIPT|TITLE)$/.test(el.tagName)) continue;
    total++;
    var shown = true, op = 1;
    for (var e = el; e; e = e.parentElement) {
      var cs = getComputedStyle(e);
      if (cs.display === 'none' || cs.visibility === 'hidden') { shown = false; break; }
      op *= parseFloat(cs.opacity);
    }
    if (!shown || op < 0.05) continue;
    var r = document.createRange(); r.selectNodeContents(n);
    var b = r.getBoundingClientRect();
    if (b.width < 1 || b.height < 1) continue;
    out.push({text: txt.slice(0, 40), left: b.left, top: b.top, right: b.right, bottom: b.bottom, house: !!(el.closest && el.closest('#verse,#hit,#hitbg'))});
  }
  return JSON.stringify({total: total, boxes: out});
})()`

// TextBoxes measures the text the page draws at the time it was last set to, and says how many text nodes
// the page has in all (those that are not drawn are the ones hidden by display, visibility or opacity).
func (s *Session) TextBoxes(ctx context.Context) (boxes []TextBox, total int, err error) {
	var res struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := s.c.call(ctx, "Runtime.evaluate", map[string]any{"expression": textBoxesScript, "returnByValue": true}, &res); err != nil {
		return nil, 0, err
	}
	var raw struct {
		Total int `json:"total"`
		Boxes []struct {
			Text string  `json:"text"`
			L    float64 `json:"left"`
			T    float64 `json:"top"`
			R    float64 `json:"right"`
			B    float64 `json:"bottom"`
			H    bool    `json:"house"`
		} `json:"boxes"`
	}
	if err := json.Unmarshal([]byte(res.Result.Value), &raw); err != nil {
		return nil, 0, err
	}
	boxes = make([]TextBox, 0, len(raw.Boxes))
	for _, r := range raw.Boxes {
		boxes = append(boxes, TextBox{Text: r.Text, Left: r.L, Top: r.T, Right: r.R, Bottom: r.B, House: r.H})
	}
	return boxes, raw.Total, nil
}

// JudgeInvisible says that a page has text and none of it is drawn at the end of the clip: an animation that
// never reaches its visible state (an invalid CSS value leaves the opacity at 0, for one).
func JudgeInvisible(boxes []TextBox, total int, at float64) []string {
	if total > 0 && len(boxes) == 0 {
		return []string{fmt.Sprintf("the page has text but none of it is visible at %.1fs: every text is still hidden (opacity 0, visibility or display) at the end of the animation; check the animation values (an invalid CSS value, such as a var() with extra words, makes the whole animation fail) so the text ends visible", at)}
	}
	return nil
}

const (
	// edgeSlack is how far a box may go past a side before it counts as cut (sub-pixel rounding, italics).
	edgeSlack = 1.0
	// overlapShare is the share of the smaller box another box may cover before they count as overlapping.
	overlapShare = 0.2
)

// HouseProblemPrefix starts a problem about a text of the page's own layers: whoever reads the problems may tell it from a part's.
const HouseProblemPrefix = "server: "

// HouseMark is the prefix a problem about the box carries: HouseProblemPrefix for a house text, nothing for the rest.
func HouseMark(b TextBox) string {
	if b.House {
		return HouseProblemPrefix
	}
	return ""
}

// JudgeCut says which texts are partly outside the frame on its left or right side. A text wholly outside
// is not reported (it may be about to enter), and neither is a cut at the top or the bottom (credits leave).
func JudgeCut(boxes []TextBox, width int, at float64) []string {
	var out []string
	w := float64(width)
	for _, b := range boxes {
		if b.Right <= 0 || b.Left >= w {
			continue
		}
		switch {
		case b.Left < -edgeSlack:
			out = append(out, HouseMark(b)+fmt.Sprintf("the text %q is cut by the left edge of the frame at %.1fs (it starts at x=%.0f of %d): keep every text inside the frame with a margin", b.Text, at, b.Left, width))
		case b.Right > w+edgeSlack:
			out = append(out, HouseMark(b)+fmt.Sprintf("the text %q is cut by the right edge of the frame at %.1fs (it ends at x=%.0f of %d): keep every text inside the frame with a margin", b.Text, at, b.Right, width))
		}
	}
	return out
}

// JudgeOverlap says which pairs of texts cover more than 20% of the smaller one.
func JudgeOverlap(boxes []TextBox, at float64) []string {
	var out []string
	for i := range boxes {
		for j := i + 1; j < len(boxes); j++ {
			a, b := boxes[i], boxes[j]
			w := minF(a.Right, b.Right) - maxF(a.Left, b.Left)
			h := minF(a.Bottom, b.Bottom) - maxF(a.Top, b.Top)
			if w <= 0 || h <= 0 {
				continue
			}
			small := minF(a.area(), b.area())
			if small > 0 && w*h/small > overlapShare {
				out = append(out, fmt.Sprintf("the texts %q and %q overlap at %.1fs: separate them so each one can be read", clipText(a.Text), clipText(b.Text), at))
			}
		}
	}
	return out
}

func clipText(s string) string {
	if r := []rune(strings.TrimSpace(s)); len(r) > 24 {
		return string(r[:24]) + "…"
	}
	return s
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// EvalJSON runs an expression in the page and decodes its value, which must be JSON text
// (the expression returns JSON.stringify(...)).
func (s *Session) EvalJSON(ctx context.Context, expression string, out any) error {
	var res struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
		Exception *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := s.c.call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true}, &res); err != nil {
		return err
	}
	if res.Exception != nil {
		return fmt.Errorf("the page failed evaluating %q: %s", expression, res.Exception.Text)
	}
	return json.Unmarshal([]byte(res.Result.Value), out)
}
