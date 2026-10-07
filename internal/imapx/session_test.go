package imapx_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/store"
)

// seedMem appends dev/incus/seed/*.eml to INBOX, in file order (UIDs 1-5).
func seedMem(t *testing.T, mem *imapxtest.Mem) {
	t.Helper()
	files, err := filepath.Glob("../../dev/incus/seed/*.eml")
	if err != nil || len(files) != 5 {
		t.Fatalf("seed files = %v, %v", files, err)
	}
	slices.Sort(files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := mem.User.Append("INBOX", bytes.NewReader(data), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
	}
}

func openMem(t *testing.T) *imapx.Session {
	t.Helper()
	mem := imapxtest.StartMem(t)
	seedMem(t, mem)
	s, err := imapx.Open(t.Context(), mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSessionListAndRoles(t *testing.T) {
	s := openMem(t)
	list, err := s.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]api.MailboxRole{}
	for _, mb := range list {
		roles[mb.Path] = mb.Role
		if !mb.Selectable {
			t.Errorf("%s is not selectable", mb.Path)
		}
	}
	want := map[string]api.MailboxRole{
		"INBOX": api.MailboxRoleInbox, "Sent": api.MailboxRoleSent, "Drafts": api.MailboxRoleDrafts,
		"Trash": api.MailboxRoleTrash, "Junk": api.MailboxRoleJunk, "Archive": api.MailboxRoleArchive,
	}
	for path, role := range want {
		if roles[path] != role {
			t.Errorf("role of %s = %q, want %q", path, roles[path], role)
		}
	}
}

func TestRoleFor(t *testing.T) {
	cases := []struct {
		path, delim string
		attrs       []string
		want        api.MailboxRole
	}{
		{"inbox", "/", nil, api.MailboxRoleInbox},
		{"Gesendet", "/", []string{`\Sent`}, api.MailboxRoleSent},
		{"[Gmail]/All Mail", "/", []string{`\All`, `\HasNoChildren`}, api.MailboxRoleAll},
		{"Sent Messages", "/", nil, api.MailboxRoleSent},
		{"Deleted Messages", ".", nil, api.MailboxRoleTrash},
		{"Junk E-mail", "/", nil, api.MailboxRoleJunk},
		{"Projects/Sent", "/", nil, api.MailboxRoleNone},
		{"Receipts", "/", nil, api.MailboxRoleNone},
	}
	for _, tc := range cases {
		if got := imapx.RoleFor(tc.path, tc.delim, tc.attrs); got != tc.want {
			t.Errorf("RoleFor(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestSessionSelectAndFetchHeaders(t *testing.T) {
	s := openMem(t)
	ctx := t.Context()
	sel, err := s.Select(ctx, "INBOX")
	if err != nil {
		t.Fatal(err)
	}
	if sel.Messages != 5 || sel.UIDValidity == 0 || sel.UIDNext != 6 {
		t.Fatalf("select = %+v", sel)
	}
	uids, err := s.UIDs(ctx)
	if err != nil || !slices.Equal(uids, []uint32{1, 2, 3, 4, 5}) {
		t.Fatalf("UIDs = %v, %v", uids, err)
	}
	hs, err := s.FetchHeaders(ctx, []uint32{5, 1, 3, 4, 2, 99})
	if err != nil {
		t.Fatal(err)
	}
	byUID := map[uint32]store.MessageHeader{}
	for _, h := range hs {
		byUID[h.UID] = h
	}
	if len(byUID) != 5 {
		t.Fatalf("got %d headers", len(byUID))
	}

	welcome := byUID[1]
	if welcome.Subject != "Welcome to the frostmail test server" || welcome.From != (store.Address{Name: "Alice Example", Addr: "alice@mailtest.test"}) ||
		welcome.MessageID != "welcome-0001@mailtest.test" || welcome.Date.IsZero() || welcome.Size == 0 || welcome.InternalDate.IsZero() {
		t.Errorf("welcome = %+v", welcome)
	}
	if !strings.HasPrefix(welcome.Preview, "Hello! This mailbox is reset") || !strings.Contains(welcome.Preview, "café") {
		t.Errorf("welcome preview = %q", welcome.Preview)
	}

	reply := byUID[3]
	if reply.InReplyTo != "thread-0001@mailtest.test" || !slices.Equal(reply.References, []string{"thread-0001@mailtest.test"}) {
		t.Errorf("reply threading = %q %v", reply.InReplyTo, reply.References)
	}
	if reply.Preview != "Thursday works. Noon?" {
		t.Errorf("reply preview = %q (quotes must be dropped)", reply.Preview)
	}

	news := byUID[4]
	if news.Subject != "October deals — up to 40% off" || news.ListID != "news.shop.mailtest.test" ||
		news.ListUnsubscribe != "<https://shop.mailtest.test/unsub?u=1>" || news.HasAttachments {
		t.Errorf("newsletter = %+v", news)
	}
	if news.Preview != "October deals: up to 40% off. View in a browser: https://shop.mailtest.test/oct" {
		t.Errorf("newsletter preview = %q (from text/plain)", news.Preview)
	}

	att := byUID[5]
	wantParts := []store.Part{
		{Path: "1", ContentType: "text/plain", Charset: "utf-8", Encoding: "7bit", Size: att.Parts[0].Size},
		{Path: "2", ContentType: "text/csv", Encoding: "base64", Disposition: "attachment", Filename: "q3.csv", Size: att.Parts[1].Size},
	}
	if !att.HasAttachments || !slices.Equal(att.Parts, wantParts) || att.Preview != "Numbers attached." {
		t.Errorf("attachment message parts = %+v preview %q", att.Parts, att.Preview)
	}
}

func TestSessionFlagsAndRaw(t *testing.T) {
	s := openMem(t)
	ctx := t.Context()
	if _, err := s.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}
	if err := s.StoreFlags(ctx, []uint32{2, 3}, []string{`\Seen`, `\Flagged`, "$MailFlagBit0"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.StoreFlags(ctx, []uint32{3}, nil, []string{`\Flagged`}); err != nil {
		t.Fatal(err)
	}
	ups, err := s.FetchFlags(ctx, []uint32{1, 2, 3}, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[uint32]store.Flags{}
	for _, u := range ups {
		got[u.UID] = u.Flags
	}
	if got[1].Seen || !got[2].Seen || !got[2].Flagged || got[2].Color != 2 || !got[3].Seen || got[3].Flagged {
		t.Fatalf("flags = %+v", got)
	}
	raw, err := s.FetchRaw(ctx, 1)
	if err != nil || !bytes.Contains(raw, []byte("Subject: Welcome to the frostmail test server")) {
		t.Fatalf("raw = %q, %v", raw, err)
	}
	if _, err := s.FetchRaw(ctx, 77); err == nil {
		t.Fatal("FetchRaw of a missing UID succeeded")
	}
	if _, err := s.Move(ctx, []uint32{1}, "Archive"); err == nil {
		t.Fatal("Move without MOVE or UIDPLUS succeeded")
	}
}

func TestSessionStatusAndIdleWake(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seedMem(t, mem)
	woke := make(chan struct{}, 8)
	opts := mem.DialOptions()
	opts.OnUpdate = func() {
		select {
		case woke <- struct{}{}:
		default:
		}
	}
	s, err := imapx.Open(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := t.Context()
	st, err := s.Status(ctx, "INBOX")
	if err != nil || st.Messages != 5 || st.UIDNext != 6 {
		t.Fatalf("status = %+v, %v", st, err)
	}
	if _, err := s.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}
	idling := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- s.Idle(ctx, woke, time.Minute, func() { close(idling) }) }()
	<-idling
	if _, err := mem.User.Append("INBOX", bytes.NewReader([]byte("Subject: new\r\n\r\nhi\r\n")), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("IDLE did not wake on a new message")
	}
}
