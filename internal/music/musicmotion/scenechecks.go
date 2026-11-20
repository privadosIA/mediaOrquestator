package musicmotion

// scenechecks.go: what a scene written for a verse has to pass besides what any part passes (spec motion-scene-parts, "A scene
// carries no captions, draws big, and keeps the design's system"). Two kinds of check, both deterministic:
//
//   - sceneStyleProblems reads the CSS and the SVG attributes of the fragment before any browser is opened: the fonts and the
//     colours must come from the design through its variables.
//   - sceneFrameProblems draws one frame in the middle of the verse and reads the page: no text that is not the verse (no
//     captions), no text too small to read, the words of the verse and the word that matters big, and a picture that fills at
//     least a quarter of the frame.
//
// They run on the scenes of a script piece (CheckOptions.Scene set), never on the parts of the catalog.

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// minTextVh is the smallest a visible text may be, in percent of the frame's height.
	minTextVh = 2.0
	// minVerseVh is the smallest a word of the verse may be.
	minVerseVh = 4.0
	// minHeadlineVh is the size the largest text of a scene with a verse has to reach: the word that matters.
	minHeadlineVh = 12.0
	// minPictureShare is the share of the frame the picture (what is not text) has to cover.
	minPictureShare = 0.25
	// maxSceneProblems caps what one kind of check says, so the answer to the writer stays short.
	maxSceneProblems = 3
)

// --- the CSS and the SVG ----------------------------------------------------------------------

var (
	reSceneStyle  = regexp.MustCompile(`(?is)<style[^>]*>(.*?)</style>`)
	reStyleAttr   = regexp.MustCompile(`(?is)\bstyle\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	reSVGColorAtt = regexp.MustCompile(`(?is)\b(fill|stroke|stop-color|flood-color|lighting-color|color)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	reSVGFontAtt  = regexp.MustCompile(`(?is)\b(font-family)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	// reDeclaration finds property: value pairs; the value ends at ; or the end of the block.
	reDeclaration = regexp.MustCompile(`(?i)(?:^|[;{\s])(--[\w-]+|[a-z-]+)\s*:\s*([^;{}]*)`)
	reVar         = regexp.MustCompile(`(?i)var\(\s*--[\w-]+\s*(?:,[^)]*)?\)`)
	reRelative    = regexp.MustCompile(`(?i)\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color)\(\s*from\b[^)]*\)`)
	reColourFunc  = regexp.MustCompile(`(?i)\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color|device-cmyk)\(`)
	reHex         = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	reWord        = regexp.MustCompile(`[a-zA-Z]+`)
	reWordToken   = regexp.MustCompile(`[\p{L}\p{N}]+(?:'[\p{L}]+)?`)
	reURL         = regexp.MustCompile(`(?i)url\([^)]*\)`)
	reFuncName    = regexp.MustCompile(`(?i)[a-z-]+\(`)
)

// colourProps are the properties whose value is a colour (or holds one), plus the custom properties.
var colourProps = map[string]bool{
	"color": true, "background": true, "background-color": true, "background-image": true,
	"border": true, "border-color": true, "border-top": true, "border-right": true, "border-bottom": true, "border-left": true,
	"border-top-color": true, "border-right-color": true, "border-bottom-color": true, "border-left-color": true,
	"outline": true, "outline-color": true, "box-shadow": true, "text-shadow": true, "text-decoration": true,
	"text-decoration-color": true, "text-emphasis-color": true, "caret-color": true, "accent-color": true, "column-rule": true,
	"column-rule-color": true, "fill": true, "stroke": true, "stop-color": true, "flood-color": true, "lighting-color": true,
	"filter": true, "drop-shadow": true, "-webkit-text-stroke": true, "-webkit-text-fill-color": true, "-webkit-text-stroke-color": true,
}

// namedColours are the CSS colour keywords; transparent, currentColor and the CSS-wide keywords are fine and not here.
var namedColours = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range strings.Fields(`aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue blueviolet brown
		burlywood cadetblue chartreuse chocolate coral cornflowerblue cornsilk crimson cyan darkblue darkcyan darkgoldenrod darkgray
		darkgreen darkgrey darkkhaki darkmagenta darkolivegreen darkorange darkorchid darkred darksalmon darkseagreen darkslateblue
		darkslategray darkslategrey darkturquoise darkviolet deeppink deepskyblue dimgray dimgrey dodgerblue firebrick floralwhite
		forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray green greenyellow grey honeydew hotpink indianred indigo ivory khaki
		lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral lightcyan lightgoldenrodyellow lightgray lightgreen lightgrey
		lightpink lightsalmon lightseagreen lightskyblue lightslategray lightslategrey lightsteelblue lightyellow lime limegreen linen
		magenta maroon mediumaquamarine mediumblue mediumorchid mediumpurple mediumseagreen mediumslateblue mediumspringgreen
		mediumturquoise mediumvioletred midnightblue mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange
		orangered orchid palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum powderblue purple
		rebeccapurple red rosybrown royalblue saddlebrown salmon sandybrown seagreen seashell sienna silver skyblue slateblue slategray
		slategrey snow springgreen steelblue tan teal thistle tomato turquoise violet wheat white whitesmoke yellow yellowgreen`) {
		m[n] = true
	}
	return m
}()

