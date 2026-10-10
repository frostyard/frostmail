package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func davServer(t *testing.T) (*rpctest.Server, *davtest.Server) {
	t.Helper()
	dav := davtest.New(t, davtest.Options{})
	srv := rpctest.StartWith(t, rpctest.Options{PIM: &pimsync.Config{HTTP: dav.Client, Interval: time.Hour}})
	return srv, dav
}

func davAccount(t *testing.T, c *api.Client) int64 {
	t.Helper()
	ctx := t.Context()
	p := createParams("user@dav.test")
	p.IMAP.Username = davtest.User
	a, err := c.Account().Create(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: davtest.Password}); err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func TestServicesAPI(t *testing.T) {
	srv, dav := davServer(t)
	c := srv.Dial(t)
	ctx := t.Context()
	id := davAccount(t, c)

	list, err := c.Account().Services(ctx, &api.AccountServicesParams{ID: id})
	if err != nil || len(list) != 3 {
		t.Fatalf("services = %+v, %v", list, err)
	}
	for i, kind := range []api.ServiceKind{api.ServiceKindContacts, api.ServiceKindCalendar, api.ServiceKindTasks} {
		s := list[i]
		if s.Service != kind || !s.Available || s.Enabled || !s.SignedIn || s.URL != "" {
			t.Errorf("service %d = %+v", i, s)
		}
	}

	book := dav.AddressBook("default", "Contacts")
	dav.Put(book+"ada.vcf", []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:ada\r\nFN:Ada Lovelace\r\nEMAIL:ada@example.com\r\nEND:VCARD\r\n"))
	on, err := c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: id, Service: api.ServiceKindContacts, Enabled: true, URL: ptr(dav.URL)})
	if err != nil || !on.Enabled || on.URL != dav.URL {
		t.Fatalf("setService contacts = %+v, %v", on, err)
	}
	if err := srv.PIM.Pass(ctx, id); err != nil {
		t.Fatal(err)
	}
	cols, err := c.Account().Collections(ctx, &api.AccountCollectionsParams{AccountID: &id})
	if err != nil || len(cols) != 1 || cols[0].Name != "Contacts" || cols[0].Kind != api.CollectionKindAddressbook || !cols[0].IsDefault {
		t.Fatalf("collections = %+v, %v", cols, err)
	}
	people, err := c.People().List(ctx, &api.PeopleListParams{})
	if err != nil || len(people) != 1 || people[0].DisplayName != "Ada Lovelace" {
		t.Errorf("people = %+v, %v", people, err)
	}

	// Hiding the address book hides its people; turning contacts off too.
	off := false
	col, err := c.Account().SetCollection(ctx, &api.AccountSetCollectionParams{ID: cols[0].ID, Enabled: &off})
	if err != nil || col.Enabled {
		t.Errorf("setCollection = %+v, %v", col, err)
	}
	if people, _ := c.People().List(ctx, &api.PeopleListParams{}); len(people) != 0 {
		t.Errorf("people of a hidden book = %+v", people)
	}
	if _, err := c.Account().SetCollection(ctx, &api.AccountSetCollectionParams{ID: cols[0].ID, IsDefault: ptr(false)}); code(err) != api.CodeInvalidParams {
		t.Errorf("clearing a default = %v", err)
	}
	on, err = c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: id, Service: api.ServiceKindContacts, Enabled: false})
	if err != nil || on.Enabled || on.URL != dav.URL {
		t.Errorf("contacts off = %+v, %v", on, err)
	}

	// A server that does not answer is reported, and nothing is turned on.
	_, err = c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: id, Service: api.ServiceKindCalendar, Enabled: true,
		URL: ptr("https://127.0.0.1:1/")})
	if code(err) != api.CodeUnavailable {
		t.Errorf("unreachable calendar server = %v", err)
	}
	if list, _ := c.Account().Services(ctx, &api.AccountServicesParams{ID: id}); list[1].Enabled {
		t.Errorf("calendar = %+v", list[1])
	}
	if err := c.Sync().Pim(ctx, &api.SyncPimParams{}); err != nil {
		t.Errorf("sync.pim = %v", err)
	}
}

