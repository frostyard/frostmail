package discover

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func doc(imapHost, smtpHost string) string {
	var b strings.Builder
	b.WriteString(`<clientConfig><emailProvider>`)
	if imapHost != "" {
		b.WriteString(`<incomingServer type="imap"><hostname>` + imapHost + `</hostname><port>993</port><socketType>SSL</socketType><username>%EMAILADDRESS%</username><authentication>password-cleartext</authentication></incomingServer>`)
	}
	if smtpHost != "" {
		b.WriteString(`<outgoingServer type="smtp"><hostname>` + smtpHost + `</hostname><port>465</port><socketType>SSL</socketType><username>%EMAILADDRESS%</username><authentication>password-cleartext</authentication></outgoingServer>`)
	}
	b.WriteString(`</emailProvider></clientConfig>`)
	return b.String()
}

func getter(docs map[string]string) func(context.Context, string) ([]byte, error) {
	return func(_ context.Context, u string) ([]byte, error) {
		for prefix, d := range docs {
			if strings.HasPrefix(u, prefix) {
				return []byte(d), nil
			}
		}
		return nil, errFetch
	}
}

type noSRV struct{}

func (noSRV) LookupSRV(_ context.Context, service, proto, name string) (string, []*net.SRV, error) {
	return "", nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func TestDiscoverProfiles(t *testing.T) {
	r, err := Discover(t.Context(), "ann@gmail.com", Deps{Get: getter(nil), Resolver: noSRV{}})
	if err != nil || r.Kind != api.AccountKindGmail || r.Source != api.DiscoverySourceProfile ||
		r.IMAP.Host != "imap.gmail.com" || r.SMTP.Username != "ann@gmail.com" || r.Auth[0] != api.AuthKindOAuth2 {
		t.Fatalf("gmail = %+v, %v", r, err)
	}
}

func TestDiscoverOrder(t *testing.T) {
	cases := []struct {
		name string
		docs map[string]string
		want api.DiscoverySource
		host string
	}{
		{"autoconfig host", map[string]string{"https://autoconfig.x.test/": doc("a.x.test", "s.x.test"), "https://isp.test/": doc("i.x.test", "s.x.test")}, api.DiscoverySourceAutoconfig, "a.x.test"},
		{"well-known", map[string]string{"https://x.test/.well-known/": doc("w.x.test", "s.x.test")}, api.DiscoverySourceAutoconfig, "w.x.test"},
		{"ispdb", map[string]string{"https://isp.test/x.test": doc("i.x.test", "s.x.test")}, api.DiscoverySourceIspdb, "i.x.test"},
		{"complete beats partial", map[string]string{"https://autoconfig.x.test/": doc("a.x.test", ""), "https://isp.test/x.test": doc("i.x.test", "s.x.test")}, api.DiscoverySourceIspdb, "i.x.test"},
		{"partial when nothing is complete", map[string]string{"https://autoconfig.x.test/": doc("a.x.test", "")}, api.DiscoverySourceAutoconfig, "a.x.test"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := Discover(t.Context(), "ann@X.test", Deps{Get: getter(c.docs), Resolver: noSRV{}, ISPDB: "https://isp.test/"})
			if err != nil || r.Source != c.want || r.IMAP == nil || r.IMAP.Host != c.host || r.Kind != api.AccountKindIMAP {
				t.Fatalf("= %+v (%+v), %v", r, r.IMAP, err)
			}
		})
	}
}

type oneSRV struct{}

func (oneSRV) LookupSRV(_ context.Context, service, _, name string) (string, []*net.SRV, error) {
	if service == "imaps" {
		return "", []*net.SRV{{Target: "srv.x.test.", Port: 993}}, nil
	}
	if service == "submissions" {
		return "", []*net.SRV{{Target: "smtp.x.test.", Port: 465}}, nil
	}
	return "", nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func TestDiscoverSRVAndNothing(t *testing.T) {
	r, err := Discover(t.Context(), "ann@x.test", Deps{Get: getter(nil), Resolver: oneSRV{}, ISPDB: "https://isp.test/"})
	if err != nil || r.Source != api.DiscoverySourceSrv || r.IMAP.Host != "srv.x.test" {
		t.Fatalf("srv = %+v, %v", r, err)
	}
	r, err = Discover(t.Context(), "ann@x.test", Deps{Get: getter(nil), Resolver: noSRV{}, ISPDB: "https://isp.test/"})
	if err != nil || r.Source != api.DiscoverySourceNone || r.IMAP != nil || r.SMTP != nil {
		t.Fatalf("nothing = %+v, %v", r, err)
	}
	if _, err := Discover(t.Context(), "nobody", Deps{}); err == nil {
		t.Error("an address without @ was accepted")
	}
}

func TestFetchRefusesPlainHTTP(t *testing.T) {
	if _, err := fetch(t.Context(), "http://x.test/config.xml"); !errors.Is(err, errFetch) {
		t.Errorf("http = %v", err)
	}
}
