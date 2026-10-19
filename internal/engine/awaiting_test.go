package engine_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// waitNow is the clock of the waiting tests: Tuesday 2026-10-20, noon UTC.
var waitNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// waitMsg is one message of a waiting fixture's thread.
type waitMsg struct {
	mailboxes []string // by path; the first is the message's own
	from      string
	to, cc    []string
	ago       time.Duration
	answered  bool
	draft     bool
	list      string // List-Id
	unsub     string // List-Unsubscribe
}

// waitThread is a fixture thread: its messages, oldest first, in an
// account.
type waitThread struct {
	acct  string // "a", "b" or "g"
	msgs  []waitMsg
	muted bool
}

func addrs(list []string) []store.Address {
	out := make([]store.Address, 0, len(list))
	for _, a := range list {
		out = append(out, store.Address{Addr: a})
	}
	return out
}

// waitServer stores the threads, by subject, in three accounts: me@a.test
// (who also sends as alias@a.test) and me@b.test, each with INBOX, Sent,
// Archive, Drafts, Trash, Junk and All Mail, and the Gmail account
// me@g.test, whose mailboxes but All Mail, Trash and Junk are labels: a
// message's later mailboxes there are labels. vip@x.test is a VIP, and
// spam@x.test is blocked. It returns the newest message's ID of each
// thread by subject, and the accounts' IDs.
func waitServer(t *testing.T, threads map[string]waitThread) (*rpctest.Server, map[string]int64, map[string]int64) {
	t.Helper()
	srv := rpctest.StartWith(t, rpctest.Options{Now: func() time.Time { return waitNow }})
	ctx := t.Context()
	newest := map[string]int64{}
	accts := map[string]int64{}
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mailboxes := map[string]map[string]int64{}
		for _, name := range []string{"a", "b", "g"} {
			email := "me@" + name + ".test"
			kind, auth := api.AccountKindIMAP, api.AuthKindPassword
			if name == "g" {
				kind, auth = api.AccountKindGmail, api.AuthKindOAuth2
			}
			a, err := tx.InsertAccount(ctx, store.Account{Kind: kind, Email: email, DisplayName: email,
				Auth: auth, IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: email},
				SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: email}})
			if err != nil {
				return err
			}
			accts[name] = a.ID
			mbs, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{
				{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true},
				{Path: "Sent", Role: api.MailboxRoleSent, Selectable: true},
				{Path: "Archive", Role: api.MailboxRoleArchive, Selectable: true},
				{Path: "Drafts", Role: api.MailboxRoleDrafts, Selectable: true},
				{Path: "Trash", Role: api.MailboxRoleTrash, Selectable: true},
				{Path: "Junk", Role: api.MailboxRoleJunk, Selectable: true},
				{Path: "All Mail", Role: api.MailboxRoleAll, Selectable: true},
			})
			if err != nil {
				return err
			}
			mailboxes[name] = map[string]int64{}
			for _, mb := range mbs {
				mailboxes[name][mb.Path] = mb.ID
			}
			if name == "g" {
				if err := tx.MarkGmailLabels(ctx, a.ID); err != nil {
					return err
				}
			}
		}
		if _, err := tx.AddIdentity(ctx, accts["a"], "Me", "Alias@A.test"); err != nil {
			return err
		}
		if err := tx.AddVIPs(ctx, []string{"vip@x.test"}); err != nil {
			return err
		}
		if err := tx.Block(ctx, []string{"spam@x.test"}); err != nil {
			return err
		}
		uid := uint32(0)
		names := slices.Sorted(func(yield func(string) bool) {
			for k := range threads {
				if !yield(k) {
					return
				}
			}
		})
		for _, subject := range names {
			th := threads[subject]
			acct := accts[th.acct]
			var ids []int64
			var prev string
			for i, m := range th.msgs {
				uid++
				date := waitNow.Add(-m.ago)
				msgid := fmt.Sprintf("%s-%d@test", strings.ReplaceAll(subject, " ", "-"), i)
				h := store.MessageHeader{UID: uid, Size: 100, Subject: subject, From: store.Address{Name: "Sender", Addr: m.from},
					To: addrs(m.to), Cc: addrs(m.cc), Date: date, InternalDate: date, MessageID: msgid, Preview: subject,
					ListID: m.list, ListUnsubscribe: m.unsub, Flags: store.Flags{Answered: m.answered, Draft: m.draft, Seen: true}}
				if prev != "" {
					h.InReplyTo, h.References = prev, []string{prev}
				}
				prev = msgid
				got, err := tx.InsertHeaders(ctx, acct, mailboxes[th.acct][m.mailboxes[0]], []store.MessageHeader{h})
				if err != nil {
					return err
				}
				for _, label := range m.mailboxes[1:] {
					if _, err := tx.EditGmailLabels(ctx, got[0], []int64{mailboxes[th.acct][label]}, nil); err != nil {
						return err
					}
				}
				ids = append(ids, got[0])
			}
			if _, err := tx.AssignThreads(ctx, acct, ids); err != nil {
				return err
			}
			if th.muted {
				if err := tx.MuteThreads(ctx, ids[:1], true); err != nil {
					return err
				}
			}
			newest[subject] = ids[len(ids)-1]
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, newest, accts
}

