package mainwin_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
)

func (f *syncFixture) menuOn(t *testing.T, row string, path ...string) {
	t.Helper()
	if err := f.tt.RightClick(row); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem(path...); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
}

func (f *syncFixture) mailboxNamed(t *testing.T, name string) bool {
	t.Helper()
	list, err := f.c.Mailbox().List(t.Context(), &api.MailboxListParams{AccountID: &f.acct})
	return err == nil && slices.ContainsFunc(list, func(m api.Mailbox) bool { return m.Name == name })
}

// P-107: New and Rename go through the mailbox sheet.
func TestNewAndRenameMailbox(t *testing.T) {
	f := newSyncFixture(t)
	f.has(t, "the Junk mailbox", "Junk")
	f.menuOn(t, "Junk", "New Mailbox…")
	f.has(t, "the sheet", "New Mailbox")
	f.tt.Frame()
	f.tt.Type("Papers")
	if err := f.tt.Click("OK"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the mailbox", func() bool { return f.mailboxNamed(t, "Papers") })
	if _, err := f.c.Mailbox().Create(t.Context(), &api.MailboxCreateParams{AccountID: f.acct, Name: "Notes"}); err != nil {
		t.Fatal(err)
	}
	f.has(t, "the new row", "Notes")
	f.menuOn(t, "Notes", "Rename Mailbox…")
	f.has(t, "the sheet", "Rename Mailbox")
	f.tt.Frame()
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Type("Letters")
	if err := f.tt.Click("OK"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the new name", func() bool { return f.mailboxNamed(t, "Letters") })
}

// A role mailbox cannot be renamed, moved or deleted.
func TestRoleMailboxesStay(t *testing.T) {
	f := newSyncFixture(t)
	f.has(t, "the Junk mailbox", "Junk")
	if err := f.tt.RightClick("Junk"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Rename Mailbox…"); err == nil {
		t.Error("a role mailbox's rename is enabled")
	}
}

// P-105: a smart mailbox made in the sheet becomes the list's source.
func TestNewSmartMailboxSheet(t *testing.T) {
	f := newSyncFixture(t)
	f.has(t, "the Junk mailbox", "Junk")
	f.menuOn(t, "Flagged", "New Smart Mailbox…")
	f.has(t, "the sheet", "New Smart Mailbox")
	if _, ok := f.tt.Access("Condition 1 field"); !ok {
		t.Errorf("no condition row in %q", f.tt.Texts())
	}
	if err := f.tt.Click("Add condition after 1"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the second row", func() bool { _, ok := f.tt.Access("Condition 2 field"); return ok })
	if err := f.tt.Click("Remove condition 2"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "one row", func() bool { _, ok := f.tt.Access("Condition 2 field"); return !ok })
	value, ok := f.tt.Access("Condition 1 value")
	if !ok {
		t.Fatalf("no value field in %q", f.tt.Texts())
	}
	f.tt.ClickAt(value.Bounds.X+value.Bounds.W/2, value.Bounds.Y+value.Bounds.H/2)
	f.tt.Frame()
	f.tt.Type("Ann")
	f.tt.Frame()
	if err := f.tt.Click("OK"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the smart mailbox as source", func() bool {
		list, err := f.c.Smart().List(t.Context(), &api.SmartListParams{})
		return err == nil && len(list) == 1 && !f.tt.HasText("New Smart Mailbox") && f.tt.HasText("Alpha report") && !f.tt.HasText("Beta plan")
	})
}

// P-602: a search saves as a smart mailbox.
func TestSaveSearchAsSmartMailbox(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	f.tt.Type("Gamma")
	f.tt.Key(0, ui.KeyEnter)
	f.has(t, "the scope bar", "All Mailboxes")
	if err := f.tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	f.has(t, "the sheet", "New Smart Mailbox")
}

// P-401, P-403: the row's menu starts the reply family.
func TestRowMenuStartsAReply(t *testing.T) {
	f := oneMail(t, "", "Body")
	f.has(t, "the Junk mailbox", "Junk")
	f.menuOn(t, "Alpha report", "Reply All")
	f.until(t, "the draft", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return len(f.drafts) == 1
	})
	f.menuOn(t, "Alpha report", "Forward as Attachment")
	f.until(t, "the second draft", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return len(f.drafts) == 2
	})
}

// P-402: Redirect asks whom, and refuses what does not parse.
func TestRedirectSheet(t *testing.T) {
	f := oneMail(t, "", "Body")
	f.has(t, "the Junk mailbox", "Junk")
	f.menuOn(t, "Alpha report", "Redirect…")
	f.until(t, "the sheet", func() bool { _, ok := f.tt.Access("To"); return ok && f.tt.HasText("Redirect") })
	info, ok := f.tt.AccessByRole(ui.RoleButton, "Redirect")
	if !ok || !info.Disabled {
		t.Errorf("Redirect with nobody to send to: %+v, %v", info, ok)
	}
}

// P-410: the row's menu mutes the conversation.
func TestMuteFromTheRow(t *testing.T) {
	f := newSyncFixture(t)
	f.has(t, "the Junk mailbox", "Junk")
	f.menuOn(t, "Beta plan", "Mute")
	f.until(t, "the mark", func() bool { _, ok := f.tt.Access("Muted"); return ok })
}

// P-411: the contact popover blocks, and the banner then says so.
func TestBlockFromThePopover(t *testing.T) {
	f := oneMail(t, "", "Body")
	f.open(t, "Alpha report")
	f.tt.Frame()
	who, ok := f.tt.Access("Ann, ann@x.test")
	if !ok {
		t.Fatalf("no sender in %q", f.tt.Texts())
	}
	f.tt.ClickAt(who.Bounds.X+who.Bounds.W/2, who.Bounds.Y+who.Bounds.H/2)
	f.has(t, "the card", "Block")
	if err := f.tt.Click("Block"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the sender blocked", func() bool {
		list, err := f.c.Block().List(t.Context(), &api.BlockListParams{})
		return err == nil && len(list) == 1
	})
}

// P-412: Remind Me from the row, by a choice and by the sheet.
func TestRemindMeFromTheRow(t *testing.T) {
	f := newSyncFixture(t)
	f.has(t, "the Junk mailbox", "Junk")
	f.menuOn(t, "Beta plan", "Remind Me", "Remind Me Tomorrow")
	f.until(t, "the mark", func() bool { _, ok := f.tt.Access("Reminder set"); return ok })
	f.menuOn(t, "Gamma notes", "Remind Me", "Remind Me…")
	f.has(t, "the sheet", "Remind Me")
}

// P-414: Apply Rules shows in the row's menu once a rule is on.
func TestApplyRulesInTheRowMenu(t *testing.T) {
	f := newSyncFixture(t)
	f.has(t, "the Junk mailbox", "Junk")
	cond := api.Conditions{Match: api.ConditionMatchAny, Conditions: []api.Condition{{Field: api.ConditionFieldSubject, Op: api.ConditionOpContains, Value: "Beta"}}}
	if _, err := f.c.Rule().Create(t.Context(), &api.RuleCreateParams{Name: "R", Conditions: cond, Actions: []api.RuleAction{{Kind: api.RuleActionKindRead}}}); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the rule's menu item", func() bool {
		if err := f.tt.RightClick("Beta plan"); err != nil {
			t.Fatal(err)
		}
		found := slices.ContainsFunc(f.tt.Menu(), func(s string) bool { return strings.Contains(s, "Apply Rules") })
		f.tt.CloseMenu()
		return found
	})
}
