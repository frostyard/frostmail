// Package unsubscribe works out how a list message can be left and sends
// RFC 8058 one-click requests (ADR-0027): an HTTPS POST that carries
// nothing of the user's and never reaches the user's own network.
package unsubscribe

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/frostyard/lodge/kit/netguard"
)

// Method is a way to unsubscribe, best first: OneClick, Mail, Web.
type Method string

const (
	OneClick Method = "oneclick" // an HTTPS POST (RFC 8058)
	Mail     Method = "mail"     // a message to the mailto: address
	Web      Method = "web"      // the https: page, in the user's browser
)

// Info is what a message's headers offer.
type Info struct {
	Methods []Method // best first; none when the message offers nothing
	URL     string   // the https: URI, for OneClick and Web
	Host    string   // URL's host
	Address string   // the mailto: address
	Subject string   // the mailto: URI's subject; "unsubscribe" when it has none
	Body    string   // the mailto: URI's body
}

var (
	uriRE  = regexp.MustCompile(`<([^>]*)>`)
	dkimRE = regexp.MustCompile(`(?i)(^|[\s;])dkim\s*=\s*pass([\s;(]|$)`)
)

// Parse reads a message's List-Unsubscribe, List-Unsubscribe-Post and first
// Authentication-Results headers. One-click needs an https: URI, the
// One-Click POST header, and the receiving provider's dkim=pass.
func Parse(listUnsubscribe, post, authResults string) Info {
	var info Info
	for _, m := range uriRE.FindAllStringSubmatch(listUnsubscribe, -1) {
		u, err := url.Parse(strings.TrimSpace(m[1]))
		if err != nil {
			continue
		}
		switch strings.ToLower(u.Scheme) {
		case "https":
			if info.URL == "" && u.Host != "" {
				info.URL, info.Host = u.String(), u.Hostname()
			}
		case "mailto":
			addr := u.Opaque
			if addr == "" {
				addr = u.Path
			}
			if info.Address == "" && strings.Contains(addr, "@") {
				if a, err := url.PathUnescape(addr); err == nil {
					addr = a
				}
				info.Address = addr
				q := u.Query()
				info.Subject, info.Body = q.Get("subject"), q.Get("body")
				if info.Subject == "" {
					info.Subject = "unsubscribe"
				}
			}
		}
	}
	oneClick := strings.EqualFold(strings.TrimSpace(post), "List-Unsubscribe=One-Click")
	if info.URL != "" && oneClick && dkimRE.MatchString(authResults) {
		info.Methods = append(info.Methods, OneClick)
	}
	if info.Address != "" {
		info.Methods = append(info.Methods, Mail)
	}
	if info.URL != "" {
		info.Methods = append(info.Methods, Web)
	}
	return info
}

// limit bounds a one-click request, connection to response.
const limit = 10 * time.Second

// Poster sends one-click requests.
type Poster struct {
	client *http.Client
}

// New is a Poster that refuses every address inside the user's network.
// The check is lodge kit's netguard (ADR-0017 there), shared with the other
// apps: it refuses loopback, private, link-local, multicast, unspecified,
// shared (RFC 6598), the IETF protocol block, benchmarking, the reserved
// block, the documentation ranges, NAT64, Teredo, 6to4 and site-local, and
// it runs on each address after DNS.
func New() *Poster { return newPoster(netguard.Control, nil) }

// newPoster checks each address the dialer is about to connect to, after
// DNS, so a name that resolves inside (or rebinds) is refused. Production
// passes netguard.Control; a test passes controlWith to allow its local
// server.
func newPoster(control func(network, address string, c syscall.RawConn) error, tlsConfig *tls.Config) *Poster {
	dialer := &net.Dialer{
		Timeout: limit,
		Control: control,
	}
	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		TLSClientConfig:     tlsConfig,
		TLSHandshakeTimeout: limit,
		DisableKeepAlives:   true,
		ForceAttemptHTTP2:   true,
		// No proxy: an environment proxy would dial the address itself, and
		// this process would never see it.
		Proxy: nil,
	}
	return &Poster{client: &http.Client{
		Transport: transport,
		Timeout:   limit,
		// A redirect is not followed: its status is the answer.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// controlWith is netguard.Control's host:port check with another guard:
// netguard exports no hook, and the tests allow the local test server's
// address.
func controlWith(guard func(netip.Addr) error) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("unsubscribe: %q is not host:port: %w", address, err)
		}
		ip, err := netip.ParseAddr(host)
		if err != nil {
			return fmt.Errorf("unsubscribe: %q is not an address: %w", host, err)
		}
		return guard(ip)
	}
}

// Post sends the one-click request: a POST of List-Unsubscribe=One-Click,
// with a fixed User-Agent and nothing else of the user's. A 2xx status is
// success.
func (p *Poster) Post(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" {
		return fmt.Errorf("unsubscribe: %q is not an https: URI", rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader("List-Unsubscribe=One-Click"))
	if err != nil {
		return fmt.Errorf("unsubscribe: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Frostmail")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("unsubscribe: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("unsubscribe: %s answered %s", u.Host, resp.Status)
	}
	return nil
}
