package providers

import (
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestForAddress(t *testing.T) {
	cases := map[string]api.AccountKind{
		"ann@gmail.com":      api.AccountKindGmail,
		"Ann@GoogleMail.com": api.AccountKindGmail,
		"bob@icloud.com":     api.AccountKindICloud,
		"bob@me.com":         api.AccountKindICloud,
		"bob@mac.com":        api.AccountKindICloud,
		"cy@hotmail.com":     api.AccountKindMicrosoft,
		"cy@Outlook.com":     api.AccountKindMicrosoft,
		"cy@live.co.uk":      api.AccountKindMicrosoft,
	}
	for addr, kind := range cases {
		p, ok := ForAddress(addr)
		if !ok || p.Kind != kind {
			t.Errorf("ForAddress(%q) = %v, %v; want %v", addr, p.Kind, ok, kind)
		}
	}
	for _, addr := range []string{"ann@fastmail.com", "nobody", "x@gmail.com.evil.test"} {
		if _, ok := ForAddress(addr); ok {
			t.Errorf("ForAddress(%q) found a profile", addr)
		}
	}
}

func TestForKind(t *testing.T) {
	g := ForKind(api.AccountKindGmail)
	if !g.Quirks.Gmail || !g.Quirks.SavesSent || g.OAuth != "google" || g.IMAP.Host != "imap.gmail.com" {
		t.Errorf("gmail profile = %+v", g)
	}
	if i := ForKind(api.AccountKindICloud); !i.Quirks.SilentEnable || i.SMTP.Port != 587 {
		t.Errorf("icloud profile = %+v", i)
	}
	m := ForKind(api.AccountKindMicrosoft)
	if m.OAuth != "microsoft" || !m.Quirks.SavesSent || !m.Quirks.NoKeywords || m.Quirks.Gmail ||
		m.Quirks.OriginalMessageID != "X-Microsoft-Original-Message-ID" ||
		m.IMAP != (Server{Host: "outlook.office365.com", Port: 993, TLS: api.TLSModeTLS}) ||
		m.SMTP != (Server{Host: "smtp-mail.outlook.com", Port: 587, TLS: api.TLSModeStartTLS}) ||
		len(m.Auth) != 1 || m.Auth[0] != api.AuthKindOAuth2 {
		t.Errorf("microsoft profile = %+v", m)
	}
	if p := ForKind(api.AccountKindIMAP); p.IMAP.Host != "" || p.Quirks != (Quirks{}) {
		t.Errorf("imap profile = %+v", p)
	}
}
