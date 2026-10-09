// CONTRACT TEST for task card T-0095 (docs/tasks). Do not edit.
package pimsync_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/store"
)

// TestUnnamedCollections: iCloud's address book has no display name. A
// collection the server leaves unnamed, or names with only spaces, is
// called by its kind until the server names it.
func TestUnnamedCollections(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts, api.ServiceKindCalendar)
	ctx := t.Context()
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		return tx.SetService(ctx, e.acct.ID, api.ServiceKindTasks, true, e.dav.URL+davtest.CalendarsHome)
	}); err != nil {
		t.Fatal(err)
	}
	book := e.dav.AddressBook("card", "")
	e.dav.Calendar("home", "  ", "", "VEVENT")
	e.dav.Calendar("todo", "", "", "VTODO")
	e.pass()
	for kind, want := range map[api.CollectionKind]string{
		api.CollectionKindAddressbook: "Contacts", api.CollectionKindCalendar: "Calendar", api.CollectionKindTasklist: "Tasks",
	} {
		if cols := e.collections(kind); len(cols) != 1 || cols[0].Name != want {
			t.Errorf("%s = %+v, want %q", kind, cols, want)
		}
	}
	e.dav.Rename(book, "Family")
	e.pass()
	if cols := e.collections(api.CollectionKindAddressbook); len(cols) != 1 || cols[0].Name != "Family" {
		t.Errorf("renamed = %+v", cols)
	}
}

func TestUnnamedGoogleTaskList(t *testing.T) {
	e := newTasksEnv(t, allScopes())
	e.srv.AddList("")
	e.mustPass()
	lists, err := e.db.Collections(t.Context(), store.CollectionFilter{AccountID: e.acct.ID, Kind: api.CollectionKindTasklist})
	if err != nil || len(lists) != 1 || lists[0].Name != "Tasks" {
		t.Errorf("lists = %+v, %v", lists, err)
	}
}
