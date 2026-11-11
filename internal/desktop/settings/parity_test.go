package settings_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
)

// P-802: a scope chosen in General is what maild keeps.
func TestGeneralNotifyScope(t *testing.T) {
	f := newFixture(t, setup{})
	f.general()
	f.choose("New message notifications", "VIPs")
	f.until("the scope", func() bool { return f.settings().NotifyScope == api.NotifyScopeVips })
	f.choose("New message notifications", "All Mailboxes")
	f.until("the wider scope", func() bool { return f.settings().NotifyScope == api.NotifyScopeAll })
}

// P-952: a new account is writable unless the box says otherwise.
func TestNewAccountsAreWritableUnlessChecked(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		f, _ := accountsFixture(t, nil)
		f.sees("New Account")
		f.fill("Email Address:", "ann@a.test")
		f.fill("Full Name:", "Ann Example")
		f.fill("Password:", "test-password")
		f.fill("IMAP Server", "imap.a.test")
		f.fill("IMAP User Name", "ann@a.test")
		f.fill("SMTP Server", "smtp.a.test")
		f.fill("SMTP User Name", "ann@a.test")
		if f.checked("Read only: never change anything on the server") {
			t.Fatal("Read only starts checked")
		}
		if readOnly {
			f.check("Read only: never change anything on the server")
		}
		f.click("Add This Account")
		f.until("the account", func() bool { return len(f.listed()) == 1 })
		if got := f.listed()[0].ReadOnly; got != readOnly {
			t.Errorf("checked %v, the account's ReadOnly is %v", readOnly, got)
		}
	}
}