// sceneStyleProblems is the check of the fonts and the colours of a scene fragment: a font family has to reference var(--display)
// or var(--body), and a colour has to be one of the design's variables, a mix or a relative colour of them, transparent or
// currentColor. The drawings of the theme ({{asset:name}}) are exempt: they were cleaned when they were uploaded.
func sceneStyleProblems(fragment string) []string {
	frag := reAssetSlot.ReplaceAllString(fragment, "")
	var problems []string
	seen := map[string]bool{}
	add := func(msg string) {
		if !seen[msg] {
			seen[msg] = true
			problems = append(problems, msg)
		}
	}
	check := func(prop, value, where string) {
		prop, value = strings.ToLower(strings.TrimSpace(prop)), strings.TrimSpace(value)
		if value == "" {
			return
		}
		switch {
		case prop == "font-family":
			if p := fontProblem(prop, value, where); p != "" {
				add(p)
			}
		case prop == "font":
			if p := fontProblem(prop, value, where); p != "" {
				add(p)
			}
		case colourProps[prop] || strings.HasPrefix(prop, "--"):
			if tok := colourLiteral(withoutNeutralShadows(prop, value)); tok != "" {
				add(fmt.Sprintf("the %s has the colour %s (%s: %s): colours only through var(--bg), var(--fg) and var(--accent), a color-mix() or a relative colour of them, transparent or currentColor", where, tok, prop, clipText(value, 60)))
			}
		}
	}
	for _, m := range reSceneStyle.FindAllStringSubmatch(frag, -1) {
		for _, d := range reDeclaration.FindAllStringSubmatch(m[1], -1) {
			check(d[1], d[2], "CSS")
		}
	}
	for _, m := range reStyleAttr.FindAllStringSubmatch(frag, -1) {
		body := m[1] + m[2]
		for _, d := range reDeclaration.FindAllStringSubmatch(body, -1) {
			check(d[1], d[2], "inline style")
		}
	}
	for _, m := range reSVGColorAtt.FindAllStringSubmatch(frag, -1) {
		check(m[1], m[2]+m[3], "SVG")
	}
	for _, m := range reSVGFontAtt.FindAllStringSubmatch(frag, -1) {
		check(m[1], m[2]+m[3], "SVG")
	}
	if len(problems) > 5 {
		problems = append(problems[:5], fmt.Sprintf("and %d more of the same kind", len(problems)-5))
	}
	return problems
}

