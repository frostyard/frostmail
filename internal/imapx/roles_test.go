package imapx_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/store"
)

// iCloud's LIST for an account other clients have used (recorded
// 2026-10-08): its own folders carry \Sent and \Trash, the others only
// names that look like roles.
func TestAssignRolesPrefersTheServersAttributes(t *testing.T) {
	list := []store.ServerMailbox{
		{Path: "Sent Items", Delimiter: "/"},
		{Path: "Archive", Delimiter: "/"},
		{Path: "Junk", Delimiter: "/"},
		{Path: "Trash", Delimiter: "/"},
		{Path: "Deleted Items", Delimiter: "/"},
		{Path: "INBOX", Delimiter: "/", Attrs: []string{`\Noinferiors`}},
		{Path: "Notes", Delimiter: "/"},
		{Path: "Sent", Delimiter: "/"},
		{Path: "Notes/Recovered Items", Delimiter: "/"},
		{Path: "Deleted Messages", Delimiter: "/", Attrs: []string{`\Trash`}},
		{Path: "Sent Messages", Delimiter: "/", Attrs: []string{`\Sent`}},
		{Path: "Drafts", Delimiter: "/"},
	}
	imapx.AssignRoles(list)
	want := map[string]api.MailboxRole{
		"INBOX": api.MailboxRoleInbox, "Sent Messages": api.MailboxRoleSent, "Deleted Messages": api.MailboxRoleTrash,
		"Drafts": api.MailboxRoleDrafts, "Junk": api.MailboxRoleJunk, "Archive": api.MailboxRoleArchive,
	}
	for _, mb := range list {
		w, ok := want[mb.Path]
		if !ok {
			w = api.MailboxRoleNone
		}
		if mb.Role != w {
			t.Errorf("%s: role %q, want %q", mb.Path, mb.Role, w)
		}
	}
}

func TestAssignRolesFallsBackToNamesInListOrder(t *testing.T) {
	list := []store.ServerMailbox{{Path: "Sent"}, {Path: "Sent Messages"}, {Path: "Trash"}, {Path: "INBOX"}}
	imapx.AssignRoles(list)
	got := []api.MailboxRole{list[0].Role, list[1].Role, list[2].Role, list[3].Role}
	want := []api.MailboxRole{api.MailboxRoleSent, api.MailboxRoleNone, api.MailboxRoleTrash, api.MailboxRoleInbox}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s: role %q, want %q", list[i].Path, got[i], want[i])
		}
	}
}