func subjects(a *api.Awaiting) []string {
	var out []string
	for _, th := range a.Threads {
		out = append(out, th.Message.Subject)
	}
	return out
}

const hour = time.Hour

// TestAwaitingReply: each rule of thread.awaitingReply leaves out a
// thread that breaks only that rule (docs/specs/lodge-app.md, Calendar,
// tasks and contacts).
func TestAwaitingReply(t *testing.T) {
	in := []string{"INBOX"}
	toMe := []string{"me@a.test"}
	ann := func(ago time.Duration) waitMsg { return waitMsg{mailboxes: in, from: "ann@x.test", to: toMe, ago: ago} }
	threads := map[string]waitThread{
		"Kept":                    {acct: "a", msgs: []waitMsg{ann(48 * hour)}},
		"From a VIP":              {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "VIP@x.test", to: toMe, ago: 100 * hour}}},
		"To an alias":             {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "ann@x.test", to: []string{"alias@a.test"}, ago: 50 * hour}}},
		"Among others in To":      {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "ann@x.test", to: []string{"bob@x.test", "Me@A.test"}, ago: 51 * hour}}},
		"A draft after":           {acct: "a", msgs: []waitMsg{ann(60 * hour), {mailboxes: []string{"Drafts"}, from: "me@a.test", to: []string{"ann@x.test"}, ago: hour, draft: true}}},
		"Junk after":              {acct: "a", msgs: []waitMsg{ann(61 * hour), {mailboxes: []string{"Junk"}, from: "bob@x.test", to: toMe, ago: hour}}},
		"In Gmail's INBOX label":  {acct: "g", msgs: []waitMsg{{mailboxes: []string{"All Mail", "INBOX"}, from: "ann@x.test", to: []string{"me@g.test"}, ago: 62 * hour}}},
		"Almost 30 days":          {acct: "a", msgs: []waitMsg{ann(30*24*hour - time.Minute)}},
		"In the other account":    {acct: "b", msgs: []waitMsg{{mailboxes: in, from: "ann@x.test", to: []string{"me@b.test"}, ago: 70 * hour}}},
		"Replied":                 {acct: "a", msgs: []waitMsg{ann(80 * hour), {mailboxes: []string{"Sent"}, from: "me@a.test", to: []string{"ann@x.test"}, ago: 79 * hour}}},
		"Replied from the alias":  {acct: "a", msgs: []waitMsg{ann(81 * hour), {mailboxes: []string{"Sent"}, from: "alias@a.test", to: []string{"ann@x.test"}, ago: 2 * hour}}},
		"Cc only":                 {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "ann@x.test", to: []string{"bob@x.test"}, cc: toMe, ago: 3 * hour}}},
		"To another account":      {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "ann@x.test", to: []string{"me@b.test"}, ago: 3 * hour}}},
		"From a list":             {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "ann@x.test", to: toMe, ago: 3 * hour, list: "<l.x.test>"}}},
		"With List-Unsubscribe":   {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "ann@x.test", to: toMe, ago: 3 * hour, unsub: "<mailto:u@x.test>"}}},
		"Answered":                {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "ann@x.test", to: toMe, ago: 3 * hour, answered: true}}},
		"Muted":                   {acct: "a", msgs: []waitMsg{ann(3 * hour)}, muted: true},
		"From a blocked sender":   {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "spam@x.test", to: toMe, ago: 3 * hour}}},
		"Archived":                {acct: "a", msgs: []waitMsg{{mailboxes: []string{"Archive"}, from: "ann@x.test", to: toMe, ago: 3 * hour}}},
		"Archived in Gmail":       {acct: "g", msgs: []waitMsg{{mailboxes: []string{"All Mail"}, from: "ann@x.test", to: []string{"me@g.test"}, ago: 3 * hour}}},
		"In the Trash":            {acct: "a", msgs: []waitMsg{{mailboxes: []string{"Trash"}, from: "ann@x.test", to: toMe, ago: 3 * hour}}},
		"Exactly 30 days":         {acct: "a", msgs: []waitMsg{ann(30 * 24 * hour)}},
		"From me to me":           {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "me@a.test", to: toMe, ago: 3 * hour}}},
		"From the alias to me":    {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "alias@a.test", to: toMe, ago: 3 * hour}}},
		"Only a draft":            {acct: "a", msgs: []waitMsg{{mailboxes: []string{"Drafts"}, from: "ann@x.test", to: toMe, ago: 3 * hour, draft: true}}},
		"A draft flag in the box": {acct: "a", msgs: []waitMsg{{mailboxes: in, from: "ann@x.test", to: toMe, ago: 3 * hour, draft: true}}},
	}
	for _, local := range noReplyLocals {
		threads["From "+local] = waitThread{acct: "a", msgs: []waitMsg{{mailboxes: in, from: strings.ToUpper(local) + "@x.test", to: toMe, ago: 3 * hour}}}
	}
	srv, newest, accts := waitServer(t, threads)
	ctx := t.Context()
	c := srv.Dial(t)

	got, err := c.Thread().AwaitingReply(ctx, &api.ThreadAwaitingReplyParams{Limit: new(int64(25))})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"From a VIP", "Kept", "To an alias", "Among others in To", "A draft after", "Junk after",
		"In Gmail's INBOX label", "In the other account", "Almost 30 days"}
	if got := subjects(got); !slices.Equal(got, want) {
		t.Errorf("awaitingReply = %q, want %q", got, want)
	}
	if got.Total != int64(len(want)) {
		t.Errorf("total = %d, want %d", got.Total, len(want))
	}
	vip, kept := got.Threads[0], got.Threads[1]
	if !vip.Vip || kept.Vip || kept.WaitingDays != 2 || vip.WaitingDays != 4 || kept.AccountID != accts["a"] ||
		kept.Message.ID != newest["Kept"] || kept.ThreadID != kept.Message.ThreadID || kept.RecipientCount != 1 ||
		len(kept.Recipients) != 1 || kept.Recipients[0].Address != "me@a.test" {
		t.Errorf("the VIP's and Ann's = %+v, %+v", vip, kept)
	}
	for _, th := range got.Threads {
		if th.Message.Subject == "A draft after" && th.Message.From.Address != "ann@x.test" {
			t.Errorf("a thread's newest past its draft = %+v, want Ann's", th.Message)
		}
	}

	// The defaults: the five newest after the VIPs.
	if d, err := c.Thread().AwaitingReply(ctx, &api.ThreadAwaitingReplyParams{}); err != nil || d.Total != int64(len(want)) ||
		!slices.Equal(subjects(d), want[:5]) {
		t.Errorf("awaitingReply's defaults = %q of %d, %v; want %q", subjects(d), d.Total, err, want[:5])
	}
	// waitingDays includes a thread exactly that old, withinDays leaves out
	// one exactly that old.
	if d, err := c.Thread().AwaitingReply(ctx, &api.ThreadAwaitingReplyParams{Limit: new(int64(25)), WaitingDays: new(int64(2))}); err != nil ||
		!slices.Contains(subjects(d), "Kept") || !slices.Contains(subjects(d), "A draft after") {
		t.Errorf("waitingDays 2 = %q, %v; want Kept, waiting exactly 2 days", subjects(d), err)
	}
	if d, err := c.Thread().AwaitingReply(ctx, &api.ThreadAwaitingReplyParams{Limit: new(int64(25)), WaitingDays: new(int64(3))}); err != nil ||
		slices.Contains(subjects(d), "Kept") || !slices.Contains(subjects(d), "From a VIP") {
		t.Errorf("waitingDays 3 = %q, %v; want the VIP's and not Ann's", subjects(d), err)
	}
	if d, err := c.Thread().AwaitingReply(ctx, &api.ThreadAwaitingReplyParams{Limit: new(int64(25)), WithinDays: new(int64(2))}); err != nil ||
		slices.Contains(subjects(d), "Kept") {
		t.Errorf("withinDays 2 = %q, %v; want Ann's left out, exactly 2 days old", subjects(d), err)
	}

	// An agent's scope narrows it to its accounts.
	agent, _ := srv.DialAs(t, api.Caller{Kind: api.CallerKindAgent, Token: "t", Scope: api.Scope{Accounts: []int64{accts["a"]}}})
	if d, err := agent.Thread().AwaitingReply(ctx, &api.ThreadAwaitingReplyParams{Limit: new(int64(25))}); err != nil ||
		slices.Contains(subjects(d), "In the other account") || slices.Contains(subjects(d), "In Gmail's INBOX label") ||
		d.Total != int64(len(want)-2) {
		t.Errorf("awaitingReply under account=%d = %q, %v; want the other accounts' left out", accts["a"], subjects(d), err)
	}
}

