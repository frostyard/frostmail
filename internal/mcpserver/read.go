package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/frostyard/frostmail/api"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The read tools adapt maild's account, mailbox, view, thread and message
// domains (docs/specs/lodge-app.md, /mcp).

// Limits on what the tools return.
const (
	maxSearch  = 50
	maxThread  = 20
	maxText    = 32 << 10
	defaultHit = 20
)

// The tools' inputs and outputs. The SDK infers each tool's JSON schemas
// from them: a field without omitzero is required, and a jsonschema tag is
// its description.

type listIn struct{}

type mailboxOut struct {
	ID     int64  `json:"id"`
	Path   string `json:"path"`
	Role   string `json:"role" jsonschema:"inbox, drafts, sent, archive, junk, trash, all, flagged, or none for a user's folder"`
	Total  int64  `json:"total"`
	Unread int64  `json:"unread"`
}

type accountOut struct {
	ID        int64        `json:"id"`
	Email     string       `json:"email"`
	Name      string       `json:"name"`
	Mailboxes []mailboxOut `json:"mailboxes"`
}

type listOut struct {
	Accounts []accountOut `json:"accounts"`
	Notice   string       `json:"notice"`
}

type searchIn struct {
	Query     string `json:"query,omitzero" jsonschema:"what to find, in the search field's language; empty for every message"`
	MailboxID int64  `json:"mailbox_id,omitzero" jsonschema:"only this mailbox, by its ID from list_mailboxes"`
	Limit     int    `json:"limit,omitzero" jsonschema:"at most this many messages, 1 to 50; default 20"`
}

type hitOut struct {
	ID             int64  `json:"id"`
	ThreadID       int64  `json:"thread_id" jsonschema:"0 for a message in no thread"`
	AccountID      int64  `json:"account_id"`
	Date           string `json:"date" jsonschema:"RFC 3339"`
	From           string `json:"from"`
	Subject        string `json:"subject"`
	Preview        string `json:"preview" jsonschema:"the start of the message's text"`
	Unread         bool   `json:"unread"`
	Flagged        bool   `json:"flagged"`
	HasAttachments bool   `json:"has_attachments"`
}

type searchOut struct {
	Total    int64    `json:"total" jsonschema:"how many messages match"`
	Messages []hitOut `json:"messages"`
	Notice   string   `json:"notice"`
}

type threadIn struct {
	MessageID int64 `json:"message_id" jsonschema:"any message of the conversation, by its ID from search_messages"`
}

type attachmentOut struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

type messageOut struct {
	ID          int64           `json:"id"`
	Date        string          `json:"date" jsonschema:"RFC 3339"`
	From        string          `json:"from"`
	To          []string        `json:"to"`
	Cc          []string        `json:"cc"`
	Subject     string          `json:"subject"`
	Text        string          `json:"text" jsonschema:"the message's text, or text made from its HTML"`
	Truncated   bool            `json:"truncated" jsonschema:"the text was cut at 32 KiB"`
	Attachments []attachmentOut `json:"attachments"`
}

type threadOut struct {
	ThreadID int64        `json:"thread_id"`
	Count    int          `json:"count" jsonschema:"how many messages the conversation has"`
	Messages []messageOut `json:"messages" jsonschema:"oldest first; the newest 20 when there are more"`
	Notice   string       `json:"notice"`
}

func (t *tools) listMailboxes(ctx context.Context, _ *sdk.CallToolRequest, _ listIn) (*sdk.CallToolResult, listOut, error) {
	c := t.o.Client
	accounts, err := c.Account().List(ctx, &api.AccountListParams{})
	if err != nil {
		return nil, listOut{}, err
	}
	mbs, err := c.Mailbox().List(ctx, &api.MailboxListParams{})
	if err != nil {
		return nil, listOut{}, err
	}
	out := listOut{Accounts: []accountOut{}, Notice: Notice}
	for _, a := range accounts {
		acct := accountOut{ID: a.ID, Email: a.Email, Name: a.DisplayName, Mailboxes: []mailboxOut{}}
		for _, mb := range mbs {
			if mb.AccountID == a.ID {
				acct.Mailboxes = append(acct.Mailboxes, mailboxOut{ID: mb.ID, Path: mb.Path, Role: string(mb.Role),
					Total: mb.Total, Unread: mb.Unread})
			}
		}
		out.Accounts = append(out.Accounts, acct)
	}
	return nil, out, nil
}

