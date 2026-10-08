package discover

// CONTRACT TEST for task card T-0050 (docs/tasks). Do not edit.

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/frostyard/frostmail/api"
)

// fakeDNS answers SRV queries from a map keyed by "_service._proto.name".
type fakeDNS struct {
	records map[string][]*net.SRV
	asked   []string
}

func (f *fakeDNS) LookupSRV(_ context.Context, service, proto, name string) (string, []*net.SRV, error) {
	key := "_" + service + "._" + proto + "." + name
	f.asked = append(f.asked, key)
	recs, ok := f.records[key]
	if !ok {
		return "", nil, &net.DNSError{Err: "no such host", Name: key, IsNotFound: true}
	}
	return key, recs, nil
}

func TestFromSRVPrefersImplicitTLS(t *testing.T) {
	dns := &fakeDNS{records: map[string][]*net.SRV{
		"_imaps._tcp.x.test":       {{Target: "imap.x.test.", Port: 993, Priority: 0, Weight: 1}},
		"_imap._tcp.x.test":        {{Target: "imap.x.test.", Port: 143, Priority: 0, Weight: 1}},
		"_submissions._tcp.x.test": {{Target: "smtp.x.test.", Port: 465, Priority: 0, Weight: 1}},
		"_submission._tcp.x.test":  {{Target: "smtp.x.test.", Port: 587, Priority: 0, Weight: 1}},
	}}
	got, err := FromSRV(context.Background(), dns, "Ann@X.Test")
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{
		IMAP: &Server{Host: "imap.x.test", Port: 993, TLS: api.TLSModeTLS, Username: "Ann@X.Test"},
		SMTP: &Server{Host: "smtp.x.test", Port: 465, TLS: api.TLSModeTLS, Username: "Ann@X.Test"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v / %+v", got.IMAP, got.SMTP)
	}
	if dns.asked[0] != "_imaps._tcp.x.test" {
		t.Errorf("queried %v; the domain must be lowercased and imaps asked first", dns.asked)
	}
}

func TestFromSRVFallsBackAndPicksByPriorityAndWeight(t *testing.T) {
	dns := &fakeDNS{records: map[string][]*net.SRV{
		"_imaps._tcp.x.test": {{Target: ".", Port: 0}},
		"_imap._tcp.x.test": {
			{Target: "backup.x.test.", Port: 143, Priority: 20, Weight: 100},
			{Target: "light.x.test.", Port: 143, Priority: 10, Weight: 1},
			{Target: "heavy.x.test.", Port: 1143, Priority: 10, Weight: 50},
		},
		"_submission._tcp.x.test": {{Target: "smtp.x.test", Port: 587, Priority: 0, Weight: 0}},
	}}
	got, err := FromSRV(context.Background(), dns, "a@x.test")
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{
		IMAP: &Server{Host: "heavy.x.test", Port: 1143, TLS: api.TLSModeStartTLS, Username: "a@x.test"},
		SMTP: &Server{Host: "smtp.x.test", Port: 587, TLS: api.TLSModeStartTLS, Username: "a@x.test"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v / %+v", got.IMAP, got.SMTP)
	}
}

func TestFromSRVNothing(t *testing.T) {
	dns := &fakeDNS{records: map[string][]*net.SRV{
		"_imaps._tcp.x.test":      {{Target: ".", Port: 0}},
		"_submission._tcp.x.test": {{Target: "", Port: 587}},
	}}
	if _, err := FromSRV(context.Background(), dns, "a@x.test"); !errors.Is(err, ErrNothing) {
		t.Errorf("err = %v, want ErrNothing", err)
	}
	if _, err := FromSRV(context.Background(), &fakeDNS{}, "nobody"); err == nil || errors.Is(err, ErrNothing) {
		t.Errorf("an address without @: err = %v", err)
	}
}

type failingDNS struct{}

func (failingDNS) LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error) {
	return "", nil, &net.DNSError{Err: "server misbehaving", Name: "x", IsTemporary: true}
}

func TestFromSRVReportsDNSFailures(t *testing.T) {
	_, err := FromSRV(context.Background(), failingDNS{}, "a@x.test")
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		t.Errorf("err = %v, want the DNS error", err)
	}
}
