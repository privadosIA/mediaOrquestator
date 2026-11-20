package musicmotion

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// GSAPFile is where scripts/get-gsap.sh installs the GSAP core, relative to the server's root (the environment report
// looks at the same path). The file is not part of the repository: see THIRD_PARTY.md.
var GSAPFile = filepath.Join("third_party", "gsap", "gsap.min.js")

// GSAPInstall is what a person runs when the file is missing.
const GSAPInstall = "bash scripts/get-gsap.sh"

var (
	reUsesGSAP  = regexp.MustCompile(`\bgsap\b|__timelines`)
	reUsesClips = regexp.MustCompile(`class\s*=\s*["'][^"']*\bclip\b`)
)

// usesGSAP says whether a fragment needs the GSAP file in its page: it names gsap or registers a timeline.
func usesGSAP(frag string) bool { return reUsesGSAP.MatchString(frag) }

// usesTimelines says whether a fragment takes part in the timeline contract: it needs GSAP or it marks clips.
func usesTimelines(frag string) bool { return usesGSAP(frag) || reUsesClips.MatchString(frag) }

var gsapCache struct {
	sync.Mutex
	key string
	src string
}

// gsapSource reads the installed GSAP core, ready to go inside a <script>. It is read again only when the file changes, and
// a missing file is not remembered, so installing it takes effect without restarting the server.
func gsapSource() (string, error) {
	fi, err := os.Stat(GSAPFile)
	if err != nil || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not installed on this server (run `%s`): do not use GSAP, animate from window.__partSeek only", filepath.ToSlash(GSAPFile), GSAPInstall)
	}
	abs, _ := filepath.Abs(GSAPFile)
	key := fmt.Sprintf("%s|%d|%d", abs, fi.Size(), fi.ModTime().UnixNano())
	gsapCache.Lock()
	defer gsapCache.Unlock()
	if gsapCache.key == key {
		return gsapCache.src, nil
	}
	b, err := os.ReadFile(GSAPFile)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", filepath.ToSlash(GSAPFile), err)
	}
	// "</script" inside the library's text would end the element: written as an escape, the same text to JavaScript.
	gsapCache.key, gsapCache.src = key, strings.ReplaceAll(string(b), "</script", `<\/script`)
	return gsapCache.src, nil
}

// timelineCSS hides a clip outside its range; the runtime toggles the class (the fragment's own display is untouched).
const timelineCSS = `.clip.hf-off{visibility:hidden !important}`

