package settings

// ctxpress.go: the connection to the optional ctxpress compression service (see internal/assistant/ctxpress). The address, whether it is on and the
// shortest text worth compressing live in settings.json; the token lives in the 0600 key store (keys.json) under the id CtxpressKey, like the
// keys of the providers, and is never returned by any report. The environment (CTXPRESS_URL, CTXPRESS_TOKEN, CTXPRESS_MIN_CHARS) is the default
// when nothing is saved.

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CtxpressKey is the id the token of the service is kept under in the key store.
const CtxpressKey = "ctxpress"

// Limits of the shortest text worth compressing.
const (
	DefaultCtxpressMinChars = 2000
	MinCtxpressMinChars     = 100
	MaxCtxpressMinChars     = 1000000
)

// CtxpressSettings is the section `ctxpress` of settings.json.
type CtxpressSettings struct {
	// Enabled turns the connection on; with it off nothing is sent to the service even if an address is saved.
	Enabled bool `json:"enabled"`
	// URL is the address of the service (scheme://host:port); empty with Enabled true means "not configured".
	URL string `json:"url,omitempty"`
	// MinChars is the length from which a text is sent to be compressed; 0 means DefaultCtxpressMinChars.
	MinChars int `json:"min_chars,omitempty"`
}

// CtxpressConfig is the connection in use, with where it comes from.
type CtxpressConfig struct {
	// Enabled is true only when it is on and there is an address.
	Enabled  bool
	URL      string
	Token    string
	MinChars int
	// Source is "setting" (saved), "env" (the environment, nothing saved) or "none".
	Source string
	// Saved is what settings.json holds (nil when nothing is saved).
	Saved *CtxpressSettings
}

// Ctxpress resolves the connection: the saved settings first, else the environment. It reads the file on every call, so a change takes effect
// without a restart.
func Ctxpress() CtxpressConfig {
	c := CtxpressConfig{MinChars: DefaultCtxpressMinChars, Source: "none"}
	saved := Load().Ctxpress
	if saved != nil {
		cp := *saved
		c.Saved, c.Source = &cp, "setting"
		c.URL = strings.TrimRight(strings.TrimSpace(saved.URL), "/")
		c.Enabled = saved.Enabled && c.URL != ""
		if saved.MinChars > 0 {
			c.MinChars = saved.MinChars
		}
	} else if u := strings.TrimRight(strings.TrimSpace(os.Getenv("CTXPRESS_URL")), "/"); u != "" {
		c.Source, c.URL, c.Enabled = "env", u, true
		if v, err := strconv.Atoi(os.Getenv("CTXPRESS_MIN_CHARS")); err == nil && v > 0 {
			c.MinChars = v
		}
	}
	c.Token = Key(CtxpressKey)
	if c.Token == "" {
		c.Token, _ = Env("CTXPRESS_TOKEN") // or CTXPRESS_TOKEN_FILE
	}
	return c
}

// ValidateCtxpressAddress checks the address of the service like any other (http or https, a host, no user, path or query) and normalizes it.
func ValidateCtxpressAddress(raw string) (string, error) { return ValidateAddress(raw) }

// ValidateCtxpressMinChars checks the shortest text worth compressing.
func ValidateCtxpressMinChars(n int) error {
	if n < MinCtxpressMinChars || n > MaxCtxpressMinChars {
		return fmt.Errorf("min_chars must be from %d to %d", MinCtxpressMinChars, MaxCtxpressMinChars)
	}
	return nil
}

// CtxpressWarning says what is risky about an address: a token that would travel in clear text to another machine. Empty when nothing is.
func CtxpressWarning(url string) string {
	if url == "" || !Remote(url) {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(url), "http://") {
		return "the service is on another machine and the address is http: the token and every text sent to it travel in clear text; use https or a tunnel"
	}
	return ""
}

// SaveCtxpress stores the section ctxpress of settings.json; nil removes it (the environment is the default again).
func SaveCtxpress(c *CtxpressSettings) error {
	if c != nil && c.URL != "" {
		if _, err := ValidateCtxpressAddress(c.URL); err != nil {
			return err
		}
	}
	if c != nil && c.MinChars != 0 {
		if err := ValidateCtxpressMinChars(c.MinChars); err != nil {
			return err
		}
	}
	mu.Lock()
	defer mu.Unlock()
	f, rerr := ReadFile() // a source that cannot be read is not "no settings": saving would erase them
	if rerr != nil {
		return rerr
	}
	f.Ctxpress = c
	return Save(f)
}

// SetCtxpressToken stores the token of the service; an empty one removes it.
func SetCtxpressToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return DeleteKey(CtxpressKey)
	}
	if len(token) > maxKeyLen {
		return errors.New("the token is longer than 500 characters")
	}
	return SetKey(CtxpressKey, token)
}
