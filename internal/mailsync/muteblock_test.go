package mailsync_test

import (
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/notify"
)

// TestMutedAndBlockedNeverNotify: a muted conversation's new mail and a
// blocked sender's mail are not announced, not even by a rule's Send
// Notification; with blockAction mark, the blocked mail stays in the inbox
// (ADR-0028, parity P-410 and P-411).
func TestMutedAndBlockedNeverNotify(t *testing.T) {
	r := newRulesHarness(t)
	ctx := t.Context()
	r.deliver("Ann <ann@x.test>", "Plans")
	r.settledWith("Plans")
	if err := r.c.Message().Mute(ctx, &api.MessageMuteParams{IDs: []int64{r.summary("Plans").ID}, Muted: true}); err != nil {
		t.Fatal(err)
	}
	if m := r.summary("Plans").Muted; m == nil || !*m {
		t.Fatalf("Plans muted = %v, want true", m)
	}
	r.rule("Ann notifies", cond(api.ConditionFieldFrom, api.ConditionOpContains, "ann@"), api.RuleAction{Kind: api.RuleActionKindNotify})
	if _, err := r.c.Block().Add(ctx, &api.BlockAddParams{Addresses: []string{"Spam@Y.test"}}); err != nil {
		t.Fatal(err)
	}

	reply := "From: Ann <ann@x.test>\r\nSubject: Re: Plans\r\nMessage-ID: <re.plans@x.test>\r\n" +
		"In-Reply-To: <Plans@x.test>\r\n\r\nYes.\r\n"
	if err := r.appendRaw([]byte(reply)); err != nil {
		t.Fatal(err)
	}
	r.deliver("Spammer <spam@y.test>", "Offer")
	r.deliver("Bob <bob@x.test>", "Lunch")
	all := r.settledWith("Lunch")
	if subjects := subjectsOf(all); !slices.Equal(subjects, []string{"Lunch"}) {
		t.Errorf("announced %v, want only Lunch", subjects)
	}
	if m := r.summary("Re: Plans").Muted; m == nil || !*m {
		t.Errorf("the reply's muted = %v, want true: it joined the muted conversation", m)
	}
	if !slices.Contains(r.summary("Offer").MailboxIDs, r.inbox().ID) {
		t.Error("the blocked sender's mail left the inbox under blockAction mark")
	}

	// Unmuted and unblocked, they notify again.
	if err := r.c.Message().Mute(ctx, &api.MessageMuteParams{IDs: []int64{r.summary("Plans").ID}, Muted: false}); err != nil {
		t.Fatal(err)
	}
	if err := r.c.Block().Remove(ctx, &api.BlockRemoveParams{Addresses: []string{"spam@y.test"}}); err != nil {
		t.Fatal(err)
	}
	r.deliver("Spammer <spam@y.test>", "Second offer")
	if subjects := subjectsOf(r.settledWith("Second offer")); !slices.Equal(subjects, []string{"Second offer"}) {
		t.Errorf("announced %v after unblocking, want Second offer", subjects)
	}
	if m := r.summary("Re: Plans").Muted; m != nil {
		t.Errorf("after unmuting, muted = %v, want absent", *m)
	}
}

// TestBlockedMailToTrash: with blockAction trash, new inbox mail from a
// blocked sender moves to the Trash before any rule sees it.
func TestBlockedMailToTrash(t *testing.T) {
	r := newRulesHarness(t)
	ctx := t.Context()
	trash := api.BlockActionTrash
	if _, err := r.c.Settings().Set(ctx, &api.SettingsSetParams{BlockAction: &trash}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.c.Block().Add(ctx, &api.BlockAddParams{Addresses: []string{"spam@y.test"}}); err != nil {
		t.Fatal(err)
	}
	r.rule("Flag y.test", cond(api.ConditionFieldFrom, api.ConditionOpContains, "y.test"),
		api.RuleAction{Kind: api.RuleActionKindFlag, Color: ptr(int64(1))})
	r.deliver("Spammer <spam@y.test>", "Offer")
	r.deliver("Bob <bob@x.test>", "Lunch")
	r.settledWith("Lunch")
	trashBox := r.mailboxByPath("Trash")
	waitUntil(t, 5*time.Second, "Offer in the Trash", func() bool {
		s, ok := r.find("Offer")
		return ok && slices.Equal(s.MailboxIDs, []int64{trashBox.ID})
	})
	if r.flagged("Offer") {
		t.Error("a rule acted on the blocked sender's mail")
	}
	r.waitReplayed()
	if err := r.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: r.acct}); err != nil {
		t.Fatal(err)
	}
	r.waitPhase(api.SyncPhaseIdle)
	if s := r.summary("Offer"); !slices.Equal(s.MailboxIDs, []int64{trashBox.ID}) {
		t.Errorf("after the server's next pass Offer is in %v, want the Trash %d", s.MailboxIDs, trashBox.ID)
	}
}

func subjectsOf(mail []notify.Mail) []string {
	out := make([]string, 0, len(mail))
	for _, m := range mail {
		out = append(out, m.Subject)
	}
	return out
}
