package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// URLSourceTimeout bounds the whole download of a video from an address.
const URLSourceTimeout = 5 * time.Minute

var youtubeHosts = map[string]bool{
	"youtube.com": true, "www.youtube.com": true, "m.youtube.com": true,
	"music.youtube.com": true, "youtu.be": true,
}

var youtubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

const youtubeAccepted = "use the https address of one YouTube video (youtube.com/watch?v=…, youtu.be/…, youtube.com/shorts/…); playlists and other sites are not accepted"

// ParseYouTubeURL validates the address and returns it normalized. The host
// is matched exactly (never as a substring) and a playlist-only address, a
// user-info part or a port other than 443 is refused.
func ParseYouTubeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || !youtubeHosts[strings.ToLower(u.Hostname())] {
		return "", errors.New(youtubeAccepted)
	}
	if p := u.Port(); p != "" && p != "443" {
		return "", errors.New(youtubeAccepted)
	}
	host := strings.ToLower(u.Hostname())
	var id string
	if host == "youtu.be" {
		id = strings.Trim(u.Path, "/")
	} else {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		switch {
		case len(parts) == 1 && parts[0] == "watch":
			id = u.Query().Get("v")
		case len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "live" || parts[0] == "embed" || parts[0] == "v"):
			id = parts[1]
		}
	}
	if !youtubeID.MatchString(id) {
		return "", errors.New(youtubeAccepted)
	}
	return "https://www.youtube.com/watch?v=" + id, nil
}

// signInHints are fragments of yt-dlp's messages when a video needs an account.
var signInHints = []string{"sign in", "log in", "login", "private video", "members-only", "members only", "age-restricted", "confirm your age", "not a bot"}

// YtdlpFailure maps a yt-dlp error to the HTTP answer.
func YtdlpFailure(w http.ResponseWriter, err error) {
	msg := err.Error()
	low := strings.ToLower(msg)
	for _, h := range signInHints {
		if strings.Contains(low, h) {
			WriteError(w, http.StatusUnprocessableEntity, CodeValidation,
				"YouTube asks for a sign-in for this video (private, members only, age-restricted or blocked as automated); this tool does not use accounts or cookies. "+LastLines(msg, 2))
			return
		}
	}
	WriteError(w, http.StatusUnprocessableEntity, CodeValidation,
		"cannot get the audio of this video (unavailable, removed, or yt-dlp is out of date: run scripts/get-ytdlp.sh to update it): "+LastLines(msg, 3))
}

// LastLines returns the last n lines of s, for an error message that must not carry a whole log.
func LastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// Refuse writes a 422 for a request setting the option does not accept.
func Refuse(w http.ResponseWriter, format string, args ...any) {
	WriteError(w, http.StatusUnprocessableEntity, CodeValidation, fmt.Sprintf(format, args...))
}
