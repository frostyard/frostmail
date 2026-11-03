package lodge_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	kitlodgeapp "github.com/frostyard/lodge/kit/lodgeapp"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/lodge"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// seedInboxes stores an account with an inbox and an archive: one unread
// message in the inbox, one unread message in the archive and one read
// message in the inbox.
func seedInboxes(t *testing.T) *rpctest.Server {
	t.Helper()
	srv := rpctest.StartWith(t, rpctest.Options{Lodge: true})
	ctx := t.Context()
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		acct, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "me@a.test", DisplayName: "Me",
			Auth: api.AuthKindPassword, IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"},
			SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"}})
		if err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, acct.ID, []store.ServerMailbox{
			{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true},
			{Path: "Archive", Role: api.MailboxRoleArchive, Selectable: true}})
		if err != nil {
			return err
		}
		add := func(box int64, uid uint32, msgid string, seen bool) error {
			_, err := tx.InsertHeaders(ctx, acct.ID, box, []store.MessageHeader{{UID: uid,
				Subject: "Mail " + msgid, From: store.Address{Addr: "ann@a.test"},
				To: []store.Address{{Addr: "me@a.test"}}, MessageID: msgid + "@x.test",
				Flags: store.Flags{Seen: seen}}})
			return err
		}
		if err := add(mbs[0].ID, 1, "unread", false); err != nil {
			return err
		}
		if err := add(mbs[1].ID, 1, "archived", false); err != nil {
			return err
		}
		return add(mbs[0].ID, 2, "read", true)
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

// badgeServer serves srv as mail's lodge socket with unread as its badge
// count, as maild --lodge does.
func badgeServer(t *testing.T, srv *rpctest.Server, unread func(context.Context) (int, error)) *kitlodgeapp.Server {
	t.Helper()
	return lodge.NewServer(lodge.Options{
		Ctx: t.Context(), Version: "test", RPC: srv.RPC, Router: srv.Router, Content: srv.Content, Log: discard(), Unread: unread,
	})
}

// TestHealthBadge: /health's badge is the unread messages in the inboxes,
// and no other mailbox's mail counts.
func TestHealthBadge(t *testing.T) {
	srv := seedInboxes(t)
	l := startLodge(t, badgeServer(t, srv, srv.DB.InboxUnread))
	h, err := l.Health(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !h.OK || h.Badge != 1 {
		t.Errorf("health = %+v, %v; want ok with badge 1", h, err)
	}
}

// TestHealthBadgeAbsent: no count, or a count that fails, sends no badge,
// and a failed count never makes the app degraded.
func TestHealthBadgeAbsent(t *testing.T) {
	t.Run("no count", func(t *testing.T) {
		srv := rpctest.StartWith(t, rpctest.Options{Lodge: true})
		l := startLodge(t, badgeServer(t, srv, nil))
		h, err := l.Health(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !h.OK || h.Badge != 0 {
			t.Errorf("health = %+v, %v; want ok with no badge", h, err)
		}
	})
	t.Run("failed count", func(t *testing.T) {
		srv := rpctest.StartWith(t, rpctest.Options{Lodge: true})
		fail := func(context.Context) (int, error) { return 0, errors.New("the store is closed") }
		l := startLodge(t, badgeServer(t, srv, fail))
		h, err := l.Health(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !h.OK || h.Badge != 0 {
			t.Errorf("health = %+v, %v; want ok with no badge", h, err)
		}
	})
}

// TestHealthBadgeTimeout: a count that outlives 1 s sends no badge and does
// not hold /health, and a second call starts no second count.
func TestHealthBadgeTimeout(t *testing.T) {
	srv := rpctest.StartWith(t, rpctest.Options{Lodge: true})
	release := make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	// An early t.Fatal must not leave the count blocked.
	t.Cleanup(free)
	var counted atomic.Int64
	unread := func(context.Context) (int, error) {
		counted.Add(1)
		<-release
		return 7, nil
	}
	l := startLodge(t, badgeServer(t, srv, unread))
	h, err := l.Health(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !h.OK || h.Badge != 0 {
		t.Fatalf("health with a slow count = %+v, %v; want ok with no badge", h, err)
	}
	// The first count still runs: this call starts none, and answers at once.
	h, err = l.Health(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !h.OK || h.Badge != 0 {
		t.Fatalf("health while a count runs = %+v, %v; want ok with no badge", h, err)
	}
	if n := counted.Load(); n != 1 {
		t.Fatalf("counts so far = %d, want 1", n)
	}
	free()
	// The count that ended cleared the guard, so a later call counts again.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		h, err = l.Health(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !h.OK {
			t.Fatalf("health = %+v, want ok", h)
		}
		if h.Badge == 7 {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("no badge came back after the count was released: %+v", h)
		}
	}
	if n := counted.Load(); n != 2 {
		t.Fatalf("counts = %d, want 2", n)
	}
}
