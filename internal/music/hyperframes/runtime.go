package hyperframes

import (
	"encoding/json"
	"fmt"
)

// runtimeJS stands in for the HyperFrames runtime inside the page of a block: it gives the block its variables
// (window.__hyperframes.getVariables), and, on every seek of the virtual clock, puts every timeline the block registered in
// window.__timelines at that time without firing its callbacks, and shows each .clip only over [data-start, data-start +
// data-duration). Nothing here reads a real clock, so two passes at the same time give the same frame.
const runtimeJS = `(function(){
  var VARS = %s;
  try { if (typeof window.name === 'string' && window.name.indexOf('hfvars:') === 0) VARS = JSON.parse(window.name.slice(7)); } catch(e){}
  window.__timelines = window.__timelines || {}; // the runtime of HyperFrames has it ready before the scripts of a block
  window.__hyperframes = { getVariables: function(){ return VARS; }, duration: %g };
  var styled = false;
  function clips(t){
    if (!styled) {
      styled = true;
      var style = document.createElement('style');
      style.textContent = 'html,body{margin:0;overflow:hidden}.clip.hf-off{visibility:hidden !important}';
      (document.head || document.documentElement).appendChild(style);
    }
    var els = document.querySelectorAll('.clip[data-start]');
    for (var i = 0; i < els.length; i++) {
      var s = parseFloat(els[i].getAttribute('data-start')), d = parseFloat(els[i].getAttribute('data-duration'));
      if (isNaN(s)) continue;
      var on = t >= s && (isNaN(d) || t < s + d);
      els[i].classList.toggle('hf-off', !on);
    }
  }
  window.__hfSeeks = 0;
  var ready = null;
  // The seek the GPU blocks (three, WebGPU shaders) know: the time in two variables and an event whose listeners may register work
  // to wait for (event.detail.waitUntil). The first seek also waits for what the page said it was building (__hf.buildReady).
  function gpu(t){
    var waits = [];
    if (!ready) {
      var b = (window.__hf && window.__hf.buildReady) || {};
      ready = Promise.all(Object.keys(b).map(function(k){ return b[k]; })).catch(function(){});
    }
    waits.push(ready);
    window.__hfThreeTime = t; window.__hfTypegpuTime = t;
    try {
      window.dispatchEvent(new CustomEvent('hf-seek', {detail: {time: t, waitUntil: function(p){ waits.push(p); }}}));
    } catch(e){ setTimeout(function(){ throw e; }); }
    return Promise.all(waits).then(function(){ return Promise.all(waits); });
  }
  window.__seek = function(t){
    var T = window.__timelines || {};
    for (var id in T) {
      var tl = T[id];
      if (tl && typeof tl.seek === 'function') { try { tl.pause(); tl.seek(t, false); } catch(e){ setTimeout(function(){ throw e; }); } }
    }
    clips(t);
    window.__hfSeeks++;
    return gpu(t);
  };
})();`

// SeedScript makes Math.random give the same numbers on every load of a page (a small seeded generator), so that a block that draws something
// at random (a stagger "from random", scattered particles) looks the same on every pass and every render, and its frames can be cached.
const SeedScript = `(function(){var s=0x9E3779B9;Math.random=function(){s|=0;s=s+0x6D2B79F5|0;var t=Math.imul(s^s>>>15,1|s);t=t+Math.imul(t^t>>>7,61|t)^t;return((t^t>>>14)>>>0)/4294967296;};})();`

// VarsPrefix starts the name of an iframe that carries the variables of the block it holds (the runtime reads them from window.name).
const VarsPrefix = "hfvars:"

// Runtime is the script a block's page runs before its own, with the block's variables (name -> value; nil for none) and length.
func Runtime(vars map[string]any, duration float64) string {
	if vars == nil {
		vars = map[string]any{}
	}
	b, _ := json.Marshal(vars)
	return SeedScript + "\n" + fmt.Sprintf(runtimeJS, b, duration)
}

// Defaults reads the default value of each variable a block declares (registry "variables": [{id, default, ...}]).
func Defaults(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	var vs []struct {
		ID      string `json:"id"`
		Default any    `json:"default"`
	}
	if json.Unmarshal(raw, &vs) != nil {
		return out
	}
	for _, v := range vs {
		if v.ID != "" && v.Default != nil {
			out[v.ID] = v.Default
		}
	}
	return out
}
