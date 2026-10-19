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
// a token whose scope grants it: ;write sets flags, moves messages, drafts
// replies, creates and completes tasks and sets Remind Me (ADR-0042);
// ;send queues a draft to send. maild's router and engine enforce the
// scope whatever the tools offer. Nothing here writes an event.

// Limits on what the write tools take.
const (
	maxIDs       = 100 // messages one call changes
	maxTaskTitle = 1024
	maxTaskNotes = 32 << 10
	maxRemind    = 365 * 24 * time.Hour
)

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

type createTaskIn struct {
	Title  string `json:"title" jsonschema:"what to do, 1 to 1,024 bytes"`
	Notes  string `json:"notes,omitzero" jsonschema:"more about it, at most 32 KiB"`
	Due    string `json:"due,omitzero" jsonschema:"the day it is due, YYYY-MM-DD"`
	ListID int64  `json:"list_id,omitzero" jsonschema:"the task list, by its ID from list_collections; default the first account's default list"`
}

type taskWriteOut struct {
	TaskID    int64  `json:"task_id"`
	ListID    int64  `json:"list_id"`
	List      string `json:"list"`
	AccountID int64  `json:"account_id"`
	Title     string `json:"title"`
	Due       string `json:"due,omitzero"`
	Completed bool   `json:"completed"`
	Note      string `json:"note"`
}

type completeTaskIn struct {
	TaskID    int64 `json:"task_id" jsonschema:"the task, by its ID from list_tasks"`
	Completed *bool `json:"completed,omitzero" jsonschema:"true (the default) completes it; false reopens it"`
}

type remindIn struct {
	MessageID int64  `json:"message_id" jsonschema:"the message, by its ID from search_messages"`
	At        string `json:"at,omitzero" jsonschema:"when, RFC 3339 with its offset, after now and at most 365 days ahead; omit to clear the reminder"`
}

type remindOut struct {
	MessageID       int64  `json:"message_id"`
	MessageIDHeader string `json:"message_id_header,omitzero" jsonschema:"the message's Message-ID header, without angle brackets; absent when it has none"`
	URL             string `json:"url,omitzero" jsonschema:"the message's permanent lodge URL, to link it or for lodge.open; use it as it is"`
	At              string `json:"at,omitzero" jsonschema:"when it comes back, RFC 3339"`
	Note            string `json:"note"`
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
		sdk.AddTool(s, &sdk.Tool{Name: "create_task", Annotations: writes, Description: `Add a task to the user's
Tasks, at the top of a list: by default the default list of the first
account this token reaches. It is written to the list's server.`}, t.createTask)
		sdk.AddTool(s, &sdk.Tool{Name: "complete_task", Annotations: writes, Description: `Complete a task, or
reopen it with completed false. The change reaches the task's server.`}, t.completeTask)
		sdk.AddTool(s, &sdk.Tool{Name: "remind_me", Annotations: writes, Description: `Remind Me: at a time, bring
a message back to the top of its inbox and notify the user; without at,
clear its reminder.`}, t.remindMe)
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

func (t *tools) createTask(ctx context.Context, _ *sdk.CallToolRequest, in createTaskIn) (*sdk.CallToolResult, taskWriteOut, error) {
	if strings.TrimSpace(in.Title) == "" || len(in.Title) > maxTaskTitle {
		return nil, taskWriteOut{}, fmt.Errorf("title must be 1 to %d bytes", maxTaskTitle)
	}
	if len(in.Notes) > maxTaskNotes {
		return nil, taskWriteOut{}, fmt.Errorf("notes must be at most %d bytes", maxTaskNotes)
	}
	p := &api.TasksCreateParams{Title: in.Title}
	if in.Notes != "" {
		p.Notes = &in.Notes
	}
	if in.Due != "" {
		p.Due = &in.Due
	}
	if in.ListID != 0 {
		p.ListID = &in.ListID
	}
	task, err := t.o.Client.Tasks().Create(ctx, p)
	if err != nil {
		return nil, taskWriteOut{}, err
	}
	return t.taskWritten(ctx, task, "The task is in the user's Tasks; it is written to its list's server.")
}

func (t *tools) completeTask(ctx context.Context, _ *sdk.CallToolRequest, in completeTaskIn) (*sdk.CallToolResult, taskWriteOut, error) {
	done := in.Completed == nil || *in.Completed
	task, err := t.o.Client.Tasks().Update(ctx, &api.TasksUpdateParams{ID: in.TaskID, Completed: &done})
	if err != nil {
		return nil, taskWriteOut{}, err
	}
	note := "The task is completed."
	if !done {
		note = "The task is open again."
	}
	return t.taskWritten(ctx, task, note)
}

func (t *tools) taskWritten(ctx context.Context, task *api.Task, note string) (*sdk.CallToolResult, taskWriteOut, error) {
	names, err := t.collectionNames(ctx, api.CollectionKindTasklist)
	if err != nil {
		return nil, taskWriteOut{}, err
	}
	return nil, taskWriteOut{TaskID: task.ID, ListID: task.ListID, List: names[task.ListID], AccountID: task.AccountID,
		Title: task.Title, Due: task.Due, Completed: task.Completed, Note: note}, nil
}

func (t *tools) remindMe(ctx context.Context, _ *sdk.CallToolRequest, in remindIn) (*sdk.CallToolResult, remindOut, error) {
	p := &api.MessageRemindParams{IDs: []int64{in.MessageID}}
	out := remindOut{MessageID: in.MessageID, Note: "The reminder is cleared."}
	if in.At != "" {
		at, err := time.Parse(time.RFC3339, in.At)
		if err != nil {
			return nil, remindOut{}, fmt.Errorf("at must be RFC 3339 with its offset, such as 2026-10-23T09:00:00+02:00")
		}
		now := t.now()
		if !at.After(now) || at.After(now.Add(maxRemind)) {
			return nil, remindOut{}, fmt.Errorf("at must be after now and at most 365 days ahead")
		}
		p.At, out.At = &at, at.Format(time.RFC3339)
		out.Note = "At that time the message comes back to the top of its inbox, and the user is notified."
	}
	if err := t.o.Client.Message().Remind(ctx, p); err != nil {
		return nil, remindOut{}, err
	}
	headers, err := t.messageIDHeaders(ctx, p.IDs)
	if err != nil {
		return nil, remindOut{}, err
	}
	out.MessageIDHeader, out.URL = messageLink(headers[in.MessageID])
	return nil, out, nil
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
