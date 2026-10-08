package discover

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// ParseAutoconfig reads a Mozilla autoconfig document (config-v1.1.xml) for
// email and returns the first usable IMAP and SMTP servers it offers, in
// document order across providers. A server is usable when its socketType is
// SSL or STARTTLS, it offers at least one password-* authentication, and
// its expanded host name and port are valid. Placeholders in hostname and
// username are expanded from email. It returns ErrNothing when neither
// server is found, and a plain error when email is not an address or the
// document does not decode.
func ParseAutoconfig(doc []byte, email string) (Settings, error) {
	local, domain, err := splitEmail(email)
	if err != nil {
		return Settings{}, err
	}
	var cfg clientConfig
	if err := xml.Unmarshal(doc, &cfg); err != nil {
		return Settings{}, err
	}
	exp := strings.NewReplacer("%EMAILADDRESS%", email, "%EMAILLOCALPART%", local, "%EMAILDOMAIN%", domain)
	var s Settings
	for _, p := range cfg.Providers {
		for i := range p.Incoming {
			if s.IMAP == nil && p.Incoming[i].Type == "imap" {
				s.IMAP = usableServer(&p.Incoming[i], exp)
			}
		}
		for i := range p.Outgoing {
			if s.SMTP == nil && p.Outgoing[i].Type == "smtp" {
				s.SMTP = usableServer(&p.Outgoing[i], exp)
			}
		}
	}
	if s.IMAP == nil && s.SMTP == nil {
		return Settings{}, ErrNothing
	}
	return s, nil
}

type clientConfig struct {
	XMLName   xml.Name   `xml:"clientConfig"`
	Providers []provider `xml:"emailProvider"`
}

type provider struct {
	ID       string       `xml:"id,attr"`
	Incoming []autoServer `xml:"incomingServer"`
	Outgoing []autoServer `xml:"outgoingServer"`
}

type autoServer struct {
	Type           string   `xml:"type,attr"`
	Hostname       string   `xml:"hostname"`
	Port           string   `xml:"port"`
	SocketType     string   `xml:"socketType"`
	Username       string   `xml:"username"`
	Authenticators []string `xml:"authentication"`
}

func usableServer(a *autoServer, exp *strings.Replacer) *Server {
	var tls api.TLSMode
	switch strings.ToLower(strings.TrimSpace(a.SocketType)) {
	case "ssl":
		tls = api.TLSModeTLS
	case "starttls":
		tls = api.TLSModeStartTLS
	default:
		return nil
	}
	if !hasPasswordAuth(a.Authenticators) {
		return nil
	}
	host := strings.ToLower(strings.TrimSpace(exp.Replace(a.Hostname)))
	if host == "" {
		return nil
	}
	port, err := strconv.Atoi(strings.TrimSpace(a.Port))
	if err != nil || port < 1 || port > 65535 {
		return nil
	}
	return &Server{Host: host, Port: port, TLS: tls, Username: strings.TrimSpace(exp.Replace(a.Username))}
}

func hasPasswordAuth(auths []string) bool {
	for _, a := range auths {
		if strings.HasPrefix(strings.TrimSpace(a), "password-") {
			return true
		}
	}
	return false
}

func splitEmail(email string) (local, domain string, err error) {
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "", "", fmt.Errorf("discover: %q is not an email address", email)
	}
	return email[:at], strings.ToLower(email[at+1:]), nil
}
