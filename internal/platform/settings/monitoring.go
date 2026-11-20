package settings

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// The external tools the server knows the address of. None of them is needed: they are looked at when the user asks
// (a test, a link) and the server never depends on them being up.
const (
	MonGrafana    = "grafana"
	MonPrometheus = "prometheus"
	MonLoki       = "loki"
	MonRedis      = "redis"
)

// MonitoringIDs are the tools, in the order the screen lists them.
var MonitoringIDs = []string{MonGrafana, MonPrometheus, MonLoki, MonRedis}

// Endpoint is where one of them is and how to get in. For the HTTP tools a User makes the calls go with Basic
// authentication; with no User the Secret is a token sent as a Bearer. For Redis the Secret is the password (and
// the User its ACL user, if it has one).
type Endpoint struct {
	Address string `json:"address,omitempty"`
	User    string `json:"user,omitempty"`
	Secret  string `json:"secret,omitempty"`
}

// DefaultMonitoringPort is the usual port of the tool.
func DefaultMonitoringPort(id string) int {
	switch id {
	case MonGrafana:
		return 3000
	case MonPrometheus:
		return 9090
	case MonLoki:
		return 3100
	case MonRedis:
		return 6379
	}
	return 0
}

// ValidMonitoringID reports whether id is one of the tools.
func ValidMonitoringID(id string) bool {
	for _, k := range MonitoringIDs {
		if k == id {
			return true
		}
	}
	return false
}

// ValidateMonitoringAddress checks what a user typed for a tool and returns it in the form it is kept: http://host:port
// (https allowed) for the web tools, host:port for Redis. The port is the tool's usual one when none is given.
func ValidateMonitoringAddress(id, raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("the address is empty")
	}
	if len(s) > maxAddressLen {
		return "", fmt.Errorf("the address is longer than %d characters", maxAddressLen)
	}
	def := strconv.Itoa(DefaultMonitoringPort(id))
	if id == MonRedis {
		if strings.Contains(s, "://") || strings.ContainsAny(s, "/@ ") {
			return "", errors.New("the address is a host or host:port, with no scheme, user or path")
		}
		h, p, err := net.SplitHostPort(s)
		if err != nil {
			h, p = strings.Trim(s, "[]"), def
		}
		if h == "" {
			return "", errors.New("the host is empty")
		}
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return "", errors.New("the port is not valid")
		}
		return net.JoinHostPort(h, p), nil
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return "", errors.New("the address is not valid")
	}
	if sch := strings.ToLower(u.Scheme); sch != "http" && sch != "https" {
		return "", fmt.Errorf("only http and https addresses are accepted, not %q", u.Scheme)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("the address has no user, query or fragment: the user and the key go in their own fields")
	}
	port := u.Port()
	if port == "" {
		port = def
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", errors.New("the port is not valid")
	}
	path := strings.TrimRight(u.Path, "/") // Grafana behind a sub path keeps it
	return strings.ToLower(u.Scheme) + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port) + path, nil
}

// SavedMonitoring returns what is saved for a tool (empty when nothing is).
func SavedMonitoring(id string) Endpoint { return Load().Monitoring[id] }

// GetMonitoring returns what is in use for a tool: what is saved, else what the environment describes (MONITORING_<TOOL>_ADDRESS, _USER and _SECRET,
// see env.go), else nothing.
func GetMonitoring(id string) Endpoint {
	if e := SavedMonitoring(id); e.Address != "" {
		return e
	}
	return envMonitoring(id)
}

// MonitoringSource says where GetMonitoring gets a tool from: FromSetting, FromEnv or FromDefault (nothing).
func MonitoringSource(id string) string {
	switch {
	case SavedMonitoring(id).Address != "":
		return FromSetting
	case envMonitoring(id).Address != "":
		return FromEnv
	}
	return FromDefault
}

// SetMonitoring saves a tool. An empty address removes it. A Secret left empty keeps the saved one when the
// address and the user are the same, so a form that never shows it can be saved again.
func SetMonitoring(id string, e Endpoint) error {
	if !ValidMonitoringID(id) {
		return fmt.Errorf("unknown tool %q", id)
	}
	mu.Lock()
	defer mu.Unlock()
	f, rerr := ReadFile() // a source that cannot be read is not "no settings": saving would erase them
	if rerr != nil {
		return rerr
	}
	if e.Address == "" {
		delete(f.Monitoring, id)
	} else {
		old := f.Monitoring[id]
		if e.Secret == "" && old.Address == e.Address && old.User == e.User {
			e.Secret = old.Secret
		}
		if f.Monitoring == nil {
			f.Monitoring = map[string]Endpoint{}
		}
		f.Monitoring[id] = e
	}
	if len(f.Monitoring) == 0 {
		f.Monitoring = nil
	}
	return Save(f)
}