// fontProblem is the message for a font that does not come from the design, or "".
func fontProblem(prop, value, where string) string {
	v := strings.ToLower(value)
	switch strings.TrimSuffix(v, " !important") {
	case "inherit", "initial", "unset", "revert":
		return ""
	}
	if strings.Contains(v, "var(--display)") || strings.Contains(v, "var(--body)") {
		return ""
	}
	return fmt.Sprintf("the %s names its own font (%s: %s): fonts only through var(--display) and var(--body)", where, prop, clipText(value, 60))
}

var (
	reNeutralColour = regexp.MustCompile(`(?i)rgba?\(\s*(?:0\s*[, ]\s*0\s*[, ]\s*0|255\s*[, ]\s*255\s*[, ]\s*255)\s*(?:[,/]\s*[\d.]+%?\s*)?\)|#(?:000|fff)(?:[0-9a-f])?\b|#(?:000000|ffffff)(?:[0-9a-f]{2})?\b|\b(?:black|white)\b`)
	reDropShadow    = regexp.MustCompile(`(?i)drop-shadow\([^)]*(?:\([^)]*\)[^)]*)*\)`)
)

// withoutNeutralShadows takes out of a value the shadows of the design's neutrals: black or white with or without an alpha inside
// box-shadow, text-shadow or drop-shadow(). A shadow is not a colour of the design, it darkens or lightens what is under it; anywhere
// else (color, background, border...) the colour rules stay as they are.
func withoutNeutralShadows(prop, value string) string {
	if prop == "box-shadow" || prop == "text-shadow" {
		return reNeutralColour.ReplaceAllString(value, " ")
	}
	return reDropShadow.ReplaceAllStringFunc(value, func(m string) string { return reNeutralColour.ReplaceAllString(m, " ") })
}

// colourLiteral returns the first colour of a CSS value that is not one of the design's (a hex, a colour function, a name), or "".
func colourLiteral(value string) string {
	v := reURL.ReplaceAllString(value, " ") // an id such as url(#bad) is not a colour
	v = reVar.ReplaceAllString(v, " ")
	v = reRelative.ReplaceAllString(v, " ")
	if h := reHex.FindString(v); h != "" {
		return h
	}
	if f := reColourFunc.FindString(v); f != "" {
		return strings.TrimSuffix(strings.ToLower(f), "(") + "()"
	}
	// A name is a word that is not part of a function name, a unit or a url: look at the bare words only.
	bare := reFuncName.ReplaceAllString(v, " ")
	for _, w := range reWord.FindAllString(bare, -1) {
		if namedColours[strings.ToLower(w)] {
			return w
		}
	}
	return ""
}

func clipText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// --- the page, drawn once ---------------------------------------------------------------------