var noReplyLocals = []string{"noreply", "no-reply", "donotreply", "do-not-reply", "mailer-daemon", "postmaster"}

// TestAwaitingOthers: each rule of thread.awaitingOthers, and the
// recipients' cut at 10.
func TestAwaitingOthers(t *testing.T) {
	sent := []string{"Sent"}
	mine := func(ago time.Duration, to ...string) waitMsg {
		return waitMsg{mailboxes: sent, from: "me@a.test", to: to, ago: ago}
	}
	ten, eleven := make([]string, 10), make([]string, 11)
	for i := range eleven {
		eleven[i] = fmt.Sprintf("p%d@x.test", i)
	}
	copy(ten, eleven)
	threads := map[string]waitThread{
		"Kept":                   {acct: "a", msgs: []waitMsg{mine(6*24*hour, "bob@x.test")}},
		"To a VIP":               {acct: "a", msgs: []waitMsg{mine(8*24*hour, "bob@x.test", "VIP@x.test")}},
		"From the alias":         {acct: "a", msgs: []waitMsg{{mailboxes: sent, from: "alias@a.test", to: []string{"bob@x.test"}, ago: 7 * 24 * hour}}},
		"Cc someone":             {acct: "a", msgs: []waitMsg{{mailboxes: sent, from: "me@a.test", to: []string{"me@b.test"}, cc: []string{"carol@x.test"}, ago: 9 * 24 * hour}}},
		"Exactly 5 days":         {acct: "a", msgs: []waitMsg{mine(5*24*hour, "bob@x.test")}},
		"To ten":                 {acct: "a", msgs: []waitMsg{mine(10*24*hour, ten...)}},
		"To eleven":              {acct: "a", msgs: []waitMsg{mine(11*24*hour, eleven...)}},
		"In the other account":   {acct: "b", msgs: []waitMsg{{mailboxes: sent, from: "me@b.test", to: []string{"bob@x.test"}, ago: 12 * 24 * hour}}},
		"Not 5 days yet":         {acct: "a", msgs: []waitMsg{mine(5*24*hour-time.Minute, "bob@x.test")}},
		"Answered":               {acct: "a", msgs: []waitMsg{mine(8*24*hour, "bob@x.test"), {mailboxes: []string{"INBOX"}, from: "bob@x.test", to: []string{"me@a.test"}, ago: 6 * 24 * hour}}},
		"To myself":              {acct: "a", msgs: []waitMsg{mine(6*24*hour, "me@a.test", "alias@a.test", "me@b.test")}},
		"Muted":                  {acct: "a", msgs: []waitMsg{mine(6*24*hour, "bob@x.test")}, muted: true},
		"From someone else":      {acct: "a", msgs: []waitMsg{{mailboxes: []string{"INBOX"}, from: "bob@x.test", to: []string{"me@a.test"}, ago: 6 * 24 * hour}}},
		"Thirty days":            {acct: "a", msgs: []waitMsg{mine(30*24*hour, "bob@x.test")}},
		"A draft is not a reply": {acct: "a", msgs: []waitMsg{mine(13*24*hour, "bob@x.test"), {mailboxes: []string{"Drafts"}, from: "bob@x.test", to: []string{"me@a.test"}, ago: hour, draft: true}}},
	}
	srv, _, _ := waitServer(t, threads)
	ctx := t.Context()
	c := srv.Dial(t)
	got, err := c.Thread().AwaitingOthers(ctx, &api.ThreadAwaitingOthersParams{Limit: new(int64(25))})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"To a VIP", "Exactly 5 days", "Kept", "From the alias", "Cc someone", "To ten", "To eleven",
		"In the other account", "A draft is not a reply"}
	if got := subjects(got); !slices.Equal(got, want) {
		t.Errorf("awaitingOthers = %q, want %q", got, want)
	}
	for _, th := range got.Threads {
		switch th.Message.Subject {
		case "To a VIP":
			if !th.Vip || th.WaitingDays != 8 || th.RecipientCount != 2 {
				t.Errorf("to a VIP = %+v", th)
			}
		case "To ten":
			if th.RecipientCount != 10 || len(th.Recipients) != 10 {
				t.Errorf("to ten = %d of %d recipients, want all", len(th.Recipients), th.RecipientCount)
			}
		case "To eleven":
			if th.RecipientCount != 11 || len(th.Recipients) != 10 || th.Recipients[9].Address != "p9@x.test" {
				t.Errorf("to eleven = %d of %d recipients, want the first 10", len(th.Recipients), th.RecipientCount)
			}
		}
	}
	if d, err := c.Thread().AwaitingOthers(ctx, &api.ThreadAwaitingOthersParams{}); err != nil || d.Total != int64(len(want)) ||
		!slices.Equal(subjects(d), want) {
		t.Errorf("awaitingOthers' defaults = %q of %d, %v; want all %d", subjects(d), d.Total, err, len(want))
	}
	if d, err := c.Thread().AwaitingOthers(ctx, &api.ThreadAwaitingOthersParams{Limit: new(int64(1)), WaitingDays: new(int64(0))}); err != nil ||
		d.Total != int64(len(want)+1) || !slices.Equal(subjects(d), []string{"To a VIP"}) {
		t.Errorf("awaitingOthers waitingDays 0, limit 1 = %q of %d, %v; want the VIP's of %d", subjects(d), d.Total, err, len(want)+1)
	}
}

