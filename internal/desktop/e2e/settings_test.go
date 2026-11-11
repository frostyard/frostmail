//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

type heldAccountSave struct {
	api.AccountService
	arrived atomic.Bool
	release chan struct{}
}

func (h *heldAccountSave) Update(ctx context.Context, p *api.AccountUpdateParams) (*api.Account, error) {
	h.arrived.Store(true)
	select {
	case <-h.release:
		return h.AccountService.Update(ctx, p)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestAccountFieldsDuringAnotherAccountsSave(t *testing.T) {
	h := &heldAccountSave{release: make(chan struct{})}
	s := start(t, options{fixture: []string{"-n", "2"}, server: &rpctest.Options{Services: func(services *api.Services) {
		h.AccountService, services.Account = services.Account, h
	}}})
	var once sync.Once
	release := func() { once.Do(func() { close(h.release) }) }
	t.Cleanup(release)
	for _, name := range []string{"first", "second"} {
		_, err := s.client.Account().Create(t.Context(), &api.AccountCreateParams{
			Kind: api.AccountKindIMAP, Email: name + "@mailtest.test", DisplayName: name, Auth: api.AuthKindPassword,
			IMAP: &api.ServerConfig{Host: "imap." + name + ".test", Port: 993, TLS: api.TLSModeTLS, Username: "in-" + name},
			SMTP: &api.ServerConfig{Host: "smtp." + name + ".test", Port: 465, TLS: api.TLSModeTLS, Username: "out-" + name},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	s.key(1, "CmdOrCtrl+,")
	w := s.waitWindow("^Settings$")
	s.press(w, s.waitNode(w, "treeItem", "^Accounts"))
	s.press(w, s.waitNode(w, "", "^first$"))
	field := s.waitNode(w, "textField", "^IMAP Server$")
	s.must("act", map[string]any{"window": w, "id": field.ID, "action": "setValue", "text": "imap.changed.test"}, nil)
	s.idle(w)
	s.press(w, s.waitNode(w, "button", "^Save$"))
	eventually(t, "the held account save", h.arrived.Load)
	s.press(w, s.waitNode(w, "", "^second$"))
	s.waitNode(w, "heading", "^second$")
	s.idle(w)
	// Optional captures make the same scenario inspectable on a real
	// desktop without storing fixture screenshots in the repository.
	if dir := os.Getenv("FROSTMAIL_E2E_CAPTURE_DIR"); dir != "" {
		var captured struct {
			PNG []byte `json:"png"`
		}
		s.must("capture", map[string]any{"window": w}, &captured)
		if err := os.WriteFile(filepath.Join(dir, "account-during-save.png"), captured.PNG, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Contains(s.waitNode(w, "button", "^Save$").States, "disabled") {
		t.Error("Save is enabled during the held save")
	}
	for _, check := range []struct{ name, want string }{
		{"IMAP Server", "imap.second.test"}, {"IMAP Port", "993"}, {"IMAP User Name", "in-second"},
		{"SMTP Server", "smtp.second.test"}, {"SMTP Port", "465"}, {"SMTP User Name", "out-second"},
	} {
		field := s.waitNode(w, "textField", "^"+check.name+"$")
		t.Logf("during first account's held save: %s = %q", check.name, field.Value)
		if !slices.Contains(field.States, "disabled") {
			t.Errorf("%s is enabled during the held save", check.name)
		}
		if field.Value != check.want {
			t.Errorf("%s = %q, want %q", check.name, field.Value, check.want)
		}
	}
	release()
	s.waitFor(w, "the save's end", func(tree []node) bool {
		button := find(tree, "button", "^Save$")
		if button == nil {
			return false
		}
		for _, state := range button.States {
			if state == "disabled" {
				return false
			}
		}
		return true
	})
	if got := s.waitNode(w, "textField", "^IMAP Server$").Value; got != "imap.second.test" {
		t.Errorf("after the save IMAP Server = %q", got)
	}
}