// sceneMeasureScript reads, at the frame the session is on, the text that is drawn with its size in pixels, and how much of the frame
// the elements that are not text cover. An element counts as picture when it is drawn (display, visibility and the opacity of it
// and its ancestors above 0.05), sits in the stage of the part, and is either a leaf with no text of its own (a shape, an image, a
// canvas) or has a fill or a border. The cover is measured on a grid of 96 x 54 cells, a cell being covered when its centre is
// inside a box of the picture, so dots scattered over the frame count as what they cover and not as the box around them.
const sceneMeasureScript = `(function(){
  var W = innerWidth, H = innerHeight;
  function shown(el){
    var op = 1;
    for (var e = el; e && e !== document.documentElement; e = e.parentElement) {
      var cs = getComputedStyle(e);
      if (cs.display === 'none' || cs.visibility === 'hidden') return 0;
      op *= parseFloat(cs.opacity);
    }
    return op;
  }
  var texts = [], walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  while (walker.nextNode()) {
    var n = walker.currentNode, txt = n.nodeValue.replace(/\s+/g, ' ').trim();
    if (!txt) continue;
    var el = n.parentElement;
    if (!el || /^(SCRIPT|STYLE|NOSCRIPT|TITLE)$/i.test(el.tagName)) continue;
    if (shown(el) < 0.05) continue;
    var r = document.createRange(); r.selectNodeContents(n);
    var b = r.getBoundingClientRect();
    if (b.width < 1 || b.height < 1) continue;
    // The size that is seen: the computed font size times the scale the element is drawn at (its own transform and its ancestors': the
    // box it takes on screen over the box it takes in the layout), so a word grown with transform: scale() counts at its seen size.
    var scale = 1, lay = el.offsetHeight, seen = el.getBoundingClientRect().height;
    if (lay > 0 && seen > 0) scale = Math.min(20, Math.max(0.05, seen / lay));
    texts.push({text: txt.slice(0, 60), px: (parseFloat(getComputedStyle(el).fontSize) || 0) * scale});
  }
  var C = 96, R = 54, covered = new Uint8Array(C * R);
  var stage = document.getElementById('stage');
  var all = stage ? stage.querySelectorAll('*') : [];
  function alpha(c){
    var m = /rgba?\(([^)]*)\)/.exec(c); if (!m) return c === 'transparent' ? 0 : 1;
    var p = m[1].split(/[ ,\/]+/).filter(Boolean); return p.length > 3 ? parseFloat(p[3]) : 1;
  }
  for (var i = 0; i < all.length; i++) {
    var e = all[i];
    if (/^(SCRIPT|STYLE|NOSCRIPT|BR|TITLE|DEFS|CLIPPATH|MASK|LINEARGRADIENT|RADIALGRADIENT|STOP|FILTER|SYMBOL)$/i.test(e.tagName)) continue;
    if (shown(e) < 0.05) continue;
    var cs = getComputedStyle(e), own = false;
    for (var k = e.firstChild; k; k = k.nextSibling) { if (k.nodeType === 3 && k.nodeValue.trim()) own = true; }
    var leaf = e.children.length === 0 && !own;
    var fill = alpha(cs.backgroundColor) > 0.05 || (cs.backgroundImage && cs.backgroundImage !== 'none') ||
      (parseFloat(cs.borderTopWidth) > 0 && cs.borderTopStyle !== 'none') || (parseFloat(cs.borderBottomWidth) > 0 && cs.borderBottomStyle !== 'none');
    if (!leaf && !fill) continue;
    var q = e.getBoundingClientRect();
    if (q.width < 1 || q.height < 1) continue;
    var x0 = Math.max(0, Math.floor((q.left / W) * C)), x1 = Math.min(C - 1, Math.floor((q.right / W) * C));
    var y0 = Math.max(0, Math.floor((q.top / H) * R)), y1 = Math.min(R - 1, Math.floor((q.bottom / H) * R));
    for (var y = y0; y <= y1; y++) for (var x = x0; x <= x1; x++) {
      var cx = (x + 0.5) * W / C, cy = (y + 0.5) * H / R;
      if (cx >= q.left && cx <= q.right && cy >= q.top && cy <= q.bottom) covered[y * C + x] = 1;
    }
  }
  var n = 0; for (var j = 0; j < covered.length; j++) n += covered[j];
  return JSON.stringify({height: H, texts: texts, cover: n / covered.length});
})()`

type measuredText struct {
	Text string  `json:"text"`
	Px   float64 `json:"px"`
}

type sceneMeasure struct {
	Height float64        `json:"height"`
	Texts  []measuredText `json:"texts"`
	Cover  float64        `json:"cover"`
}

// wordsOf splits a text into lower-case words (letters and digits, with an inner apostrophe); the other characters are dropped.
func wordsOf(s string) []string {
	s = strings.NewReplacer("’", "'", "‘", "'").Replace(strings.ToLower(s))
	return reWordToken.FindAllString(s, -1)
}

func isNumber(w string) bool {
	for _, r := range w {
		if r < '0' || r > '9' {
			return false
		}
	}
	return w != ""
}

