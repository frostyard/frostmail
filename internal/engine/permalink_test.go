package engine_test

import (
	"errors"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// TestByMessageIDPrefersTheInbox resolves lodge's permanent route
// /message/<Message-ID>: the inbox's copy over one in Sent, which beats
// one in Trash; across accounts, the earliest stored.
func TestByMessageIDPrefersTheInbox(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	a, err := c.Account().Create(ctx, createParams("test1@mailtest.test"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Account().Create(ctx, createParams("test2@mailtest.test"))
	if err != nil {
		t.Fatal(err)
	}
	hdr := func(uid uint32, msgid string) store.MessageHeader {
		at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
		return store.MessageHeader{UID: uid, InternalDate: at, Date: at, MessageID: msgid, Subject: "Plan",
			From: store.Address{Addr: "ann@mailtest.test"}}
	}
	// put stores one message in the account's mailbox of role.
	put := func(account int64, role api.MailboxRole, uid uint32, msgid string) (id int64) {
		t.Helper()
		err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
			mbs, err := tx.ReplaceMailboxes(ctx, account, []store.ServerMailbox{
				{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true},
				{Path: "Sent", Role: api.MailboxRoleSent, Selectable: true},
				{Path: "Trash", Role: api.MailboxRoleTrash, Selectable: true},
			})
			if err != nil {
				return err
			}
			for _, mb := range mbs {
				if mb.Role == role {
					got, err := tx.InsertHeaders(ctx, account, mb.ID, []store.MessageHeader{hdr(uid, msgid)})
					if err == nil {
						id = got[0]
					}
					return err
				}
			}
			return errors.New("no such mailbox")
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	put(a.ID, api.MailboxRoleTrash, 1, "plan@x")
	put(a.ID, api.MailboxRoleSent, 1, "plan@x")
	inbox := put(b.ID, api.MailboxRoleInbox, 1, "plan@x")
	trashed := put(a.ID, api.MailboxRoleTrash, 2, "gone@x")
	earliest := put(a.ID, api.MailboxRoleSent, 3, "mine@x")
	put(b.ID, api.MailboxRoleSent, 4, "mine@x")

	for msgid, want := range map[string]int64{
		"plan@x":   inbox,
		"<plan@x>": inbox,
		"gone@x":   trashed,
		"mine@x":   earliest,
	} {
		s, err := c.Message().ByMessageID(ctx, &api.MessageByMessageIDParams{MessageID: msgid})
		if err != nil || s.ID != want {
			t.Errorf("byMessageId(%q) = %v, %v; want message %d", msgid, s, err, want)
		}
	}
	if _, err := c.Message().ByMessageID(ctx, &api.MessageByMessageIDParams{MessageID: "nobody@x"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("an unknown Message-ID = %v, want notFound", err)
	}
	if _, err := c.Message().ByMessageID(ctx, &api.MessageByMessageIDParams{MessageID: " <> "}); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("an empty Message-ID = %v, want invalidParams", err)
	}
}