func (t *tools) searchMessages(ctx context.Context, _ *sdk.CallToolRequest, in searchIn) (*sdk.CallToolResult, searchOut, error) {
	if in.Limit == 0 {
		in.Limit = defaultHit
	}
	if in.Limit < 1 || in.Limit > maxSearch {
		return nil, searchOut{}, fmt.Errorf("limit must be between 1 and %d", maxSearch)
	}
	q := api.ViewQuery{}
	if in.Query != "" {
		q.Text = &in.Query
	}
	if in.MailboxID != 0 {
		q.MailboxID = &in.MailboxID
	}
	c := t.o.Client
	v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: q})
	if err != nil {
		return nil, searchOut{}, err
	}
	defer func() { _ = c.View().Close(context.WithoutCancel(ctx), &api.ViewCloseParams{ID: v.ID}) }()
	rows, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: min(v.Count, int64(in.Limit))})
	if err != nil {
		return nil, searchOut{}, err
	}
	out := searchOut{Total: v.Count, Messages: []hitOut{}, Notice: Notice}
	for _, r := range rows {
		out.Messages = append(out.Messages, hitOut{
			ID: r.ID, ThreadID: r.ThreadID, AccountID: r.AccountID, Date: r.Date.UTC().Format(time.RFC3339),
			From: address(r.From), Subject: r.Subject, Preview: r.Preview, Unread: !r.Flags.Seen,
			Flagged: r.Flags.Flagged, HasAttachments: r.HasAttachments,
		})
	}
	return nil, out, nil
}

func (t *tools) readThread(ctx context.Context, _ *sdk.CallToolRequest, in threadIn) (*sdk.CallToolResult, threadOut, error) {
	c := t.o.Client
	sums, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{in.MessageID}})
	if err != nil {
		return nil, threadOut{}, err
	}
	if len(sums) == 0 {
		return nil, threadOut{}, fmt.Errorf("message %d does not exist", in.MessageID)
	}
	rows := sums
	if sums[0].ThreadID != 0 {
		if rows, err = c.Thread().Messages(ctx, &api.ThreadMessagesParams{ID: sums[0].ThreadID}); err != nil {
			return nil, threadOut{}, err
		}
	}
	out := threadOut{ThreadID: sums[0].ThreadID, Count: len(rows), Messages: []messageOut{}, Notice: Notice}
	for _, r := range rows[max(0, len(rows)-maxThread):] {
		m, err := t.message(ctx, r)
		if err != nil {
			return nil, threadOut{}, err
		}
		out.Messages = append(out.Messages, m)
	}
	return nil, out, nil
}

// message reads one message of a thread: its headers, parts and text.
func (t *tools) message(ctx context.Context, r api.MessageSummary) (messageOut, error) {
	c := t.o.Client
	m, err := c.Message().Get(ctx, &api.MessageGetParams{ID: r.ID})
	if err != nil {
		return messageOut{}, err
	}
	out := messageOut{ID: r.ID, Date: r.Date.UTC().Format(time.RFC3339), From: address(r.From), Subject: r.Subject,
		To: addresses(m.To), Cc: addresses(m.Cc), Attachments: []attachmentOut{}}
	for _, p := range m.Parts {
		if p.Filename != "" {
			out.Attachments = append(out.Attachments, attachmentOut{Filename: p.Filename, ContentType: p.ContentType, Size: p.Size})
		}
	}
	body, err := c.Message().Body(ctx, &api.MessageBodyParams{ID: r.ID})
	switch {
	case errors.Is(err, api.ErrUnavailable):
		out.Text = "(This message's text is not stored, and its server cannot be reached now.)"
	case err != nil:
		return messageOut{}, err
	default:
		out.Text, out.Truncated = cut(body.Text, maxText)
	}
	return out, nil
}

// cut shortens s to at most n bytes, on a rune boundary.
func cut(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n], true
}

func address(a api.Address) string {
	if a.Name == "" {
		return a.Address
	}
	return fmt.Sprintf("%s <%s>", a.Name, a.Address)
}

func addresses(list []api.Address) []string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, address(a))
	}
	return out
}