// allowedWords are the words a scene may show: its lines, the keyword, the word hit, the concept of the design and what the
// request adds (the idea asked for, the title of the song). verse is the part of them that is the sung text.
func allowedWords(sc Scene, extra string) (allowed, verse map[string]bool) {
	allowed, verse = map[string]bool{}, map[string]bool{}
	put := func(into map[string]bool, text string) {
		for _, w := range wordsOf(text) {
			into[w] = true
			if i := strings.Index(w, "'"); i > 0 {
				into[w[:i]] = true
			}
		}
	}
	for _, l := range sc.Lines {
		put(verse, l.Text)
		for _, w := range l.Words {
			put(verse, w.Text)
		}
	}
	if sc.Hit != nil {
		put(verse, sc.Hit.Word)
	}
	for w := range verse {
		allowed[w] = true
	}
	put(allowed, sc.Design.Concept)
	put(allowed, extra)
	return allowed, verse
}

// isGlue says a word is too short to count as a caption: one or two letters (a number is never glue).
func isGlue(w string) bool {
	return !isNumber(w) && utf8.RuneCountInString(w) <= 2
}

// judgeScene turns what was measured into the problems of the spec. The checks are separate so each can be tested without a browser.
func judgeScene(sc Scene, m sceneMeasure, extra string, at float64, ownsVerse bool) []string {
	var out []string
	allowed, verse := allowedWords(sc, extra)
	h := m.Height
	if h <= 0 {
		h = float64(sc.Height)
	}
	vh := func(px float64) float64 { return px / h * 100 }

	// Captions: a word that is in none of the places a scene may take its words from.
	var captions []string
	capSeen := map[string]bool{}
	for _, t := range m.Texts {
		for _, w := range wordsOf(t.Text) {
			if isGlue(w) || allowed[w] {
				continue
			}
			stem := w
			if i := strings.Index(w, "'"); i > 0 {
				stem = w[:i]
			}
			if allowed[stem] || isGlue(stem) {
				continue
			}
			// A verse word drawn in pieces (two spans) leaves a piece that is part of a word of the verse.
			if !isNumber(w) && utf8.RuneCountInString(w) >= 3 && insideAVerseWord(w, verse) {
				continue
			}
			if !capSeen[t.Text] {
				capSeen[t.Text] = true
				captions = append(captions, fmt.Sprintf("the scene shows %q which is not in the verse or the concept: scenes carry no captions (no section names, times, counters or route numbers; the HUD part does)", clipText(t.Text, 40)))
			}
			break
		}
	}
	out = append(out, firstN(captions, maxSceneProblems)...)

	// Sizes.
	var small []string
	for _, t := range m.Texts {
		if s := vh(t.Px); s < minTextVh {
			small = append(small, fmt.Sprintf("unreadable text %q at %.1f vh: nothing is drawn under %.0f vh", clipText(t.Text, 30), s, minTextVh))
		}
	}
	out = append(out, firstN(small, maxSceneProblems)...)
	if ownsVerse && len(sc.Lines) > 0 { // the floors of the verse are for the scene that draws it (verse: own); the page draws it otherwise
		var tiny []string
		biggest := 0.0
		for _, t := range m.Texts {
			s := vh(t.Px)
			biggest = math.Max(biggest, s)
			ws := wordsOf(t.Text)
			if len(ws) == 0 {
				continue
			}
			isVerse := true
			for _, w := range ws {
				if !verse[w] && !isGlue(w) {
					isVerse = false
				}
			}
			if isVerse && s >= minTextVh && s < minVerseVh {
				tiny = append(tiny, fmt.Sprintf("the verse word %q is drawn at %.1f vh: the words of the verse are at least %.0f vh", clipText(t.Text, 30), s, minVerseVh))
			}
		}
		out = append(out, firstN(tiny, maxSceneProblems)...)
		switch {
		case len(m.Texts) == 0:
			out = append(out, fmt.Sprintf("no text is drawn at %.1f s, in the middle of a sung line: the verse is on screen while it is sung, and its word that matters is at least %.0f vh", at, minHeadlineVh))
		case biggest < minHeadlineVh:
			out = append(out, headlineProblem(sc, at, biggest))
		}
	}

	// Presence of the picture.
	if m.Cover < minPictureShare {
		out = append(out, fmt.Sprintf("the picture covers %.0f %% of the frame at %.1f s: it is mostly empty (at least %.0f %% is expected; the picture is what is not text)", m.Cover*100, at, minPictureShare*100))
	}
	return out
}

