package motion

import (
	"regexp"
	"strings"
)

// previewCSP lets a previewed page run its own inline scripts and styles and nothing else:
// no network, no frames, no forms.
const previewCSP = `default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src data:; font-src data:; media-src data:`

// previewScript lets the editor move the page's clock: it listens for {type:"seek", t:<seconds>}
// messages from the page that embeds it, and for {type:"timeline", data:<block>}, which replaces the tracks
// of the page's timeline without reloading it.
const previewScript = `window.addEventListener("message",function(e){var d=e.data;if(d&&d.type==="seek"&&typeof d.t==="number"&&window.__motionSeek){window.__motionSeek(d.t);}if(d&&d.type==="timeline"&&window.__motionSetTimeline){window.__motionSetTimeline(d.data);}});`

var headOpen = regexp.MustCompile(`(?i)<head[^>]*>`)
var htmlOpen = regexp.MustCompile(`(?i)<html[^>]*>`)

// PreviewHTML is the page prepared for the editor's sandboxed frame: the content security
// policy and the same virtual-clock script the renderer injects come before the page's own
// scripts, so a page previewed at a given second shows what the video will show.
func PreviewHTML(html string, duration float64) string { return previewWith(html, Shim(duration)) }

func previewWith(html, shimJS string) string {
	inject := `<meta http-equiv="Content-Security-Policy" content="` + previewCSP + `"><script>` +
		strings.ReplaceAll(shimJS, "</script", "<\\/script") + previewScript + `</script>`
	if loc := headOpen.FindStringIndex(html); loc != nil {
		return html[:loc[1]] + inject + html[loc[1]:]
	}
	if loc := htmlOpen.FindStringIndex(html); loc != nil {
		return html[:loc[1]] + "<head>" + inject + "</head>" + html[loc[1]:]
	}
	return "<!doctype html><html><head>" + inject + "</head><body>" + html + "</body></html>"
}
