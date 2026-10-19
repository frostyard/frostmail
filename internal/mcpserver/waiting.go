package mcpserver

import (
	"context"
	"time"

	"github.com/frostyard/frostmail/api"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The waiting tools adapt thread.awaitingReply and thread.awaitingOthers,
// whose answers maild computes by fixed rules (ADR-0042): the model reads
// which threads wait, and does not decide it.

// addWaitingTools adds threads_awaiting_reply and threads_awaiting_others.
func addWaitingTools(s *sdk.Server, t *tools) {
	reads := &sdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)}
	sdk.AddTool(s, &sdk.Tool{Name: "threads_awaiting_reply", Annotations: reads, Description: `List the conversations
waiting on the user: those whose newest message is from someone else, to
the user in To, not from a list or a no-reply address, not answered,
muted or blocked, and still in an inbox. VIPs first, then the newest. Each
has its newest message's ID (for read_thread and draft_reply), sender,
subject, date and start, and how many days it has waited. ` + Notice}, t.awaitingReply)
	sdk.AddTool(s, &sdk.Tool{Name: "threads_awaiting_others", Annotations: reads, Description: `List the conversations
waiting on other people: those whose newest message is the user's own, to
someone else, with no reply since, by default for at least 5 days. Those
sent to a VIP first, then the newest. ` + Notice}, t.awaitingOthers)
}

type replyWaitIn struct {
	Limit       int    `json:"limit,omitzero" jsonschema:"at most this many conversations, 1 to 25; default 5"`
	WaitingDays *int64 `json:"waiting_days,omitzero" jsonschema:"only those waiting at least this many days, 0 to 30; default 0"`
	WithinDays  *int64 `json:"within_days,omitzero" jsonschema:"only those waiting less than this many days, 1 to 90; default 30"`
	TimeZone    string `json:"time_zone,omitzero" jsonschema:"the IANA zone the dates are in; default maild's"`
}

type othersWaitIn struct {
	Limit       int    `json:"limit,omitzero" jsonschema:"at most this many conversations, 1 to 25; default 10"`
	WaitingDays *int64 `json:"waiting_days,omitzero" jsonschema:"only those waiting at least this many days, 0 to 30; default 5"`
	WithinDays  *int64 `json:"within_days,omitzero" jsonschema:"only those waiting less than this many days, 1 to 90; default 30"`
	TimeZone    string `json:"time_zone,omitzero" jsonschema:"the IANA zone the dates are in; default maild's"`
}

type waitingThreadOut struct {
	ThreadID        int64    `json:"thread_id"`
	AccountID       int64    `json:"account_id"`
	MessageID       int64    `json:"message_id" jsonschema:"the newest message, for read_thread and draft_reply"`
	MessageIDHeader string   `json:"message_id_header,omitzero" jsonschema:"the newest message's Message-ID header, without angle brackets; absent when it has none"`
	URL             string   `json:"url,omitzero" jsonschema:"the newest message's permanent lodge URL, to link it or for lodge.open; use it as it is"`
	Subject         string   `json:"subject"`
	From            string   `json:"from,omitzero" jsonschema:"who the conversation waits for a reply to"`
	To              []string `json:"to,omitzero" jsonschema:"the newest message's first 10 recipients"`
	ToCount         int64    `json:"to_count,omitzero" jsonschema:"how many recipients it has"`
	Date            string   `json:"date" jsonschema:"the newest message's, RFC 3339 in the answer's zone"`
	Preview         string   `json:"preview" jsonschema:"the start of its text"`
	Unread          bool     `json:"unread"`
	Messages        int64    `json:"messages" jsonschema:"how many messages the conversation has"`
	VIP             bool     `json:"vip" jsonschema:"the sender, or a recipient, is a VIP"`
	WaitingDays     int64    `json:"waiting_days" jsonschema:"whole days since the newest message"`
}

type waitingOut struct {
	TimeZone string             `json:"time_zone"`
	Total    int64              `json:"total" jsonschema:"how many conversations wait, before limit"`
	Threads  []waitingThreadOut `json:"threads"`
	Notice   string             `json:"notice"`
}

func optionalLimit(limit int) *int64 {
	if limit == 0 {
		return nil
	}
	return new(int64(limit))
}

func (t *tools) awaitingReply(ctx context.Context, _ *sdk.CallToolRequest, in replyWaitIn) (*sdk.CallToolResult, waitingOut, error) {
	loc, zone, err := zoneOf(in.TimeZone)
	if err != nil {
		return nil, waitingOut{}, err
	}
	got, err := t.o.Client.Thread().AwaitingReply(ctx, &api.ThreadAwaitingReplyParams{Limit: optionalLimit(in.Limit),
		WaitingDays: in.WaitingDays, WithinDays: in.WithinDays})
	if err != nil {
		return nil, waitingOut{}, err
	}
	return t.waiting(ctx, got, loc, zone, false)
}

func (t *tools) awaitingOthers(ctx context.Context, _ *sdk.CallToolRequest, in othersWaitIn) (*sdk.CallToolResult, waitingOut, error) {
	loc, zone, err := zoneOf(in.TimeZone)
	if err != nil {
		return nil, waitingOut{}, err
	}
	got, err := t.o.Client.Thread().AwaitingOthers(ctx, &api.ThreadAwaitingOthersParams{Limit: optionalLimit(in.Limit),
		WaitingDays: in.WaitingDays, WithinDays: in.WithinDays})
	if err != nil {
		return nil, waitingOut{}, err
	}
	return t.waiting(ctx, got, loc, zone, true)
}

func (t *tools) waiting(ctx context.Context, a *api.Awaiting, loc *time.Location, zone string, others bool) (*sdk.CallToolResult, waitingOut, error) {
	ids := make([]int64, 0, len(a.Threads))
	for _, w := range a.Threads {
		ids = append(ids, w.Message.ID)
	}
	headers, err := t.messageIDHeaders(ctx, ids)
	if err != nil {
		return nil, waitingOut{}, err
	}
	out := waitingOut{TimeZone: zone, Total: a.Total, Threads: []waitingThreadOut{}, Notice: Notice}
	for _, w := range a.Threads {
		m := w.Message
		o := waitingThreadOut{ThreadID: w.ThreadID, AccountID: w.AccountID, MessageID: m.ID, Subject: m.Subject,
			Date: m.Date.In(loc).Format(time.RFC3339), Preview: m.Preview, Unread: !m.Flags.Seen, Messages: m.ThreadCount,
			VIP: w.Vip, WaitingDays: w.WaitingDays}
		o.MessageIDHeader, o.URL = messageLink(headers[m.ID])
		if others {
			o.To, o.ToCount = addresses(w.Recipients), w.RecipientCount
		} else {
			o.From = address(m.From)
		}
		out.Threads = append(out.Threads, o)
	}
	return nil, out, nil
}
