package settings

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// DefaultHeartMulaPort is the port of the HeartMuLa service (a .NET service).
const DefaultHeartMulaPort = "8787"

// HeartMulaConsentID is the key under which the user's acceptance of sending lyrics and tags to an external
// HeartMuLa service is kept (see Consent).
const HeartMulaConsentID = "heartmula"

// ValidateHeartMulaAddress checks the address of an external HeartMuLa service and returns it as
// scheme://host:port; the port is 8787 when none is given.
func ValidateHeartMulaAddress(raw string) (string, error) {
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
		s = u.Scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), DefaultHeartMulaPort)
	}
	return ValidateAddress(s)
}

// HeartMula returns the address of an external HeartMula service and its token: the saved ones, else the ones of the environment (HEARTMULA_HOST and
// HEARTMULA_TOKEN, see env.go), else empty. The token goes with its address: a saved address never takes the token of the environment.
func HeartMula() (host, token string) {
	f := Load()
	if h := strings.TrimSpace(f.HeartMulaHost); h != "" {
		return h, f.HeartMulaToken
	}
	return envService("HEARTMULA", ValidateHeartMulaAddress)
}

// HeartMulaSource says where HeartMula() gets the address from: FromSetting, FromEnv or FromDefault (none).
func HeartMulaSource() string {
	if strings.TrimSpace(Load().HeartMulaHost) != "" {
		return FromSetting
	}
	if h, _ := envService("HEARTMULA", ValidateHeartMulaAddress); h != "" {
		return FromEnv
	}
	return FromDefault
}

// SetHeartMula saves the address (already validated) and the token. An empty address removes both and the
// acceptance given for it; a changed address loses the acceptance too, so each address is accepted on its own.
func SetHeartMula(host, token string) error {
	mu.Lock()
	defer mu.Unlock()
	f, rerr := ReadFile() // a source that cannot be read is not "no settings": saving would erase them
	if rerr != nil {
		return rerr
	}
	if host != f.HeartMulaHost {
		delete(f.Consent, HeartMulaConsentID)
		if len(f.Consent) == 0 {
			f.Consent = nil
		}
	}
	if host == "" {
		token = ""
	}
	f.HeartMulaHost, f.HeartMulaToken = host, token
	return Save(f)
}

// AcknowledgeHeartMula records that the user accepts sending lyrics and tags to the saved address.
func AcknowledgeHeartMula() error {
	mu.Lock()
	defer mu.Unlock()
	f, rerr := ReadFile() // a source that cannot be read is not "no settings": saving would erase them
	if rerr != nil {
		return rerr
	}
	if f.HeartMulaHost == "" {
		return errors.New("no HeartMuLa address is saved")
	}
	if f.Consent == nil {
		f.Consent = map[string]string{}
	}
	f.Consent[HeartMulaConsentID] = f.HeartMulaHost
	return Save(f)
}

// HeartMulaAcknowledged reports whether the user accepted sending data to this address.
func HeartMulaAcknowledged(address string) bool {
	if address == "" {
		return false
	}
	// an address the person wrote in the environment of the stack is the person's own choice: it is accepted without a click
	if HeartMulaSource() == FromEnv {
		if h, _ := HeartMula(); h == address {
			return true
		}
	}
	return Load().Consent[HeartMulaConsentID] == address
}