func TestServicesByProvider(t *testing.T) {
	srv, _ := davServer(t)
	c := srv.Dial(t)
	ctx := t.Context()
	icloud, err := c.Account().Create(ctx, &api.AccountCreateParams{Kind: api.AccountKindICloud, Email: "ann@icloud.com", Auth: api.AuthKindPassword})
	if err != nil {
		t.Fatal(err)
	}
	list, _ := c.Account().Services(ctx, &api.AccountServicesParams{ID: icloud.ID})
	if !list[0].Available || !list[1].Available || list[2].Available {
		t.Errorf("iCloud services = %+v", list)
	}
	// Settings says why iCloud has no tasks; the services it has need no reason.
	if list[2].Reason == nil || !strings.Contains(*list[2].Reason, "Apple Reminders") || list[0].Reason != nil || list[1].Reason != nil {
		t.Errorf("iCloud reasons = %v, %v, %v", list[0].Reason, list[1].Reason, list[2].Reason)
	}
	if _, err := c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: icloud.ID, Service: api.ServiceKindTasks, Enabled: true}); code(err) != api.CodeInvalidParams {
		t.Errorf("iCloud tasks = %v", err)
	}

	// A Google account whose grant lacks the scope turns the service on
	// at the profile's address and waits for a sign-in.
	g, err := c.Account().Create(ctx, &api.AccountCreateParams{Kind: api.AccountKindGmail, Email: "ann@gmail.com", Auth: api.AuthKindOAuth2})
	if err != nil {
		t.Fatal(err)
	}
	on, err := c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: g.ID, Service: api.ServiceKindCalendar, Enabled: true})
	if err != nil || !on.Enabled || on.SignedIn || on.URL != "https://apidata.googleusercontent.com/caldav/v2/" {
		t.Errorf("Google calendar = %+v, %v", on, err)
	}
	tasks, err := c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: g.ID, Service: api.ServiceKindTasks, Enabled: true})
	if err != nil || tasks.URL != "https://tasks.googleapis.com/tasks/v1/" || tasks.SignedIn {
		t.Errorf("Google tasks = %+v, %v", tasks, err)
	}
	ms, err := c.Account().Create(ctx, &api.AccountCreateParams{Kind: api.AccountKindMicrosoft, Email: "ann@outlook.com",
		Auth: api.AuthKindPassword, IMAP: createParams("x").IMAP, SMTP: createParams("x").SMTP})
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := c.Account().Services(ctx, &api.AccountServicesParams{ID: ms.ID}); list[0].Available || list[1].Available || list[2].Reason != nil {
		t.Errorf("Microsoft services = %+v", list)
	}
}

// TestAddToContacts: people.add stores the contact at once and the next
// pass writes it to the server, which then holds the same vCard.
func TestAddToContacts(t *testing.T) {
	srv, dav := davServer(t)
	c := srv.Dial(t)
	ctx := t.Context()
	id := davAccount(t, c)
	book := dav.AddressBook("default", "Contacts")
	shared := dav.AddressBook("shared", "Shared")
	dav.SetReadOnly(shared, true)
	if _, err := c.People().Add(ctx, &api.PeopleAddParams{Email: "x@example.com"}); code(err) != api.CodeNotFound {
		t.Errorf("add without an address book = %v", err)
	}
	if _, err := c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: id, Service: api.ServiceKindContacts,
		Enabled: true, URL: ptr(dav.URL)}); err != nil {
		t.Fatal(err)
	}
	if err := srv.PIM.Pass(ctx, id); err != nil {
		t.Fatal(err)
	}

	p, err := c.People().Add(ctx, &api.PeopleAddParams{Email: " Grace@Navy.example ", Name: ptr("Grace Hopper")})
	if err != nil {
		t.Fatal(err)
	}
	if p.DisplayName != "Grace Hopper" || len(p.Contacts) != 1 || p.Contacts[0].FamilyName != "Hopper" ||
		p.Contacts[0].Emails[0].Value != "grace@navy.example" {
		t.Errorf("added = %+v", p)
	}
	if _, err := c.People().Add(ctx, &api.PeopleAddParams{Email: "grace@navy.example"}); code(err) != api.CodeConflict {
		t.Errorf("adding again = %v", err)
	}
	if _, err := c.People().Add(ctx, &api.PeopleAddParams{Email: "nope"}); code(err) != api.CodeInvalidParams {
		t.Errorf("a bad address = %v", err)
	}
	cols, _ := c.Account().Collections(ctx, &api.AccountCollectionsParams{AccountID: &id})
	var sharedID int64
	for _, col := range cols {
		if col.Name == "Shared" {
			sharedID = col.ID
		}
	}
	if _, err := c.People().Add(ctx, &api.PeopleAddParams{Email: "y@example.com", CollectionID: &sharedID}); code(err) != api.CodeNotFound {
		t.Errorf("add to a read-only book = %v", err)
	}

	if err := srv.PIM.Pass(ctx, id); err != nil {
		t.Fatal(err)
	}
	paths := dav.Paths(book)
	if len(paths) != 1 {
		t.Fatalf("server paths = %q", paths)
	}
	data, etag, _ := dav.Object(paths[0])
	if !strings.Contains(string(data), "FN:Grace Hopper\r\n") || !strings.Contains(string(data), "EMAIL;TYPE=INTERNET:grace@navy.example") {
		t.Errorf("server vCard = %q", data)
	}
	// The next pass finds nothing new: the local copy has the server's ETag.
	dav.ResetRequests()
	if err := srv.PIM.Pass(ctx, id); err != nil {
		t.Fatal(err)
	}
	for _, r := range dav.Requests() {
		if r.Report == "addressbook-multiget" || r.Method == "PUT" {
			t.Errorf("after the write, a pass sent %s %s %s (etag %s)", r.Method, r.Path, r.Report, etag)
		}
	}
	people, _ := c.People().List(ctx, &api.PeopleListParams{})
	if len(people) != 1 || people[0].DisplayName != "Grace Hopper" {
		t.Errorf("people = %+v", people)
	}
}
