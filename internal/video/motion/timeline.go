package motion

// timeline.go: the timeline a page may declare as data. A page can hold one
// <script type="application/json" id="motion-timeline"> block with {version: 1, tracks: [...]};
// the script below, part of the virtual clock the renderer and the preview inject, applies it at
// each time before the page's own __seek, so the model writes data and the preview and the video
// show the same. See specs/motion-graphics ("A page may declare its animation as a timeline").

// TimelineProperties are the properties a track may animate, with their allowed range (min, max; a
// nil bound is open). The script and the instructions to the model say the same.
var TimelineProperties = []string{"opacity", "x", "y", "scale", "scaleX", "rotate", "blur", "reveal"}

// TimelineCurves are the names of the curves a track may use (or four numbers of a cubic-bezier).
var TimelineCurves = []string{"linear", "in", "out", "in-out", "out-back"}

// timelineScript wraps window.__motionSeek (defined by shim) so the tracks are applied first. It
// also exposes window.__motionTimelineProblems(), which checks the block against the real page, and
// window.__motionSetTimeline(data), which replaces the tracks (the editor's preview) and re-applies
// them at the current time.
const timelineScript = `(function(){
  var CURVES = {linear:[0,0,1,1], 'in':[.5,0,.75,0], out:[.22,1,.36,1], 'in-out':[.65,0,.35,1], 'out-back':[.34,1.56,.64,1]};
  var RANGES = {opacity:[0,1], x:[-1e5,1e5], y:[-1e5,1e5], scale:[0,1e3], scaleX:[0,1e3], rotate:[-1e5,1e5], blur:[0,1e3], reveal:[0,1]};
  var NEUTRAL = {x:0, y:0, scale:1, scaleX:1, rotate:0, opacity:1, blur:0, reveal:1};
  var override = null, lastT = 0, cache = null;
  function bezier(p){
    var x1=p[0], y1=p[1], x2=p[2], y2=p[3];
    var cx=3*x1, bx=3*(x2-x1)-cx, ax=1-cx-bx, cy=3*y1, by=3*(y2-y1)-cy, ay=1-cy-by;
    function sx(t){ return ((ax*t+bx)*t+cx)*t; }
    function sy(t){ return ((ay*t+by)*t+cy)*t; }
    function dx(t){ return (3*ax*t+2*bx)*t+cx; }
    return function(x){
      if (x <= 0) return 0; if (x >= 1) return 1;
      var t = x, i, e;
      for (i = 0; i < 8; i++) { e = sx(t) - x; if (Math.abs(e) < 1e-6) return sy(t); var d = dx(t); if (Math.abs(d) < 1e-6) break; t -= e / d; }
      var lo = 0, hi = 1; t = x;
      while (lo < hi) { e = sx(t); if (Math.abs(e - x) < 1e-6) break; if (x > e) lo = t; else hi = t; t = (hi - lo) / 2 + lo; if (hi - lo < 1e-7) break; }
      return sy(t);
    };
  }
  function curve(ease){
    var p = Array.isArray(ease) ? ease : CURVES[ease == null ? 'out' : ease];
    return p ? bezier(p) : null;
  }
  function block(){
    if (override) return override;
    if (cache) return cache.v;
    var el = document.getElementById('motion-timeline'), v = null, err = null;
    if (el) { try { v = JSON.parse(el.textContent); } catch (e) { err = e.message; } }
    cache = {v: v, err: err, present: !!el};
    return v;
  }
  function num(v){ return typeof v === 'number' && isFinite(v); }
  function clamp(p, v){ var r = RANGES[p]; return Math.min(r[1], Math.max(r[0], v)); }
  function apply(t){
    var tl = block();
    if (!tl || !Array.isArray(tl.tracks)) return;
    var per = new Map();
    tl.tracks.forEach(function(tr){
      if (!tr || typeof tr.target !== 'string' || !num(tr.start) || !num(tr.duration) || tr.duration <= 0) return;
      var els; try { els = document.querySelectorAll(tr.target); } catch (e) { return; }
      var f = tr.from || {}, to = tr.to || {}, ease = curve(tr.ease) || curve('out'), stagger = num(tr.stagger) ? tr.stagger : 0;
      Array.prototype.forEach.call(els, function(el, i){
        var p = Math.min(1, Math.max(0, (t - (tr.start + i * stagger)) / tr.duration)), k = ease(p);
        var st = per.get(el); if (!st) { st = {}; per.set(el, st); }
        Object.keys(RANGES).forEach(function(name){
          if (num(f[name]) && num(to[name])) st[name] = clamp(name, f[name] + (to[name] - f[name]) * k);
        });
      });
    });
    per.forEach(function(st, el){
      var tf = [];
      if (st.x !== undefined || st.y !== undefined) tf.push('translate(' + (st.x || 0) + 'px,' + (st.y || 0) + 'px)');
      if (st.rotate !== undefined) tf.push('rotate(' + st.rotate + 'deg)');
      if (st.scale !== undefined || st.scaleX !== undefined) {
        var s = st.scale === undefined ? 1 : st.scale, sx = st.scaleX === undefined ? 1 : st.scaleX;
        tf.push('scale(' + (s * sx) + ',' + s + ')');
      }
      if (tf.length) el.style.transform = tf.join(' ');
      if (st.opacity !== undefined) el.style.opacity = String(st.opacity);
      if (st.blur !== undefined) el.style.filter = st.blur > 0 ? 'blur(' + st.blur + 'px)' : 'none';
      if (st.reveal !== undefined) el.style.clipPath = st.reveal >= 1 ? 'none' : 'inset(0 ' + ((1 - st.reveal) * 100) + '% 0 0)';
    });
  }
  var base = window.__motionSeek;
  window.__motionSeek = function(t){ lastT = t; try { apply(t); } catch (e) { setTimeout(function(){ throw e; }); } return base(t); };
  window.__motionSetTimeline = function(data){ override = data; if (window.__motionSeek) window.__motionSeek(lastT); return true; };
  window.__motionTimelineProblems = function(){
    var out = [], tl = block(), dur = window.__motionDuration || 0;
    block();
    if (!override && cache && !cache.present) return out;
    if (!override && cache && cache.err) return ['the timeline block is not valid JSON: ' + cache.err];
    if (!tl || typeof tl !== 'object') return ['the timeline block must be a JSON object {"version": 1, "tracks": [...]}'];
    if (tl.version !== 1) out.push('the timeline "version" must be 1');
    if (!Array.isArray(tl.tracks)) { out.push('the timeline needs a "tracks" array'); return out; }
    var ids = {};
    tl.tracks.forEach(function(tr, i){
      var name = 'track ' + (tr && typeof tr.id === 'string' && tr.id ? '"' + tr.id + '"' : '#' + (i + 1));
      if (!tr || typeof tr !== 'object') { out.push(name + ' is not an object'); return; }
      if (typeof tr.id !== 'string' || !tr.id) out.push(name + ' has no "id"'); else if (ids[tr.id]) out.push(name + ' repeats an id'); else ids[tr.id] = 1;
      var els = null;
      if (typeof tr.target !== 'string' || !tr.target) out.push(name + ' has no "target" selector');
      else { try { els = document.querySelectorAll(tr.target); } catch (e) { out.push(name + ': the selector "' + tr.target + '" is not valid'); } }
      if (els && els.length === 0) out.push(name + ': the selector "' + tr.target + '" matches no element of the page');
      if (!num(tr.start) || tr.start < 0) out.push(name + ': "start" must be a number of seconds, 0 or more');
      if (!num(tr.duration) || tr.duration <= 0) out.push(name + ': "duration" must be a number of seconds, more than 0');
      if (tr.stagger !== undefined && (!num(tr.stagger) || tr.stagger < 0)) out.push(name + ': "stagger" must be a number of seconds, 0 or more');
      if (num(tr.start) && num(tr.duration) && dur > 0) {
        var n = els ? els.length : 1, end = tr.start + (n - 1) * (num(tr.stagger) ? tr.stagger : 0) + tr.duration;
        if (end > dur + 0.05) out.push(name + ' ends at ' + end.toFixed(2) + 's, after the end of the clip (' + dur.toFixed(2) + 's)');
      }
      if (tr.ease !== undefined && !(Array.isArray(tr.ease) ? tr.ease.length === 4 && tr.ease.every(num) : typeof tr.ease === 'string' && CURVES[tr.ease])) {
        out.push(name + ': "ease" must be one of ' + Object.keys(CURVES).join(', ') + ' or four numbers of a cubic-bezier');
      }
      ['from', 'to'].forEach(function(side){
        var o = tr[side];
        if (!o || typeof o !== 'object' || Array.isArray(o)) { out.push(name + ' has no "' + side + '" object'); return; }
        Object.keys(o).forEach(function(k){
          if (!RANGES[k]) out.push(name + ': "' + k + '" in "' + side + '" is not a property (use ' + Object.keys(RANGES).join(', ') + ')');
          else if (!num(o[k])) out.push(name + ': "' + k + '" in "' + side + '" must be a number');
          else if (o[k] < RANGES[k][0] || o[k] > RANGES[k][1]) out.push(name + ': "' + k + '" in "' + side + '" is out of range (' + RANGES[k][0] + ' to ' + RANGES[k][1] + ')');
        });
      });
      if (els) Array.prototype.forEach.call(els, function(el){
        var an = getComputedStyle(el).animationName;
        if (an && an !== 'none') out.push(name + ': the element "' + tr.target + '" also has a CSS animation (' + an.split(',')[0] + '): animate it in the timeline or in CSS, not in both');
      });
    });
    return out;
  };
  window.__motionTimelineHas = function(){ block(); return !!(override || (cache && cache.present)); };
})();`