// headlineProblem is the message for a scene whose largest text is under the floor: it names the word that matters and the CSS that reaches it.
func headlineProblem(sc Scene, at, biggest float64) string {
	msg := fmt.Sprintf("the largest text of the scene is %.1f vh (as drawn): the word that matters is at least %.0f vh (a scene with a verse has one big word, not captions over a picture)", biggest, minHeadlineVh)
	if kw := keywordAt(sc, at); kw != "" {
		msg += fmt.Sprintf(": give the element that shows %q font-size: %.0fvh, or transform: scale(%.1f) on a smaller one", kw, minHeadlineVh+2, math.Ceil(minHeadlineVh/math.Max(biggest, 1)*10+1)/10)
	}
	return msg
}

// keywordAt is the keyword of the line sung at t (or the nearest to it), as the word of the line; "" when no line has one.
func keywordAt(sc Scene, t float64) string {
	best, bestD := "", math.MaxFloat64
	for _, l := range sc.Lines {
		if l.Keyword < 0 {
			continue
		}
		w := ""
		if len(l.Words) > 0 && l.Keyword < len(l.Words) {
			w = l.Words[l.Keyword].Text
		} else if fs := strings.Fields(l.Text); l.Keyword < len(fs) {
			w = fs[l.Keyword]
		}
		if w = strings.ToLower(strings.Trim(w, ".,;:!?\"'()")); w == "" {
			continue
		}
		d := 0.0
		if t < l.Start {
			d = l.Start - t
		} else if t > l.End {
			d = t - l.End
		}
		if d < bestD {
			best, bestD = w, d
		}
	}
	return best
}

func insideAVerseWord(piece string, verse map[string]bool) bool {
	for w := range verse {
		if strings.Contains(w, piece) {
			return true
		}
	}
	return false
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return append(s[:n:n], fmt.Sprintf("and %d more of the same kind", len(s)-n))
	}
	return s
}

// sceneCheckTime is the frame the page is read at: the middle of the scene, moved to the middle of the sung line nearest to it
// so that the words of the verse are on screen.
func sceneCheckTime(sc Scene) float64 {
	mid := sc.Duration / 2
	best, bestD := -1.0, math.MaxFloat64
	for _, l := range sc.Lines {
		if l.End <= l.Start {
			continue
		}
		c := (l.Start + l.End) / 2
		d := 0.0
		if mid < l.Start {
			d = l.Start - mid
		} else if mid > l.End {
			d = mid - l.End
		}
		if d < bestD {
			best, bestD = c, d
		}
	}
	if best < 0 {
		return mid
	}
	return math.Min(math.Max(best, 0), math.Max(sc.Duration-0.05, 0))
}

// sceneFrameProblems draws the scene at sceneCheckTime and returns what is wrong with it (empty: it passes).
func sceneFrameProblems(ctx context.Context, browser string, sc Scene, extra string, ownsVerse bool) ([]string, error) {
	s, _, err := openCheckSession(ctx, browser, sc)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	at := sceneCheckTime(sc)
	if _, err := s.Frame(ctx, at); err != nil {
		return nil, err
	}
	var m sceneMeasure
	if err := s.EvalJSON(ctx, sceneMeasureScript, &m); err != nil {
		return nil, err
	}
	return judgeScene(sc, m, extra, at, ownsVerse), nil
}
