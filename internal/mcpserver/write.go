package mcpserver

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The write and send tools (docs/specs/lodge-app.md, /mcp), each only for
// a token whose scope grants it: ;write sets flags, moves messages and
// drafts replies; ;send queues a draft to send. maild's router and engine
// enforce the scope whatever the tools offer.

// maxIDs bounds the messages one call changes.
const maxIDs = 100

type flagsIn struct {
	MessageIDs []int64 `json:"message_ids" jsonschema:"the messages, by their IDs from search_messages; at most 100"`
	Read       *bool   `json:"read,omitzero" jsonschema:"mark them read (true) or unread (false); omit to leave it"`
	Flagged    *bool   `json:"flagged,omitzero" jsonschema:"flag them (true) or clear their flags (false); omit to leave it"`
}

type moveIn struct {
	MessageIDs []int64 `json:"message_ids" jsonschema:"the messages, by their IDs from search_messages; at most 100"`
	MailboxID  int64   `json:"mailbox_id" jsonschema:"where to, by its ID from list_mailboxes, in the messages' account"`
}

type doneOut struct {
	Changed int `json:"changed" jsonschema:"how many messages changed"`
}

type replyIn struct {
	MessageID int64  `json:"message_id" jsonschema:"the message to reply to, by its ID from search_messages"`
	Text      string `json:"text" jsonschema:"the reply's text, above the quoted message; a blank line starts a paragraph"`
	ReplyAll  bool   `json:"reply_all,omitzero" jsonschema:"reply to every recipient, not only the sender"`
}

type draftOut struct {
	DraftID   int64    `json:"draft_id"`
	AccountID int64    `json:"account_id"`
	To        []string `json:"to"`
	Cc        []string `json:"cc"`
	Subject   string   `json:"subject"`
	Note      string   `json:"note"`
}

type sendIn struct {
	DraftID int64 `json:"draft_id" jsonschema:"the draft, by its ID from draft_reply"`
}

type sendOut struct {
	OutboxID int64    `json:"outbox_id"`
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	SendAt   string   `json:"send_at,omitzero" jsonschema:"RFC 3339: when it goes out, after the user's undo delay"`
	Note     string   `json:"note"`
}

// addWriteTools adds the tools the scope grants.
func addWriteTools(s *sdk.Server, t *tools, scope api.Scope) {
	if scope.Write {
		writes := &sdk.ToolAnnotations{DestructiveHint: new(false), OpenWorldHint: new(false)}
		sdk.AddTool(s, &sdk.Tool{Name: "set_flags", Annotations: writes, Description: `Mark messages read or
unread, and flag them or clear their flags. The change reaches the mail
server as the user's own would.`}, t.setFlags)
		sdk.AddTool(s, &sdk.Tool{Name: "move_messages", Annotations: writes, Description: `Move messages to
another mailbox of their account, such as its archive or its trash, by the
mailbox's ID from list_mailboxes.`}, t.moveMessages)
		sdk.AddTool(s, &sdk.Tool{Name: "draft_reply", Annotations: writes, Description: `Draft a reply to a
message: text above the quoted message, to its sender (or with reply_all
to every recipient), from the account that received it. The draft waits
in the user's Drafts for them to read, change and send; this does not
send it.`}, t.draftReply)
	}
	if scope.Send {
		sends := &sdk.ToolAnnotations{DestructiveHint: new(true), OpenWorldHint: new(true)}
		sdk.AddTool(s, &sdk.Tool{Name: "send_draft", Annotations: sends, Description: `Send a draft, by its ID
from draft_reply: it goes out after the user's undo delay, during which
they can still take it back in Frostmail. Send only what the user asked to
send.`}, t.sendDraft)
	}
}

func checkIDs(ids []int64) error {
	if len(ids) == 0 || len(ids) > maxIDs {
		return fmt.Errorf("message_ids must name 1 to %d messages", maxIDs)
	}
	return nil
}

func (t *tools) setFlags(ctx context.Context, _ *sdk.CallToolRequest, in flagsIn) (*sdk.CallToolResult, doneOut, error) {
	if err := checkIDs(in.MessageIDs); err != nil {
		return nil, doneOut{}, err
	}
	if in.Read == nil && in.Flagged == nil {
		return nil, doneOut{}, fmt.Errorf("say read, flagged or both")
	}
	err := t.o.Client.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: in.MessageIDs,
		Changes: api.FlagChanges{Seen: in.Read, Flagged: in.Flagged}})
	if err != nil {
		return nil, doneOut{}, err
	}
	return nil, doneOut{Changed: len(in.MessageIDs)}, nil
}

func (t *tools) moveMessages(ctx context.Context, _ *sdk.CallToolRequest, in moveIn) (*sdk.CallToolResult, doneOut, error) {
	if err := checkIDs(in.MessageIDs); err != nil {
		return nil, doneOut{}, err
	}
	if err := t.o.Client.Message().Move(ctx, &api.MessageMoveParams{IDs: in.MessageIDs, MailboxID: in.MailboxID}); err != nil {
		return nil, doneOut{}, err
	}
	return nil, doneOut{Changed: len(in.MessageIDs)}, nil
}

func (t *tools) draftReply(ctx context.Context, _ *sdk.CallToolRequest, in replyIn) (*sdk.CallToolResult, draftOut, error) {
	if strings.TrimSpace(in.Text) == "" {
		return nil, draftOut{}, fmt.Errorf("text is empty")
	}
	kind := api.DraftKindReply
	if in.ReplyAll {
		kind = api.DraftKindReplyall
	}
	c := t.o.Client
	dr, err := c.Draft().Create(ctx, &api.DraftCreateParams{Kind: kind, SourceID: &in.MessageID})
	if err != nil {
		return nil, draftOut{}, err
	}
	content := dr.Content
	content.HTML = replyHTML(in.Text, content.HTML)
	if dr, err = c.Draft().Update(ctx, &api.DraftUpdateParams{ID: dr.ID, Content: content}); err != nil {
		return nil, draftOut{}, err
	}
	return nil, draftOut{DraftID: dr.ID, AccountID: dr.AccountID, To: addresses(dr.Content.To), Cc: addresses(dr.Content.Cc),
		Subject: dr.Content.Subject, Note: "The draft is in the user's Drafts; nothing was sent."}, nil
}

// replyHTML puts text, as paragraphs, on the reply's empty first line,
// above its signature and the quote.
func replyHTML(text, body string) string {
	var b strings.Builder
	for p := range strings.SplitSeq(strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n"), "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			b.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(p), "\n", "<br>") + "</p>")
		}
	}
	const cursor = "<p><br></p>"
	if rest, ok := strings.CutPrefix(body, cursor); ok {
		return b.String() + cursor + rest
	}
	return b.String() + body
}

func (t *tools) sendDraft(ctx context.Context, _ *sdk.CallToolRequest, in sendIn) (*sdk.CallToolResult, sendOut, error) {
	item, err := t.o.Client.Draft().Send(ctx, &api.DraftSendParams{ID: in.DraftID})
	if err != nil {
		return nil, sendOut{}, err
	}
	out := sendOut{OutboxID: item.ID, To: addresses(item.To), Subject: item.Subject,
		Note: "Queued: it goes out after the user's undo delay, unless they take it back."}
	if item.SendAt != nil {
		out.SendAt = item.SendAt.UTC().Format(time.RFC3339)
	}
	return nil, out, nil
}
