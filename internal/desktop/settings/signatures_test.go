package settings_test

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/settings"
)

func (f *fixture) identities() []api.Identity {
	f.t.Helper()
	list, err := f.c.Identity().List(f.t.Context(), &api.IdentityListParams{})
	if err != nil {
		f.t.Fatal(err)
	}
	return list
}

func TestSignaturesWithNoAccount(t *testing.T) {
	f := newFixture(t, setup{})
	f.show(settings.PaneSignatures)
	f.sees("Add an account to edit its signature.")
}

func TestSignatureEditsAsTextAndSavesAsHTML(t *testing.T) {
	f := newFixture(t, setup{first: func(f *fixture) { f.addIMAP("me@a.test") }})
	f.show(settings.PaneSignatures)
	f.sees("Signature:")
	if !f.disabled("Save") {
		t.Error("Save works with nothing changed")
	}
	if !f.disabled("Remove Address") {
		t.Error("an account's own address can be removed")
	}
	f.fill("Name:", "Ann Example")
	f.fill("Reply-To:", "reply@a.test")
	f.press(f.control(ui.RoleTextField, "Signature:"))
	f.tt.Type("Ann <3")
	f.tt.Key(0, ui.KeyEnter)
	f.tt.Key(0, ui.KeyEnter)
	f.tt.Type("A & B")
	if f.disabled("Save") {
		t.Error("Save is off with changes")
	}
	f.click("Save")
	f.until("the saved identity", func() bool {
		i := f.identities()[0]
		return i.Name == "Ann Example" && i.ReplyTo == "reply@a.test" && i.SignatureHTML == "<p>Ann &lt;3</p><p><br></p><p>A &amp; B</p>"
	})
	f.until("Save off again", func() bool { return f.disabled("Save") })
	if got := f.value("Signature:"); got != "Ann <3\n\nA & B" {
		t.Errorf("the signature reads %q", got)
	}
}

func TestSignatureFieldsFollowTheStoredValues(t *testing.T) {
	f := newFixture(t, setup{first: func(f *fixture) { f.addIMAP("me@a.test") }})
	f.show(settings.PaneSignatures)
	f.sees("Signature:")
	id := f.identities()[0].ID
	name, html := "Changed Elsewhere", "<div>Hello<br>there</div>"
	if _, err := f.c.Identity().Update(t.Context(), &api.IdentityUpdateParams{ID: id, Name: &name, SignatureHTML: &html}); err != nil {
		t.Fatal(err)
	}
	// The pane reads the identities again when the accounts change.
	display := "Account Name"
	if _, err := f.c.Account().Update(t.Context(), &api.AccountUpdateParams{ID: f.listed()[0].ID, DisplayName: &display}); err != nil {
		t.Fatal(err)
	}
	f.until("the new values", func() bool { return f.value("Name:") == "Changed Elsewhere" })
	if got := f.value("Signature:"); got != "Hello\nthere" {
		t.Errorf("the signature reads %q", got)
	}
}

func TestAddAndRemoveAnAddress(t *testing.T) {
	f := newFixture(t, setup{first: func(f *fixture) { f.addIMAP("me@a.test") }})
	f.show(settings.PaneSignatures)
	f.sees("Signature:")
	f.click("Add Address")
	f.sees("An address your provider delivers to this account, such as an alias or your own domain. Frostmail sends from it and answers invitations to it.")
	if !f.disabled("Add") || !f.disabled("Add Address") || !f.disabled("Remove Address") {
		t.Error("the add form's buttons are on")
	}
	if f.choice("Account") != "me@a.test" {
		t.Errorf("Account = %q", f.choice("Account"))
	}
	f.fill("Email Address:", "alias@a.test")
	f.fill("Full Name:", "Alias Name")
	f.click("Add")
	f.until("the new address selected", func() bool { return f.tt.HasText("Reply-To:") && f.value("Name:") == "Alias Name" })
	if len(f.identities()) != 2 {
		t.Fatalf("identities = %+v", f.identities())
	}
	if f.disabled("Remove Address") {
		t.Error("an added address cannot be removed")
	}
	f.click("Remove Address")
	f.sees("Remove alias@a.test? Drafts from it will be sent from me@a.test.")
	f.press(f.control(ui.RoleButton, "Remove"))
	f.until("the removal", func() bool { return len(f.identities()) == 1 })
	f.until("the list without it", func() bool { return !f.tt.HasText("alias@a.test") })
}

func TestAddAddressErrorLeavesTheFormOpen(t *testing.T) {
	f := newFixture(t, setup{first: func(f *fixture) { f.addIMAP("me@a.test") }})
	f.show(settings.PaneSignatures)
	f.sees("Signature:")
	f.click("Add Address")
	f.fill("Email Address:", "me@a.test") // already there
	f.click("Add")
	f.until("maild's error", func() bool { _, ok := f.tt.AccessByRole(ui.RoleAlert, ""); return ok })
	f.sees("Cancel")
	f.click("Cancel")
	f.sees("Reply-To:")
}

func TestChoosingAnotherIdentityClosesTheAddForm(t *testing.T) {
	f := newFixture(t, setup{first: func(f *fixture) {
		f.addIMAP("me@a.test")
		f.addIMAP("you@a.test")
	}})
	f.show(settings.PaneSignatures)
	f.sees("Signature:")
	f.click("Add Address")
	f.sees("Cancel")
	f.press(f.control(ui.RoleButton, "you@a.test"))
	f.sees("Reply-To:")
	f.notSeen("Cancel")
}