// TestAwaitingParams: each param at the ends of its range and past each.
func TestAwaitingParams(t *testing.T) {
	srv, _, _ := waitServer(t, map[string]waitThread{})
	ctx := t.Context()
	c := srv.Dial(t)
	for _, r := range []struct {
		name   string
		lo, hi int64
		set    func(v int64) (*api.ThreadAwaitingReplyParams, *api.ThreadAwaitingOthersParams)
	}{
		{"limit", 1, 25, func(v int64) (*api.ThreadAwaitingReplyParams, *api.ThreadAwaitingOthersParams) {
			return &api.ThreadAwaitingReplyParams{Limit: &v}, &api.ThreadAwaitingOthersParams{Limit: &v}
		}},
		{"waitingDays", 0, 30, func(v int64) (*api.ThreadAwaitingReplyParams, *api.ThreadAwaitingOthersParams) {
			return &api.ThreadAwaitingReplyParams{WaitingDays: &v}, &api.ThreadAwaitingOthersParams{WaitingDays: &v}
		}},
		{"withinDays", 1, 90, func(v int64) (*api.ThreadAwaitingReplyParams, *api.ThreadAwaitingOthersParams) {
			return &api.ThreadAwaitingReplyParams{WithinDays: &v}, &api.ThreadAwaitingOthersParams{WithinDays: &v}
		}},
	} {
		for v, ok := range map[int64]bool{r.lo: true, r.hi: true, r.lo - 1: false, r.hi + 1: false} {
			reply, others := r.set(v)
			_, err1 := c.Thread().AwaitingReply(ctx, reply)
			_, err2 := c.Thread().AwaitingOthers(ctx, others)
			for _, err := range []error{err1, err2} {
				if ok && err != nil || !ok && !errors.Is(err, api.ErrInvalidParams) {
					t.Errorf("%s %d = %v, want ok %v", r.name, v, err, ok)
				}
			}
		}
	}
}
