package settings

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// DefaultDiffRhythm2Port is the port of the DiffRhythm2 service (a .NET service).
const DefaultDiffRhythm2Port = "8788"

// DiffRhythm2ConsentID is the key under which the user's acceptance of sending lyrics and style to an external
// DiffRhythm2 service is kept (see Consent).
const DiffRhythm2ConsentID = "diffrhythm2"

// ValidateDiffRhythm2Address checks the address of an external DiffRhythm2 service and returns it as
// scheme://host:port; the port is 8787 when none is given.
func ValidateDiffRhythm2Address(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("the address is empty")
	}
	probe := s
	if !strings.Contains(probe, "://") {
		probe = "http://" + probe
	}
	if u, err := url.Parse(probe); err == nil && u.User == nil && u.Hostname() != "" && u.Port() == "" && (u.Path == "" || u.Path == "/") {
		// No port: ValidateAddress would add Ollama's; this service has its own.
		s = u.Scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), DefaultDiffRhythm2Port)
	}
	return ValidateAddress(s)
}

// DiffRhythm2 returns the address of an external DiffRhythm2 service and its token: the saved ones, else the ones of the environment (DIFFRHYTHM2_HOST and
// DIFFRHYTHM2_TOKEN, see env.go), else empty. The token goes with its address: a saved address never takes the token of the environment.
func DiffRhythm2() (host, token string) {
	f := Load()
	if h := strings.TrimSpace(f.DiffRhythm2Host); h != "" {
		return h, f.DiffRhythm2Token
	}
	return envService("DIFFRHYTHM2", ValidateDiffRhythm2Address)
}

// DiffRhythm2Source says where DiffRhythm2() gets the address from: FromSetting, FromEnv or FromDefault (none).
func DiffRhythm2Source() string {
	if strings.TrimSpace(Load().DiffRhythm2Host) != "" {
		return FromSetting
	}
	if h, _ := envService("DIFFRHYTHM2", ValidateDiffRhythm2Address); h != "" {
		return FromEnv
	}
	return FromDefault
}

// SetDiffRhythm2 saves the address (already validated) and the token. An empty address removes both and the
// acceptance given for it; a changed address loses the acceptance too, so each address is accepted on its own.
func SetDiffRhythm2(host, token string) error {
	mu.Lock()
	defer mu.Unlock()
	f, rerr := ReadFile() // a source that cannot be read is not "no settings": saving would erase them
	if rerr != nil {
		return rerr
	}
	if host != f.DiffRhythm2Host {
		delete(f.Consent, DiffRhythm2ConsentID)
		if len(f.Consent) == 0 {
			f.Consent = nil
		}
	}
	if host == "" {
		token = ""
	}
	f.DiffRhythm2Host, f.DiffRhythm2Token = host, token
	return Save(f)
}

// AcknowledgeDiffRhythm2 records that the user accepts sending lyrics and style to the saved address.
func AcknowledgeDiffRhythm2() error {
	mu.Lock()
	defer mu.Unlock()
	f, rerr := ReadFile() // a source that cannot be read is not "no settings": saving would erase them
	if rerr != nil {
		return rerr
	}
	if f.DiffRhythm2Host == "" {
		return errors.New("no DiffRhythm2 address is saved")
	}
	if f.Consent == nil {
		f.Consent = map[string]string{}
	}
	f.Consent[DiffRhythm2ConsentID] = f.DiffRhythm2Host
	return Save(f)
}

// DiffRhythm2Acknowledged reports whether the user accepted sending data to this address.
func DiffRhythm2Acknowledged(address string) bool {
	if address == "" {
		return false
	}
	// an address the person wrote in the environment of the stack is the person's own choice: it is accepted without a click
	if DiffRhythm2Source() == FromEnv {
		if h, _ := DiffRhythm2(); h == address {
			return true
		}
	}
	return Load().Consent[DiffRhythm2ConsentID] == address
}
