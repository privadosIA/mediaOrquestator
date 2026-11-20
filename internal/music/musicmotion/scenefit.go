package musicmotion

// scenefit.go: the repairs of a scene that only the drawn page shows (spec motion-scene-parts, "A scene draft is repaired before it
// is refused"). The checks that measure the page (the word that matters under 12 vh, two texts that overlap, a text cut by the
// frame's edge) say what is wrong at a moment of the clip; the repair is a small script at the end of the fragment that, after every
// seek of the page and of its own layers (window.__afterSeek, called after the layers are drawn), measures its texts the way the checks do and sets what is missing: the font-size of the word that matters, and
// the CSS property translate (which does not touch the scene's own transform) of the texts that overlap or leave the frame. The
// caller draws the repaired page and keeps it only if the same checks pass; otherwise the model's own fragment stays.

import (
	"context"
	"encoding/json"
	"strings"
)

// The kinds of repair the drawn page asks for.
const (
	RepairSize    = "size"    // the word that matters is raised to the floor of the headline
	RepairOverlap = "overlap" // texts that cover each other are moved apart
	RepairCut     = "cut"     // a text cut by the left or right edge is moved or shrunk into the frame
)

// fitMark tells a fragment that has been through RepairLayout.
const fitMark = "/*fit*/"

// LayoutKinds says which of the layout repairs the problems of the checks ask for.
func LayoutKinds(problems []string) []string {
	var kinds []string
	has := func(sub string) bool {
		for _, p := range problems {
			if strings.Contains(p, sub) {
				return true
			}
		}
		return false
	}
	if has("the largest text of the scene is") {
		kinds = append(kinds, RepairSize)
	}
	if has("is cut by the left edge") || has("is cut by the right edge") {
		kinds = append(kinds, RepairCut)
	}
	if has(" overlap at ") {
		kinds = append(kinds, RepairOverlap)
	}
	return kinds
}

