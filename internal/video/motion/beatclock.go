package motion

// beatclock.go: the beat clock a page of a music motion piece may read. The renderer and the preview inject it
// next to the virtual clock, so that at each time t, before the page's own __seek runs, window.__beat holds
//   {number, position, section, sectionIndex, bpm, period, time}
// number is the index (from 0) of the last beat at or before t (-1 before the first beat), position is how far
// t is inside that beat (0 up to 1, never 1) and section is the tag of the section that holds t. The song map
// is also given as window.__motionSong. A page that is not part of a piece gets none of it. See
// specs/motion-graphics ("A page may be given the song map and a beat clock").

import (
	"encoding/json"
	"fmt"
	"strings"
)

const beatScript = `(function(){
  var song = window.__motionSong || {}, beats = song.beats || [], secs = song.sections || [], bpm = song.bpm || 0;
  var basePeriod = bpm > 0 ? 60 / bpm : (beats.length > 1 ? beats[1] - beats[0] : 0);
  function compute(t){
    var lo = 0, hi = beats.length - 1, n = -1;
    while (lo <= hi) { var mid = (lo + hi) >> 1; if (beats[mid] <= t + 1e-9) { n = mid; lo = mid + 1; } else hi = mid - 1; }
    // The length of the beat the frame is in: a song that was followed in the audio does not keep an exact tempo.
    var period = basePeriod;
    if (n >= 0 && n + 1 < beats.length) period = beats[n + 1] - beats[n];
    else if (n > 0) period = beats[n] - beats[n - 1];
    var pos = 0;
    if (period > 0) {
      var from = n >= 0 ? beats[n] : (beats.length ? beats[0] - period : 0);
      pos = Math.min(0.999999, Math.max(0, (t - from) / period));
    }
    var si = -1, tag = '';
    for (var i = 0; i < secs.length; i++) { if (t >= secs[i].start - 1e-9 && (t < secs[i].end - 1e-9 || i === secs.length - 1)) { si = i; tag = secs[i].tag; break; } }
    return {number: n < 0 ? n : n + (song.beat_base || 0), position: pos, section: tag, sectionIndex: si, bpm: bpm, period: period, time: t};
  }
  window.__beat = compute(0);
  var base = window.__motionSeek;
  window.__motionSeek = function(t){ try { window.__beat = compute(t); } catch (e) { setTimeout(function(){ throw e; }); } return base(t); };
})();`

// ShimSong is Shim for a page that belongs to a piece: the virtual clock, the timeline and the beat clock for the
// given song map (JSON). An empty song gives the plain Shim, so a page outside a piece is left as it was.
func ShimSong(duration float64, song json.RawMessage) string {
	if len(strings.TrimSpace(string(song))) == 0 {
		return Shim(duration)
	}
	return Shim(duration) + fmt.Sprintf("window.__motionSong = %s;\n", song) + beatScript
}

// PreviewHTMLSong is PreviewHTML for a page of a piece: it injects the same beat clock the renderer does.
func PreviewHTMLSong(html string, duration float64, song json.RawMessage) string {
	return previewWith(html, ShimSong(duration, song))
}
