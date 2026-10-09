package discover

import (
	"net"
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestDAVStarts(t *testing.T) {
	dns := &fakeDNS{records: map[string][]*net.SRV{
		"_carddavs._tcp.example.com": {{Target: "carddav.example.net.", Port: 443}},
		"_caldavs._tcp.example.com":  {{Target: "dav.example.net.", Port: 8443}},
	}}
	ctx := t.Context()
	got, err := DAVStarts(ctx, dns, "ann@Example.com", api.ServiceKindContacts)
	if err != nil || !slices.Equal(got, []string{"https://carddav.example.net/", "https://example.com/"}) {
		t.Errorf("contacts = %q, %v", got, err)
	}
	got, err = DAVStarts(ctx, dns, "ann@example.com", api.ServiceKindCalendar)
	if err != nil || !slices.Equal(got, []string{"https://dav.example.net:8443/", "https://example.com/"}) {
		t.Errorf("calendar = %q, %v", got, err)
	}
	got, err = DAVStarts(ctx, dns, "ann@plain.example", api.ServiceKindTasks)
	if err != nil || !slices.Equal(got, []string{"https://plain.example/"}) {
		t.Errorf("no SRV = %q, %v", got, err)
	}
	if _, err := DAVStarts(ctx, dns, "nodomain", api.ServiceKindContacts); err == nil {
		t.Error("an address without a domain was accepted")
	}
	if _, err := DAVStarts(ctx, failingDNS{}, "ann@example.com", api.ServiceKindContacts); err == nil {
		t.Error("a DNS failure was not reported")
	}
}
