//go:build integration

package smtpx_test

// Runs against the frostmail-mailtest container (dev/incus/mailtest.sh):
// make engine-it sets FROSTMAIL_IT_HOST and restores the clean snapshot.
// Delivery into Dovecot is checked by the engine's send tests.

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/smtpx"
)

func postfix(t *testing.T) smtpx.Options {
	host := os.Getenv("FROSTMAIL_IT_HOST")
	if host == "" {
		t.Skip("FROSTMAIL_IT_HOST is not set; run make engine-it")
	}
	return smtpx.Options{
		Host: host, Port: 587, TLS: api.TLSModeStartTLS,
		Username: "test1@mailtest.test", Password: "frostmail-test",
		InsecureSkipVerify: true,
	}
}

const itMessage = "From: test1@mailtest.test\r\nTo: test2@mailtest.test\r\n" +
	"Subject: smtpx integration\r\nMessage-ID: <smtpx-it@mailtest.test>\r\n\r\nHello from smtpx.\r\n"

func TestPostfixSubmission(t *testing.T) {
	opts := postfix(t)
	env := smtpx.Envelope{From: "test1@mailtest.test", To: []string{"test2@mailtest.test"}}
	if err := smtpx.Send(t.Context(), opts, env, int64(len(itMessage)), strings.NewReader(itMessage)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	max, err := smtpx.MaxSize(t.Context(), opts)
	if err != nil || max != 52428800 {
		t.Fatalf("MaxSize = %d, %v; want the container's 50 MB", max, err)
	}
}

func TestPostfixRejectsBadPassword(t *testing.T) {
	opts := postfix(t)
	opts.Password = "wrong"
	env := smtpx.Envelope{From: "test1@mailtest.test", To: []string{"test2@mailtest.test"}}
	err := smtpx.Send(t.Context(), opts, env, int64(len(itMessage)), strings.NewReader(itMessage))
	if !errors.Is(err, smtpx.ErrAuth) || !smtpx.Permanent(err) {
		t.Fatalf("Send = %v; want a permanent ErrAuth", err)
	}
}
