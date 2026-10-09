//go:build integration

package engine_test

// Runs against Radicale in the frostmail-mailtest container (make
// engine-it) as test3@mailtest.test, whose address book is empty in the
// clean snapshot: contacts reach People from another client and from Add
// to Contacts, and verify then finds both sides the same (plan 0007,
// Phase 2).

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func TestRadicaleContacts(t *testing.T) {
	host := os.Getenv("FROSTMAIL_IT_HOST")
	if host == "" {
		t.Skip("FROSTMAIL_IT_HOST is not set; run make engine-it")
	}
	const user, password = "test3@mailtest.test", "frostmail-test"
	insecure := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // the test container's certificate
	}}
	srv := rpctest.StartWith(t, rpctest.Options{PIM: &pimsync.Config{HTTP: insecure, Interval: time.Hour}})
	c := srv.Dial(t)
	ctx := t.Context()
	p := createParams(user)
	p.IMAP.Host, p.SMTP.Host = host, host
	a, err := c.Account().Create(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: password}); err != nil {
		t.Fatal(err)
	}
	start := "https://" + host + ":5232/"
	svc, err := c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: a.ID, Service: api.ServiceKindContacts,
		Enabled: true, URL: &start})
	if err != nil || !svc.Enabled {
		t.Fatalf("setService = %+v, %v", svc, err)
	}
	pass := func() {
		t.Helper()
		if err := srv.PIM.Pass(ctx, a.ID); err != nil {
			t.Fatalf("Pass: %v", err)
		}
	}
	pass()
	cols, err := c.Account().Collections(ctx, &api.AccountCollectionsParams{AccountID: &a.ID})
	if err != nil || len(cols) != 1 || cols[0].Name != "Contacts" {
		t.Fatalf("collections = %+v, %v", cols, err)
	}

	// Another client adds a contact; it reaches People.
	other := otherClient(t, ctx, start, user, password, insecure)
	book := "/" + strings.ReplaceAll(user, "@", "%40") + "/contacts/"
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:radicale-ada\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\n" +
		"EMAIL;TYPE=INTERNET:ada@example.com\r\nEND:VCARD\r\n"
	if _, err := other.Put(ctx, book+"radicale-ada.vcf", []byte(card), davx.AddressBooks, ""); err != nil {
		t.Fatal(err)
	}
	pass()
	people, err := c.People().List(ctx, &api.PeopleListParams{})
	if err != nil || len(people) != 1 || people[0].DisplayName != "Ada Lovelace" {
		t.Fatalf("people after another client's contact = %+v, %v", people, err)
	}

	// Add to Contacts reaches the server.
	if _, err := c.People().Add(ctx, &api.PeopleAddParams{Email: "grace@navy.example", Name: ptr("Grace Hopper")}); err != nil {
		t.Fatal(err)
	}
	pass()
	list, err := other.List(ctx, book)
	if err != nil || len(list) != 2 {
		t.Fatalf("server objects = %+v, %v", list, err)
	}
	var found bool
	for _, o := range list {
		obj, err := other.Get(ctx, o.Href)
		if err != nil {
			t.Fatal(err)
		}
		found = found || strings.Contains(string(obj.Data), "Grace Hopper")
	}
	if !found {
		t.Error("the added contact is not on the server")
	}
	checks, err := srv.PIM.Verify(ctx, a.ID)
	if err != nil || len(checks) != 1 || !pimsync.Clean(checks[0]) || checks[0].Server != 2 || checks[0].Local != 2 {
		t.Errorf("verify = %+v, %v", checks, err)
	}

	// Another client removes Ada; she leaves People.
	if err := other.Delete(ctx, book+"radicale-ada.vcf", ""); err != nil {
		t.Fatal(err)
	}
	pass()
	if people, _ := c.People().List(ctx, &api.PeopleListParams{}); len(people) != 1 || people[0].DisplayName != "Grace Hopper" {
		t.Errorf("people after the delete = %+v", people)
	}
}

func otherClient(t *testing.T, ctx context.Context, start, user, password string, hc *http.Client) *davx.Client {
	t.Helper()
	opts := davx.Options{HTTP: hc, Authorization: func(context.Context) (string, error) {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password)), nil
	}}
	home, err := davx.Discover(ctx, start, davx.AddressBooks, opts)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := davx.New(home, opts)
	if err != nil {
		t.Fatal(err)
	}
	return cl
}
