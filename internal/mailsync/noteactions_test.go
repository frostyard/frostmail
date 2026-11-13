package mailsync_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/notify"
)

// P-805: a notification's Archive and Trash move its message, here and on
// the server, and new mail is announced with the buttons its account's
// mailboxes allow. A second Trash, or an Archive after it, leaves the
// message in Trash: a button never erases.
func TestNotificationButtonsMoveTheMessage(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	appendNew(t, mem, "INBOX", "a")
	appendNew(t, mem, "INBOX", "b")
	announced := make(chan []notify.Mail, 8)
	cfg := testConfig
	cfg.Announce = func(_ context.Context, _ int64, mail []notify.Mail) error {
		announced <- mail
		return nil
	}
	h := newHarnessWith(t, mem.DialOptions(), imapxtest.Password, cfg)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	archive, trash := h.mailboxByPath("Archive"), h.mailboxByPath("Trash")

	appendNew(t, mem, "INBOX", "c")
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	select {
	case mail := <-announced:
		if len(mail) != 1 || !mail[0].Archive || !mail[0].Trash {
			t.Fatalf("announced %+v, want one message with Archive and Trash", mail)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the new message was not announced")
	}

	a, b := storedID(t, h.srv.DB, "a@x.test"), storedID(t, h.srv.DB, "b@x.test")
	settled := func(id, mailbox int64) func() bool {
		return func() bool { return !h.hasQueuedOps() && h.onlyIn(id) == mailbox }
	}
	if err := h.srv.Sync.ActOnAnnounced(ctx, a, notify.ActionArchive); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "a in Archive", settled(a, archive.ID))
	if err := h.srv.Sync.ActOnAnnounced(ctx, b, notify.ActionTrash); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "b in Trash", settled(b, trash.ID))
	if got := serverCount(t, mem, "Archive"); got != 1 {
		t.Errorf("Archive on the server = %d, want 1", got)
	}

	// The notification is still up: its buttons do nothing more to b.
	for _, action := range []string{notify.ActionTrash, notify.ActionArchive} {
		if err := h.srv.Sync.ActOnAnnounced(ctx, b, action); err != nil {
			t.Fatal(err)
		}
		if h.hasQueuedOps() {
			t.Errorf("%s on a message in Trash queued an op", action)
		}
	}
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	if got := serverCount(t, mem, "Trash"); got != 1 || h.onlyIn(b) != trash.ID {
		t.Errorf("after the second clicks: Trash on the server = %d, b in mailbox %d; want 1 and Trash (%d)", got, h.onlyIn(b), trash.ID)
	}

	if err := h.srv.Sync.ActOnAnnounced(ctx, 1<<40, notify.ActionTrash); err != nil {
		t.Errorf("a message that is gone: %v", err)
	}
	if err := h.srv.Sync.ActOnAnnounced(ctx, a, notify.ActionReply); !errors.Is(err, mailsync.ErrInvalid) {
		t.Errorf("an action that moves nothing: %v, want ErrInvalid", err)
	}
}

// On an account with no Trash and no archive mailbox, where Delete erases,
// the notes offer neither button and neither action touches the message.
func TestNotificationButtonsNeedTheirMailboxes(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	for _, name := range []string{"Trash", "Archive"} {
		if err := mem.User.Delete(name); err != nil {
			t.Fatal(err)
		}
	}
	appendNew(t, mem, "INBOX", "a")
	announced := make(chan []notify.Mail, 8)
	cfg := testConfig
	cfg.Announce = func(_ context.Context, _ int64, mail []notify.Mail) error {
		announced <- mail
		return nil
	}
	h := newHarnessWith(t, mem.DialOptions(), imapxtest.Password, cfg)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	appendNew(t, mem, "INBOX", "b")
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	select {
	case mail := <-announced:
		if len(mail) != 1 || mail[0].Archive || mail[0].Trash {
			t.Fatalf("announced %+v, want one message with neither button", mail)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the new message was not announced")
	}
	a, inbox := storedID(t, h.srv.DB, "a@x.test"), h.inbox()
	for _, action := range []string{notify.ActionTrash, notify.ActionArchive} {
		if err := h.srv.Sync.ActOnAnnounced(ctx, a, action); err != nil {
			t.Fatal(err)
		}
		if h.hasQueuedOps() || h.onlyIn(a) != inbox.ID {
			t.Errorf("%s without its mailbox: queued ops %v, message in %d, want INBOX (%d)", action, h.hasQueuedOps(), h.onlyIn(a), inbox.ID)
		}
	}
	if got := serverCount(t, mem, "INBOX"); got != 2 {
		t.Errorf("INBOX on the server = %d, want 2", got)
	}
}
