package discover

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/providers"
)

// Result is what discovery found for an address.
type Result struct {
	Kind   api.AccountKind
	Source api.DiscoverySource
	Auth   []api.AuthKind
	Settings
}

// Deps are discovery's ways out; zero fields use the real ones.
type Deps struct {
	// Get fetches an HTTPS URL (fetch: 64 KB, 10 s, redirects only within
	// the host).
	Get func(ctx context.Context, rawURL string) ([]byte, error)
	// Resolver answers SRV queries (net.DefaultResolver).
	Resolver Resolver
	// ISPDB is the base URL of Thunderbird's ISP database.
	ISPDB string
}

const defaultISPDB = "https://autoconfig.thunderbird.net/v1.1/"

// Discover finds an address's servers (docs/design/accounts.md, Discovery):
// a provider profile; the domain's autoconfig documents; the ISPDB; SRV
// records. The first source with both servers wins; otherwise the first
// with either; otherwise the result's Source is none.
func Discover(ctx context.Context, email string, d Deps) (Result, error) {
	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return Result{}, fmt.Errorf("discover: not an address: %q", email)
	}
	if p, ok := providers.ForAddress(email); ok {
		return Result{
			Kind: p.Kind, Source: api.DiscoverySourceProfile, Auth: p.Auth,
			Settings: Settings{
				IMAP: &Server{Host: p.IMAP.Host, Port: p.IMAP.Port, TLS: p.IMAP.TLS, Username: email},
				SMTP: &Server{Host: p.SMTP.Host, Port: p.SMTP.Port, TLS: p.SMTP.TLS, Username: email},
			},
		}, nil
	}
	if d.Get == nil {
		d.Get = fetch
	}
	if d.Resolver == nil {
		d.Resolver = net.DefaultResolver
	}
	if d.ISPDB == "" {
		d.ISPDB = defaultISPDB
	}
	domain := strings.ToLower(email[at+1:])
	q := url.Values{"emailaddress": {email}}.Encode()
	type source struct {
		name api.DiscoverySource
		find func() (Settings, error)
	}
	fromURL := func(u string) func() (Settings, error) {
		return func() (Settings, error) {
			doc, err := d.Get(ctx, u)
			if err != nil {
				return Settings{}, err
			}
			return ParseAutoconfig(doc, email)
		}
	}
	sources := []source{
		{api.DiscoverySourceAutoconfig, fromURL("https://autoconfig." + domain + "/mail/config-v1.1.xml?" + q)},
		{api.DiscoverySourceAutoconfig, fromURL("https://" + domain + "/.well-known/autoconfig/mail/config-v1.1.xml?" + q)},
		{api.DiscoverySourceIspdb, fromURL(d.ISPDB + url.PathEscape(domain))},
		{api.DiscoverySourceSrv, func() (Settings, error) { return FromSRV(ctx, d.Resolver, email) }},
	}
	best := Result{Kind: api.AccountKindIMAP, Source: api.DiscoverySourceNone, Auth: []api.AuthKind{api.AuthKindPassword}}
	for _, s := range sources {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		set, err := s.find()
		if err != nil {
			continue // unreachable, missing, malformed: try the next source
		}
		if set.IMAP != nil && set.SMTP != nil {
			best.Source, best.Settings = s.name, set
			return best, nil
		}
		if best.Source == api.DiscoverySourceNone && (set.IMAP != nil || set.SMTP != nil) {
			best.Source, best.Settings = s.name, set
		}
	}
	return best, nil
}

// errFetch marks a fetch that did not give a document.
var errFetch = errors.New("discover: fetch failed")

// fetch gets an HTTPS document of at most 64 KB within 10 s, following
// redirects only within the same host.
func fetch(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return nil, fmt.Errorf("%w: %s is not https", errFetch, rawURL)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || req.URL.Scheme != "https" || req.URL.Host != u.Host {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFetch, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s: HTTP %d", errFetch, rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFetch, err)
	}
	if len(body) > 64<<10 {
		return nil, fmt.Errorf("%w: %s is larger than 64 KB", errFetch, rawURL)
	}
	return body, nil
}
