// Package conn connects the desktop app to maild: on the local socket or
// named pipe, or through lodge's /app/mail/api WebSocket
// (docs/design/desktop.md, On a maild behind lodge), and keeps the
// connection: it reconnects every second and pings lodge, which closes a
// session after 90 s without a frame. It imports no UI.
package conn

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/frostyard/frostmail/internal/config"
)

// Lodge is lodge's address: https on the tailnet, or http on loopback for
// lodge's development build.
type Lodge struct {
	origin string
	secure bool
}

// ParseLodge parses lodge's address: a scheme and a host, with an optional
// port and no path, query, fragment or credentials. Plain http is for a
// loopback address only.
func ParseLodge(s string) (Lodge, error) {
	s = strings.TrimSpace(s)
	u, err := url.Parse(s)
	if err != nil {
		return Lodge{}, fmt.Errorf("lodge's address %q: %w", s, err)
	}
	if u.Host == "" || u.Hostname() == "" {
		return Lodge{}, fmt.Errorf("lodge's address %q has no host", s)
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return Lodge{}, fmt.Errorf("lodge's address %q must be a scheme and a host", s)
	}
	if u.Path != "" && u.Path != "/" {
		return Lodge{}, fmt.Errorf("lodge's address %q must have no path", s)
	}
	var secure bool
	switch u.Scheme {
	case "https":
		secure = true
	case "http":
		if !loopback(u.Hostname()) {
			return Lodge{}, fmt.Errorf("lodge's address %q must be https off this machine", s)
		}
	default:
		return Lodge{}, fmt.Errorf("lodge's address %q must be https, not %s", s, u.Scheme)
	}
	return Lodge{origin: u.Scheme + "://" + strings.ToLower(u.Host), secure: secure}, nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Origin is lodge's origin, which the app sends as its Origin (lodge's
// decision D7).
func (l Lodge) Origin() string { return l.origin }

// String is the origin.
func (l Lodge) String() string { return l.origin }

// APIURL is the mail app's /api WebSocket.
func (l Lodge) APIURL() string {
	_, rest, _ := strings.Cut(l.origin, "://")
	scheme := "ws"
	if l.secure {
		scheme = "wss"
	}
	return scheme + "://" + rest + "/app/mail/api"
}

// DerivedContentOrigin is the content node's usual name: lodge's, with
// lodge-content for its first label. On loopback there is none.
func (l Lodge) DerivedContentOrigin() (string, error) {
	scheme, rest, _ := strings.Cut(l.origin, "://")
	_, domain, ok := strings.Cut(rest, ".")
	if !ok || !l.secure {
		return "", fmt.Errorf("cannot tell %s's content origin", l.origin)
	}
	return scheme + "://lodge-content." + domain, nil
}

// AppConfig is the app's own app.json: which maild it asks. It is the
// app's, never maild's; maild only reads its mode (config.ReadMode).
type AppConfig struct {
	// Lodge is lodge's address; empty means the local maild.
	Lodge string `json:"lodge,omitzero"`
}

// ReadAppConfig reads app.json at path; a missing file is the local maild.
// A file that is not valid JSON, or names an invalid lodge, is an error.
func ReadAppConfig(path string) (AppConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return AppConfig{}, nil
	}
	if err != nil {
		return AppConfig{}, err
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return AppConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Lodge != "" {
		if _, err := ParseLodge(cfg.Lodge); err != nil {
			return AppConfig{}, fmt.Errorf("%s: %w", path, err)
		}
	}
	return cfg, nil
}

// WriteAppConfig writes app.json at path atomically, readable by the user
// only.
func WriteAppConfig(path string, cfg AppConfig) error {
	if cfg.Lodge != "" {
		l, err := ParseLodge(cfg.Lodge)
		if err != nil {
			return err
		}
		cfg.Lodge = l.Origin()
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".app.json-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Target says which maild to dial: lodge's when Lodge is set, else the
// local socket or pipe.
type Target struct {
	Socket string
	Lodge  *Lodge
}

// Remote reports whether the target is lodge's maild.
func (t Target) Remote() bool { return t.Lodge != nil }

// Host is where the daemon runs, for the status bar: lodge's host, "" for
// this computer.
func (t Target) Host() string {
	if t.Lodge == nil {
		return ""
	}
	_, host, _ := strings.Cut(t.Lodge.origin, "://")
	return host
}

// String names the target as the status bar shows it.
func (t Target) String() string {
	if t.Lodge != nil {
		_, host, _ := strings.Cut(t.Lodge.Origin(), "://")
		return host
	}
	return "Local"
}

// TargetFrom reads the target from the environment and app.json:
// FROSTMAIL_SOCKET or the default socket, unless app.json names lodge.
func TargetFrom(getenv func(string) string) (Target, error) {
	path, err := config.AppConfigFile(getenv)
	if err != nil {
		return Target{}, err
	}
	cfg, err := ReadAppConfig(path)
	if err != nil {
		return Target{}, err
	}
	if cfg.Lodge != "" {
		l, err := ParseLodge(cfg.Lodge)
		if err != nil {
			return Target{}, err
		}
		return Target{Lodge: &l}, nil
	}
	socket := getenv("FROSTMAIL_SOCKET")
	if socket == "" {
		if socket, err = config.DefaultSocket(getenv); err != nil {
			return Target{}, err
		}
	}
	return Target{Socket: socket}, nil
}
