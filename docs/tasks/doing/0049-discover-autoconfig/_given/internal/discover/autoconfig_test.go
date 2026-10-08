package discover

// CONTRACT TEST for task card T-0049 (docs/tasks). Do not edit.

import (
	"errors"
	"reflect"
	"testing"

	"github.com/frostyard/frostmail/api"
)

const fastmailLike = `<?xml version="1.0" encoding="UTF-8"?>
<clientConfig version="1.1">
  <emailProvider id="example.com">
    <domain>example.com</domain>
    <displayName>Example Mail</displayName>
    <incomingServer type="pop3">
      <hostname>pop.example.com</hostname>
      <port>995</port>
      <socketType>SSL</socketType>
      <username>%EMAILADDRESS%</username>
      <authentication>password-cleartext</authentication>
    </incomingServer>
    <incomingServer type="imap">
      <hostname>imap.example.com</hostname>
      <port>993</port>
      <socketType>SSL</socketType>
      <username>%EMAILADDRESS%</username>
      <authentication>password-cleartext</authentication>
    </incomingServer>
    <incomingServer type="imap">
      <hostname>imap.example.com</hostname>
      <port>143</port>
      <socketType>STARTTLS</socketType>
      <username>%EMAILADDRESS%</username>
      <authentication>password-cleartext</authentication>
    </incomingServer>
    <outgoingServer type="smtp">
      <hostname>smtp.example.com</hostname>
      <port>465</port>
      <socketType>SSL</socketType>
      <username>%EMAILADDRESS%</username>
      <authentication>password-cleartext</authentication>
    </outgoingServer>
  </emailProvider>
</clientConfig>`

func TestParseAutoconfig(t *testing.T) {
	got, err := ParseAutoconfig([]byte(fastmailLike), "ann@example.com")
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{
		IMAP: &Server{Host: "imap.example.com", Port: 993, TLS: api.TLSModeTLS, Username: "ann@example.com"},
		SMTP: &Server{Host: "smtp.example.com", Port: 465, TLS: api.TLSModeTLS, Username: "ann@example.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v / %+v", got.IMAP, got.SMTP)
	}
}

func TestParseAutoconfigPlaceholdersAndChoices(t *testing.T) {
	doc := `<clientConfig version="1.1"><emailProvider id="x">
	  <incomingServer type="imap">
	    <hostname>plain.%EMAILDOMAIN%</hostname><port>143</port><socketType>plain</socketType>
	    <username>%EMAILLOCALPART%</username><authentication>password-cleartext</authentication>
	  </incomingServer>
	  <incomingServer type="imap">
	    <hostname>  mail.%EMAILDOMAIN% </hostname><port>143</port><socketType>STARTTLS</socketType>
	    <username>%EMAILLOCALPART%</username><authentication>OAuth2</authentication><authentication>password-cleartext</authentication>
	  </incomingServer>
	  <outgoingServer type="smtp">
	    <hostname>oauth.%EMAILDOMAIN%</hostname><port>587</port><socketType>STARTTLS</socketType>
	    <username>%EMAILADDRESS%</username><authentication>OAuth2</authentication>
	  </outgoingServer>
	  <outgoingServer type="smtp">
	    <hostname>smtp.%EMAILDOMAIN%</hostname><port>587</port><socketType>STARTTLS</socketType>
	    <username>%EMAILADDRESS%</username><authentication>password-encrypted</authentication>
	  </outgoingServer>
	</emailProvider></clientConfig>`
	got, err := ParseAutoconfig([]byte(doc), "Bob.Smith@Mail.Test")
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{
		IMAP: &Server{Host: "mail.mail.test", Port: 143, TLS: api.TLSModeStartTLS, Username: "Bob.Smith"},
		SMTP: &Server{Host: "smtp.mail.test", Port: 587, TLS: api.TLSModeStartTLS, Username: "Bob.Smith@Mail.Test"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v / %+v", got.IMAP, got.SMTP)
	}
}

func TestParseAutoconfigPartialAndEmpty(t *testing.T) {
	onlyIMAP := `<clientConfig><emailProvider>
	  <incomingServer type="imap"><hostname>imap.x.test</hostname><port>993</port><socketType>SSL</socketType>
	  <username>%EMAILADDRESS%</username><authentication>password-cleartext</authentication></incomingServer>
	  <outgoingServer type="smtp"><hostname>smtp.x.test</hostname><port>99999</port><socketType>SSL</socketType>
	  <username>%EMAILADDRESS%</username><authentication>password-cleartext</authentication></outgoingServer>
	</emailProvider></clientConfig>`
	got, err := ParseAutoconfig([]byte(onlyIMAP), "a@x.test")
	if err != nil || got.IMAP == nil || got.IMAP.Host != "imap.x.test" || got.SMTP != nil {
		t.Errorf("partial = %+v, %+v, %v", got.IMAP, got.SMTP, err)
	}

	for name, doc := range map[string]string{
		"only pop3":   `<clientConfig><emailProvider><incomingServer type="pop3"><hostname>p</hostname><port>995</port><socketType>SSL</socketType><username>u</username><authentication>password-cleartext</authentication></incomingServer></emailProvider></clientConfig>`,
		"only plain":  `<clientConfig><emailProvider><incomingServer type="imap"><hostname>i</hostname><port>143</port><socketType>plain</socketType><username>u</username><authentication>password-cleartext</authentication></incomingServer></emailProvider></clientConfig>`,
		"no host":     `<clientConfig><emailProvider><incomingServer type="imap"><hostname> </hostname><port>993</port><socketType>SSL</socketType><username>u</username><authentication>password-cleartext</authentication></incomingServer></emailProvider></clientConfig>`,
		"no provider": `<clientConfig version="1.1"></clientConfig>`,
	} {
		if _, err := ParseAutoconfig([]byte(doc), "a@x.test"); !errors.Is(err, ErrNothing) {
			t.Errorf("%s: err = %v, want ErrNothing", name, err)
		}
	}
	if _, err := ParseAutoconfig([]byte("<clientConfig><unclosed>"), "a@x.test"); err == nil || errors.Is(err, ErrNothing) {
		t.Errorf("malformed XML: err = %v, want a parse error", err)
	}
	if _, err := ParseAutoconfig([]byte(fastmailLike), "not-an-address"); err == nil {
		t.Error("an address without @ was accepted")
	}
}
