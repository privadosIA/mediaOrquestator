package hyperframes

import (
	"os"
	"path/filepath"
	"sync"
)

// UpstreamFile is where scripts/get-hyperframes-runtime.sh puts the runtime of HyperFrames built from its source, relative to the server's
// working directory. It is not committed; without it blocks are drawn with this package's own small runtime.
var UpstreamFile = filepath.Join("third_party", "hyperframes-runtime", "hyperframe.runtime.iife.js")

var upstreamCache struct {
	sync.Mutex
	key string
	src string
}

// UpstreamRuntime is the built runtime, read again only when the file changes, or false when it is not installed.
func UpstreamRuntime() (string, bool) {
	fi, err := os.Stat(UpstreamFile)
	if err != nil || !fi.Mode().IsRegular() {
		return "", false
	}
	key := UpstreamFile + "|" + fi.ModTime().String() + "|" + string(rune(fi.Size()))
	upstreamCache.Lock()
	defer upstreamCache.Unlock()
	if upstreamCache.key == key {
		return upstreamCache.src, true
	}
	b, err := os.ReadFile(UpstreamFile)
	if err != nil {
		return "", false
	}
	upstreamCache.key, upstreamCache.src = key, string(b)
	return upstreamCache.src, true
}

// upstreamBridge connects the page's seek (window.__seek, which the virtual clock calls with the time of each frame) to the player
// of the upstream runtime: the first seek waits for the player to be ready and for what the page said it was building
// (__hf.buildReady), then every seek is a renderSeek and a short settle.
const upstreamBridge = `(function(){
  var ready = null;
  window.__seek = function(t){
    if (!ready) {
      ready = new Promise(function(res){
        var n = 0;
        (function poll(){ if ((window.__player && window.__playerReady) || n > 200) res(); else { n++; setTimeout(poll, 25); } })();
      }).then(function(){
        var b = (window.__hf && window.__hf.buildReady) || {};
        return Promise.all(Object.keys(b).map(function(k){ return b[k]; })).catch(function(){});
      });
    }
    return ready.then(function(){
      if (window.__player && window.__player.renderSeek) window.__player.renderSeek(t);
      // the GPU work the page's hf-seek listeners registered, then a short settle
      var w = typeof window.__hfWaitForSeekCompletion === 'function' ? window.__hfWaitForSeekCompletion({videoFrames: false, timeoutMs: 8000}) : null;
      return Promise.resolve(w).catch(function(){}).then(function(){ return new Promise(function(r){ setTimeout(r, 50); }); });
    });
  };
})();`

// UpstreamInit is the init script of a page drawn with the upstream runtime: the runtime and its bridge. With onlyFrames it runs in
// iframes only (the page of a piece, which has its own seek, embeds blocks in iframes). False when the runtime is not installed.
func UpstreamInit(onlyFrames bool) (string, bool) {
	src, ok := UpstreamRuntime()
	if !ok {
		return "", false
	}
	js := src + "\n" + upstreamBridge
	if onlyFrames {
		js = "if (window.self !== window.top) {\n" + js + "\n}"
	}
	return js, true
}