// SceneKeywords are the words of the scene that matter, in lower case: the keyword of each of its lines.
func SceneKeywords(sc Scene) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range sc.Lines {
		w := keywordAt(Scene{Lines: []Line{l}}, l.Start)
		if w != "" && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// RepairLayout adds to a scene fragment the script that sets the kinds of layout repair asked for. A fragment that already has it, or
// that has no script of its own to wrap, comes back unchanged and with no repairs.
func RepairLayout(fragment string, keywords, kinds []string) (string, []Repair) {
	if len(kinds) == 0 || strings.Contains(fragment, fitMark) || !reFirstScript.MatchString(fragment) {
		return fragment, nil
	}
	do := map[string]bool{}
	var reps []Repair
	for _, k := range kinds {
		do[k] = true
		reps = append(reps, Repair{Kind: k, Count: 1})
	}
	kw, _ := json.Marshal(keywords)
	dj, _ := json.Marshal(do)
	script := "\n<script>" + fitMark + strings.NewReplacer("__KW__", string(kw), "__DO__", string(dj)).Replace(fitScript) + "</script>\n"
	return fragment + script, reps
}

// fitScript resets its changes before window.__partSeek of the scene (defined by the scripts above it) and applies the layout repairs in window.__afterSeek. It measures the text nodes
// that are drawn as the checks do (display, visibility and opacity of the element and its ancestors, a range around the node).
const fitScript = `(function(){
var KW=__KW__, DO=__DO__, seek=window.__partSeek;
if(typeof seek!=='function')return;
var touched=[];
function seenScale(el){var lay=el.offsetHeight,seen=el.getBoundingClientRect().height; return lay>0&&seen>0?Math.min(20,Math.max(0.05,seen/lay)):1}
function px(el){return parseFloat(getComputedStyle(el).fontSize)||0}
function boxes(){
  var out=[],w=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);
  while(w.nextNode()){
    var n=w.currentNode,t=n.nodeValue.replace(/\s+/g,' ').trim(); if(!t)continue;
    var el=n.parentElement; if(!el||/^(SCRIPT|STYLE|NOSCRIPT|TITLE)$/.test(el.tagName))continue;
    var shown=true,op=1;
    for(var e=el;e;e=e.parentElement){var cs=getComputedStyle(e); if(cs.display==='none'||cs.visibility==='hidden'){shown=false;break} op*=parseFloat(cs.opacity)}
    if(!shown||op<0.05)continue;
    var r=document.createRange(); r.selectNodeContents(n); var b=r.getBoundingClientRect(); if(b.width<1||b.height<1)continue;
    out.push({el:el,text:t.toLowerCase(),l:b.left,t:b.top,r:b.right,b:b.bottom});
  }
  return out;
}
function house(el){return !!(el.closest&&el.closest('#verse,#hit,#hitbg'))}
function hide(el){ touch(el); el.style.setProperty('visibility','hidden','important') }
function touch(el){ if(touched.indexOf(el)<0){touched.push(el); el.__fitFS=el.style.fontSize; el.__fitT=[0,0]} }
function size(el,v){ touch(el); el.style.setProperty('font-size',v+'px','important') }
function move(el,dx,dy){ touch(el); var t=el.__fitT; t[0]+=dx; t[1]+=dy; el.style.setProperty('translate',t[0]+'px '+t[1]+'px','important') }
function reset(){ touched.forEach(function(el){ el.style.removeProperty('translate'); el.style.removeProperty('font-size'); el.style.removeProperty('visibility'); if(el.__fitFS)el.style.fontSize=el.__fitFS; el.__fitT=[0,0] }) }
function fit(){
  var W=window.innerWidth,H=window.innerHeight,bs,pick;
  if(DO.size){
    bs=boxes().filter(function(b){return !house(b.el)}); pick=null;
    bs.forEach(function(b){
      var ws=b.text.split(/[\s.,;:!?"()]+/).filter(Boolean);
      if(ws.length&&ws.every(function(x){return KW.indexOf(x)>=0})&&(!pick||px(b.el)>px(pick.el)))pick=b;
    });
    if(!pick)bs.forEach(function(b){ if(!pick||px(b.el)>px(pick.el))pick=b });
    if(pick){ var sc=seenScale(pick.el),seen=px(pick.el)*sc; if(seen<H*0.14)size(pick.el,H*0.14/sc) }
  }
  if(DO.cut){
    var m=W*0.02,done=[];
    boxes().forEach(function(b){
      if(b.r<=0||b.l>=W||done.indexOf(b.el)>=0)return;
      var wd=b.r-b.l,dx=0;
      if(b.l<-1)dx=m-b.l; else if(b.r>W+1)dx=(W-m)-b.r; else return;
      done.push(b.el);
      if(wd>W-2*m){ size(b.el,px(b.el)*(W-2*m)/wd); var c=boxes().filter(function(x){return x.el===b.el})[0]; if(c){ dx=c.l<m?m-c.l:(c.r>W-m?(W-m)-c.r:0) } else dx=0 }
      if(dx)move(b.el,dx,0);
    });
  }
  if(DO.overlap){
    // the same word drawn twice on top of itself (the scene's copy and the page's hit): one copy goes, the page's first
    bs=boxes();
    for(var x=0;x<bs.length;x++)for(var y=x+1;y<bs.length;y++){
      var p1=bs[x],p2=bs[y]; if(p1.el===p2.el||p1.text!==p2.text)continue;
      var ow=Math.min(p1.r,p2.r)-Math.max(p1.l,p2.l),oh=Math.min(p1.b,p2.b)-Math.max(p1.t,p2.t);
      if(ow<=0||oh<=0||ow*oh/Math.min((p1.r-p1.l)*(p1.b-p1.t),(p2.r-p2.l)*(p2.b-p2.t))<=0.1)continue;
      if(house(p1.el)&&house(p2.el))continue;
      hide(house(p1.el)?p1.el:house(p2.el)?p2.el:((p1.r-p1.l)*(p1.b-p1.t)<=(p2.r-p2.l)*(p2.b-p2.t)?p1.el:p2.el));
    }
    for(var pass=0;pass<12;pass++){
      bs=boxes(); var moved=false;
      outer: for(var i=0;i<bs.length;i++)for(var j=i+1;j<bs.length;j++){
        var a=bs[i],b=bs[j]; if(a.el===b.el)continue;
        var w=Math.min(a.r,b.r)-Math.max(a.l,b.l),h=Math.min(a.b,b.b)-Math.max(a.t,b.t);
        if(w<=0||h<=0)continue;
        var aa=(a.r-a.l)*(a.b-a.t),ba=(b.r-b.l)*(b.b-b.t),small=Math.min(aa,ba);
        if(!(small>0&&w*h/small>0.1))continue;
        var ah=house(a.el),bh=house(b.el);
        if(ah&&bh)continue;
        var mv=a,other=b;
        if(ah)mv=b,other=a; else if(!bh&&ba<aa)mv=b,other=a; else if(!ah&&!bh&&ba>=aa)mv=a,other=b;
        if(mv.text===other.text){ hide(ah||bh?(ah?a.el:b.el):mv.el); moved=true; break outer } // the scene draws the word itself: the page's copy of it (the hit) goes
        var gap=H*0.01,down=other.b-mv.t+gap,up=mv.b-other.t+gap;
        var dy=(mv.b+down>H&&mv.t-up>=0)||up<down&&mv.t-up>=0?-up:down;
        move(mv.el,0,dy); moved=true; break outer;
      }
      if(!moved)break;
    }
  }
}
window.__partSeek=function(){ reset(); return seek.apply(this,arguments) };
window.__afterSeek=function(){ try{fit()}catch(e){} };
})();`

// repairLayout tries the layout repairs the problems ask for, then all three, drawing and checking the page each time. It says the
// repaired fragment, its part and what was repaired when a version passes every check.
func (e *DraftEngine) repairLayout(ctx context.Context, sc Scene, manifest []byte, fragment string, problems []string, opts CheckOptions) (string, Part, []Repair, bool) {
	kinds := LayoutKinds(problems)
	if len(kinds) == 0 {
		return "", Part{}, nil, false
	}
	keywords := SceneKeywords(sc)
	tries := [][]string{kinds}
	if len(kinds) < 3 {
		tries = append(tries, []string{RepairSize, RepairCut, RepairOverlap})
	}
	for _, ks := range tries {
		fixed, reps := RepairLayout(fragment, keywords, ks)
		if len(reps) == 0 {
			return "", Part{}, nil, false
		}
		part, probs := ParsePart(manifest, fixed)
		if len(probs) > 0 {
			return "", Part{}, nil, false
		}
		got, err := CheckPart(ctx, e.Browser, part, opts)
		if ctx.Err() != nil || err != nil {
			return "", Part{}, nil, false
		}
		if len(got) == 0 {
			return fixed, part, reps, true
		}
	}
	return "", Part{}, nil, false
}