// timelineRuntime is added to a page whose parts use timelines or clips. Each fragment, once it has run, hands over what it
// registered (claim): the timelines in window.__timelines (the object is emptied, so the next fragment may use the same ids)
// and the clips not yet taken. The seek goes to every timeline of an owner at the part's own time, without firing callbacks
// (a timeline with onUpdate side effects stays deterministic), and then shows each clip only over [start, start + duration).
const timelineRuntime = `(function(){
  var main = {tls: [], clips: []}, all = [];
  window.__hf = {main: main, all: all,
    claim: function(owner){
      var T = window.__timelines || {};
      for (var id in T) { if (T[id] && typeof T[id].seek === 'function') { owner.tls.push(T[id]); all.push(T[id]); } }
      window.__timelines = {};
      var els = document.querySelectorAll('.clip[data-start]');
      for (var i = 0; i < els.length; i++) { if (!els[i].__hfOwned) { els[i].__hfOwned = true; owner.clips.push(els[i]); } }
      if (window.gsap && gsap.ticker && gsap.ticker.sleep) gsap.ticker.sleep();
      return owner;
    },
    // problems looks at what the registered timelines do to the page (once the part has been drawn): it returns what the part
    // must change and, apart, what only deserves a warning (a clip that ends after the scene does).
    problems: function(duration){
      var out = [], warn = [], seen = {};
      function add(list, m){ if (!seen[m]) { seen[m] = 1; list.push(m); } }
      function label(el){ var c = typeof el.className === 'string' ? el.className.trim().split(/\s+/)[0] : ''; return el.id ? '#' + el.id : (c ? '.' + c : el.tagName.toLowerCase()); }
      function sheetTransform(el){
        for (var si = 0; si < document.styleSheets.length; si++) {
          var rules; try { rules = document.styleSheets[si].cssRules; } catch (e) { continue; }
          for (var ri = 0; ri < rules.length; ri++) {
            var r = rules[ri];
            if (r.style && r.selectorText && r.style.transform && r.style.transform !== 'none') { try { if (el.matches(r.selectorText)) return r.selectorText; } catch (e) {} }
          }
        }
        return '';
      }
      var VIS = ['display', 'visibility', 'autoAlpha'], MOVE = ['x', 'y', 'xPercent', 'yPercent', 'rotation', 'rotate', 'rotationX', 'rotationY', 'scale', 'scaleX', 'scaleY', 'skewX', 'skewY', 'transform'];
      function has(v, k){ return v && (k in v || (v.css && k in v.css)); }
      var FOREVER = 'a timeline repeats forever (repeat: -1): the page seeks every timeline to the time of the frame, so an endless repeat is not needed and it hides a part that only loops; give it a finite number of repeats or build the motion over the length of the part';
      all.forEach(function(tl){
        if (tl.repeat && tl.repeat() === -1) add(out, FOREVER);
        tl.getChildren(true, true, true).forEach(function(c){
          if (c.repeat && c.repeat() === -1) add(out, FOREVER);
          if (typeof c.targets !== 'function') return;
          var v = c.vars || {}, starts = v.startAt || null;
          c.targets().forEach(function(el){
            if (!el || !el.classList) return;
            VIS.forEach(function(k){
              if ((has(v, k) || has(starts, k)) && el.classList.contains('clip')) add(out, 'a timeline tweens "' + k + '" of the clip ' + label(el) + ': a clip is shown and hidden by its data-start and data-duration; animate a child of the clip instead');
            });
            if (MOVE.some(function(k){ return has(v, k) || has(starts, k); })) {
              var sel = sheetTransform(el);
              if (sel) add(out, 'the element ' + label(el) + ' is moved by a timeline and also has a CSS transform (the rule "' + sel + '"): take the transform out of the stylesheet and set the starting position inside the tween (gsap.set or gsap.fromTo)');
            }
          });
        });
      });
      var els = document.querySelectorAll('.clip[data-start]');
      for (var i = 0; i < els.length; i++) {
        var s = parseFloat(els[i].getAttribute('data-start')) || 0, d = parseFloat(els[i].getAttribute('data-duration'));
        if (!isNaN(d) && duration > 0 && s + d > duration + 1e-6) add(warn, 'the clip ' + label(els[i]) + ' ends at ' + (s + d) + ' s, after the end of the part (' + duration + ' s)');
      }
      return {problems: out, warnings: warn};
    },
    seek: function(owner, t){
      for (var i = 0; i < owner.tls.length; i++) { var tl = owner.tls[i]; tl.seek(Math.max(0, Math.min(t, tl.duration())), true); }
      for (var k = 0; k < owner.clips.length; k++) {
        var el = owner.clips[k], s = parseFloat(el.getAttribute('data-start')) || 0, d = parseFloat(el.getAttribute('data-duration'));
        var on = t >= s - 1e-9 && (isNaN(d) || t < s + d - 1e-9);
        el.classList.toggle('hf-off', !on);
      }
    }};
})();`

// timelineUse says whether the page of the scene needs the timeline runtime (any part marks clips or registers timelines) and
// whether it needs the GSAP file (any of them names gsap or registers a timeline).
func (s Scene) timelineUse() (runtime, gsap bool) {
	frags := []string{s.Part.Fragment}
	for _, e := range s.Extras {
		frags = append(frags, e.Part.Fragment)
	}
	for _, l := range s.Layers {
		frags = append(frags, l.Part.Fragment)
	}
	for _, f := range frags {
		runtime = runtime || usesTimelines(f)
		gsap = gsap || usesGSAP(f)
	}
	return runtime, gsap
}

// timelineInstruction is what the writers of parts and of scenes are told about animating with a GSAP timeline (spec
// music-motion-parts and motion-scene-parts) and the rules of movement distilled from HyperFrames' motion guide, as two
// bullets of their rules. unit is "scene" or "part".
func timelineInstruction(unit string) string {
	return `- You may animate with GSAP instead of, or besides, window.__partSeek (a ` + unit + ` that moves only through its timeline does not need __partSeek). Build ONE paused timeline while the script loads: var tl = gsap.timeline({paused: true}); add your tweens at absolute times, in the seconds from the start of this ` + unit + `; then window.__timelines = window.__timelines || {}; window.__timelines['` + unit + `'] = tl; gsap.ticker.sleep(). The page seeks it to the time of every frame. An element that exists only for a while is marked class="clip" data-start="s" data-duration="d" (seconds of this ` + unit + `): the page shows it only inside that range. The checks enforce: never tween display, visibility or autoAlpha of a clip (animate a child of it); no CSS transform on an element GSAP transforms (put its starting position inside the tween, with gsap.set or gsap.fromTo); no repeat: -1; GSAP core only, no plugins (no SplitText, ScrollTrigger, MotionPath, ...). Keep window.__partSeek(t, beat) for what reacts to the beat (beat.position, beat.word). If a check says GSAP is not installed on this server, do not use it: animate from window.__partSeek only.
- Rules of movement: entrances ease out (power3.out, expo.out) and exits ease in and are shorter; hold at least one beat before a hit; nothing linear except a steady drift; one punch-in (a quick scale) on the strongest beat of the ` + unit + `; every change has a cause in the song (a word, a beat or a section).`
}
