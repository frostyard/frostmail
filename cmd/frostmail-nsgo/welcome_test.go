//go:build linux && integration

package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// firstRun starts the client on a maild with no account, with or without
// a host install's mail beside it and what the device's storage holds.
func firstRun(t *testing.T, hostMail bool, storage map[string]string) *nsgotest.App {
	t.Helper()
	a, _ := fixtureStart(t, rootWith(clientOptions{hostMail: func() bool { return hostMail }}), false, "", 1280, 820, false, nsgotest.Storage(storage))
	a.WaitFor("id:welcome", 10*time.Second)
	return a
}

// TestFirstRunWelcomes checks the first run: a store with no account
// shows the welcome in place of the modules, the account's dialog opens
// itself once, and the welcome's buttons lead to an account and to the
// Server pane.
func TestFirstRunWelcomes(t *testing.T) {
	a := firstRun(t, false, nil)
	a.Find("id:welcome >> text:Welcome to Frostmail")
	a.Find("id:welcome >> text:Frostmail keeps its mail on this computer.")
	// The first look at the connection opens the account's dialog.
	a.WaitFor("role:dialog >> text:Add mail account", 5*time.Second)
	a.Press("role:dialog >> label:Cancel")
	a.WaitForGone("role:dialog", 5*time.Second)
	a.Advance(2 * time.Second)
	a.Settle()
	if a.Exists("role:dialog") {
		t.Fatal("the account's dialog opened itself a second time on 1 connection")
	}
	if a.Exists("id:move-note") {
		t.Error("the move note shows with no host install's mail")
	}
	// The other modules show the welcome too; Settings does not.
	a.Press("role:tab Calendar")
	a.WaitFor("id:welcome", 5*time.Second)
	a.Press("id:welcome >> label:Add Account…")
	a.WaitFor("role:dialog >> text:Add mail account", 5*time.Second)
	a.Press("role:dialog >> label:Cancel")
	a.WaitForGone("role:dialog", 5*time.Second)
	a.Press("id:welcome >> label:Connect to lodge…")
	a.WaitFor("id:settings", 5*time.Second)
	a.WaitFor("label:Lodge address (empty for local maild)", 5*time.Second)
	if a.Exists("id:welcome") {
		t.Error("the welcome shows in place of Settings")
	}
}

// TestFirstRunSaysHowToMoveMail checks the move note: beside a host
// install's mail the welcome gives the 3 commands, and a dismissal is
// kept on the device.
func TestFirstRunSaysHowToMoveMail(t *testing.T) {
	a := firstRun(t, true, nil)
	a.WaitFor("role:dialog >> text:Add mail account", 5*time.Second)
	a.Press("role:dialog >> label:Cancel")
	a.WaitForGone("role:dialog", 5*time.Second)
	for _, step := range moveSteps {
		a.Find("id:move-note >> text:" + step)
	}
	a.Press("id:move-note >> label:Dismiss")
	a.WaitForGone("id:move-note", 5*time.Second)
	if kept, _ := a.Storage().GetItem(moveNoteKey); kept != moveNoteDismissed {
		t.Errorf("the device keeps %q of the dismissal, want %q", kept, moveNoteDismissed)
	}
	a.Find("id:welcome >> label:Add Account…")
}

// TestDismissedMoveNoteStaysAway checks that a start does not show a note
// the device dismissed.
func TestDismissedMoveNoteStaysAway(t *testing.T) {
	a := firstRun(t, true, map[string]string{moveNoteKey: moveNoteDismissed})
	if a.Exists("id:move-note") {
		t.Error("the move note shows after the device dismissed it")
	}
}

// TestAStoreWithAccountsHasNoWelcome checks that the showcase's store,
// which has accounts, shows its mail and opens no dialog.
func TestAStoreWithAccountsHasNoWelcome(t *testing.T) {
	a := fixtureWorkspace(t)
	a.WaitFor("id:messages >> id:summary-when >> nth=0", 8*time.Second)
	a.Advance(2 * time.Second)
	a.Settle()
	if a.Exists("id:welcome") || a.Exists("role:dialog") {
		t.Errorf("a store with accounts shows the welcome or a dialog\n%s", a.Dump())
	}
}

// TestPicturesOfTheFirstRun names pictures of a store with no account:
// the account's dialog over the welcome, as a first start shows it, and
// the welcome with the note of a host install's mail.
func TestPicturesOfTheFirstRun(t *testing.T) {
	a := firstRun(t, true, nil)
	a.WaitFor("role:dialog >> text:Add mail account", 5*time.Second)
	a.Screenshot("first-run-account")
	a.Press("role:dialog >> label:Cancel")
	a.WaitForGone("role:dialog", 5*time.Second)
	a.Screenshot("first-run")
}

// TestStoreChangesLeaveTheListInPlace checks that the window's content is
// not built again when the store changes: the search field over the list
// is the same view after a message is flagged, which changes the store's
// counts. The welcome's condition reads the store, and must not make the
// content follow every change of it.
func TestStoreChangesLeaveTheListInPlace(t *testing.T) {
	a := fixtureWorkspace(t)
	const search = "label:Search All Inboxes"
	const row = "id:messages >> label:Your trip to Lisbon: itinerary"
	a.WaitFor(row, 8*time.Second)
	field := a.Find(search).ID
	flagged := func() int { return len(a.Query("id:messages >> id:flag-mark")) }
	before := flagged()
	a.Press(row)
	a.WaitFor("label:Message body", 8*time.Second)
	a.Press("role:toolbar >> label:Flag")
	a.WaitUntil(5*time.Second, func() bool { return flagged() == before+1 })
	a.Advance(time.Second)
	a.Settle()
	if got := a.Find(search).ID; got != field {
		t.Errorf("the search field is view %v after a store change and was %v: the list was built again", got, field)
	}
}
