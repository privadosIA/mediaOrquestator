package settings

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
)

// ReadFile reads the settings like Load, but says when the source exists and cannot be read, so a reader that must not forget what it knew (the
// CORS lists) can keep the last good copy. A missing file is no settings, without an error.
func ReadFile() (File, error) {
	s := currentSource()
	if s == nil {
		return fileSource{}.Load()
	}
	f, err := s.Load()
	if err != nil {
		return File{}, err
	}
	b, err := loadBootstrap()
	if err != nil {
		return File{}, err
	}
	f.Database, f.ServerName = b.Database, b.ServerName
	return f, nil
}

// NormalizeCORSOrigin checks an allowed origin (scheme://host[:port], http or https, no path) and returns it in the
// form the browser sends it: lower case, no trailing slash.
func NormalizeCORSOrigin(raw string) (string, error) {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" {
		return "", fmt.Errorf("%q is not an origin: use http://host or https://host, with the port if it has one (for example http://192.168.1.50:4200)", raw)
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("%q is not an origin: it must not have a path, query or user", raw)
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), nil
}

// NormalizeCORSNetwork checks a CIDR range (a single address is taken as its /32 or /128) and returns it in its
// canonical form.
func NormalizeCORSNetwork(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if ip := net.ParseIP(s); ip != nil {
		if ip.To4() != nil {
			s += "/32"
		} else {
			s += "/128"
		}
	}
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		return "", fmt.Errorf("%q is not a network: use CIDR notation such as 192.168.1.0/24", raw)
	}
	return n.String(), nil
}

// CORS returns the origins and networks that are allowed from the settings, as written, and the ones the environment adds (see EnvCORS), without repeating one.
func CORS() (origins, networks []string) {
	f := Load()
	eo, en := EnvCORS()
	return union(f.CORSOrigins, eo), union(f.CORSNetworks, en)
}

// union is a followed by the entries of b that are not in a.
func union(a, b []string) []string {
	out := append([]string(nil), a...)
	have := map[string]bool{}
	for _, v := range a {
		have[v] = true
	}
	for _, v := range b {
		if !have[v] {
			have[v] = true
			out = append(out, v)
		}
	}
	return out
}

// ChangeCORS adds or removes an origin or a network and saves the file. It returns the normalized value and whether
// the list changed. Nothing here is reachable from the API: the command line is the only caller.
func ChangeCORS(network, remove bool, raw string) (string, bool, error) {
	var value string
	var err error
	if network {
		value, err = NormalizeCORSNetwork(raw)
	} else {
		value, err = NormalizeCORSOrigin(raw)
	}
	if err != nil {
		return "", false, err
	}
	f, err := ReadFile()
	if err != nil {
		return "", false, fmt.Errorf("the settings file cannot be read, not changing it: %w", err)
	}
	list := &f.CORSOrigins
	if network {
		list = &f.CORSNetworks
	}
	var out []string
	found := false
	for _, v := range *list {
		if network {
			if n, e := NormalizeCORSNetwork(v); e == nil {
				v = n
			}
		} else if o, e := NormalizeCORSOrigin(v); e == nil {
			v = o
		}
		if v == value {
			found = true
			if remove {
				continue
			}
		}
		out = append(out, v)
	}
	changed := false
	switch {
	case remove && found:
		changed = true
	case !remove && !found:
		out = append(out, value)
		changed = true
	}
	sort.Strings(out)
	*list = out
	if !changed {
		return value, false, nil
	}
	return value, true, Save(f)
}
