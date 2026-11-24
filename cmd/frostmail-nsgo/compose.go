package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	oldcompose "github.com/frostyard/frostmail/internal/desktop/compose"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/richeditor"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	rich "github.com/nsgo-dev/nsgo/ui/richeditor"
)

func (w *workspace) newDraft(kind api.DraftKind, source *int64) {
	p := api.DraftCreateParams{Kind: kind, SourceID: source}
	if source == nil && w.source.Kind == mail.Mailbox {
		for _, b := range w.store.State().Mailboxes {
			if b.ID == w.source.ID {
				p.AccountID = &b.AccountID
				break
			}
		}
	}
	perform(w, func(ctx context.Context, c *api.Client) (*api.Draft, error) {
		return c.Draft().Create(ctx, &p)
	}, w.editDraft)
}
func (w *workspace) editDraft(d *api.Draft, err error) {
	if err != nil {
		w.problem.Set(err.Error())
		return
	}
	w.show(func(cx *core.Scope) core.Node { return w.compose(cx, d) })
	w.routePanel(fmt.Sprintf("/compose/%d", d.ID))
}
func addressLine(list []api.Address) string {
	tokens := make([]string, 0, len(list))
	for _, a := range list {
		tokens = append(tokens, oldcompose.Token(a))
	}
	return strings.Join(tokens, ", ")
}

type draftEditor struct {
	w              *workspace
	cx             *core.Scope
	id             int64
	content        api.DraftContent
	to, cc, bcc    string
	editor         *richeditor.Session
	status         *core.Signal[string]
	busy           *core.Signal[bool]
	files          *core.Signal[[]api.DraftAttachment]
	changed, saved int
	saving         bool
	afterSave      func()
	onFailure      func()
	timer          func() bool
}

func (e *draftEditor) edit() {
	e.changed++
	e.status.Set("Unsaved changes")
	if e.timer != nil {
		e.timer()
	}
	e.timer = e.cx.AfterFunc(650*time.Millisecond, e.save)
}
func (e *draftEditor) flush(f func()) {
	if e.busy.GetUntracked() {
		return
	}
	e.busy.Set(true)
	e.afterSave = f
	e.save()
}
func (e *draftEditor) fail(err error) {
	e.busy.Set(false)
	e.afterSave = nil
	if report := e.onFailure; report != nil {
		e.onFailure = nil
		report()
	}
	e.status.Set(err.Error())
}
func (e *draftEditor) save() {
	if e.saving {
		return
	}
	if e.changed == e.saved {
		if next := e.afterSave; next != nil {
			e.afterSave = nil
			next()
		}
		return
	}
	c := e.content
	for _, v := range []struct {
		text string
		dest *[]api.Address
	}{{e.to, &c.To}, {e.cc, &c.Cc}, {e.bcc, &c.Bcc}} {
		addresses, invalid := oldcompose.ParseAddresses(v.text)
		if len(invalid) > 0 {
			e.fail(fmt.Errorf("Check the address %q", invalid[0]))
			return
		}
		*v.dest = addresses
	}
	revision := e.changed
	e.saving = true
	e.status.Set("Saving…")
	perform(e.w, func(ctx context.Context, client *api.Client) (*api.Draft, error) {
		return client.Draft().Update(ctx, &api.DraftUpdateParams{ID: e.id, Content: c})
	}, func(_ *api.Draft, err error) {
		e.saving = false
		if err != nil {
			e.fail(err)
			return
		}
		e.saved = revision
		e.status.Set("Saved")
		e.save()
	})
}

func (w *workspace) compose(cx *core.Scope, d *api.Draft) core.Node {
	e := &draftEditor{w: w, cx: cx, id: d.ID, content: d.Content, to: addressLine(d.Content.To), cc: addressLine(d.Content.Cc), bcc: addressLine(d.Content.Bcc), status: core.NewSignal(cx, "Saved"), busy: core.NewSignal(cx, false), files: core.NewSignalFunc(cx, d.Attachments, func(_, _ []api.DraftAttachment) bool { return false })}
	var err error
	e.editor, err = richeditor.NewSession(richeditor.SessionConfig{Extensions: append(richeditor.StarterKit(richeditor.StarterOptions{}), richeditor.TableKit(richeditor.TableOptions{})), HTML: d.Content.HTML})
	if err != nil {
		return w.dialog("Open draft", []core.Node{core.Text{Content: err.Error()}})
	}
	stop := e.editor.Subscribe(func(_ *richeditor.State, trs []*richeditor.Transaction) {
		for _, tr := range trs {
			if tr.DocChanged() {
				e.content.HTML = e.editor.HTML()
				e.edit()
				break
			}
		}
	})
	cx.Cleanup(stop)
	w.beforeRoute = func(goTo, stay func()) {
		if e.busy.GetUntracked() {
			stay()
			return
		}
		e.onFailure = stay
		e.flush(func() { e.onFailure = nil; goTo() })
	}
	cx.Cleanup(func() { w.beforeRoute = nil })
	// Keep the window open while its only copy of a local edit is unsaved.
	if window := cx.CurrentWindow(); window != nil {
		window.OnClosing(cx, func(event *core.WindowClosing) {
			if e.changed != e.saved || e.busy.GetUntracked() {
				event.Cancel()
				e.save()
			}
		})
	}
	cx.OnQuitRequest(func(q *core.QuitRequest) {
		if e.changed != e.saved || e.busy.GetUntracked() {
			q.Cancel()
			e.save()
		}
	})
	field := func(label string, p *string) core.Node {
		return ui.Input{Label: label, Placeholder: label, DefaultValue: *p, Disabled: e.busy.Get, OnChange: func(v string) { *p = v; e.edit() }}
	}
	identity := core.NewSignalFunc(cx, []api.Identity(nil), func(_, _ []api.Identity) bool { return false })
	perform(w, func(ctx context.Context, c *api.Client) ([]api.Identity, error) {
		return c.Identity().List(ctx, &api.IdentityListParams{AccountID: &d.AccountID})
	}, func(v []api.Identity, err error) {
		if err == nil {
			cx.Post(func() { identity.Set(v) })
		}
	})
	who := core.NewSignal(cx, d.Content.IdentityID)
	from := core.Dynamic(func(*core.Scope) core.Node {
		label := "From"
		var items []core.Node
		for _, i := range identity.Get() {
			if i.ID == who.Get() {
				label = "From: " + i.Email
			}
			items = append(items, ui.MenuItem{Text: i.Email, OnSelect: func() { who.Set(i.ID); e.content.IdentityID = i.ID; e.edit() }})
		}
		return ui.Menu{Trigger: ui.Button{Text: label, Variant: ui.ButtonSecondary}, Children: items}
	})
	var marks []core.Node
	for _, m := range []struct{ label, cmd string }{{"Bold", "toggleBold"}, {"Italic", "toggleItalic"}, {"Bullet list", "toggleBulletList"}, {"Undo", "undo"}, {"Redo", "redo"}} {
		marks = append(marks, ui.Button{Text: m.label, Variant: ui.ButtonGhost, OnPress: func() {
			if !e.busy.GetUntracked() {
				e.editor.RunNamed(m.cmd, nil)
				e.editor.Focus()
			}
		}})
	}
	// A draft can include quoted mail. Do not let its image URLs fetch remote
	// resources when a composer opens. Preserve the node in the saved document.
	image := func(_ *core.Scope, p richeditor.NodeProps) core.Node {
		return core.Text{ContentFunc: func() string { return "Image · " + p.Node().AttrStr("alt") }}
	}
	fileList := core.Dynamic(func(*core.Scope) core.Node {
		var rows []core.Node
		for _, f := range e.files.Get() {
			rows = append(rows, ui.Button{Text: "Remove " + f.Filename, Variant: ui.ButtonGhost, Disabled: e.busy.Get, OnPress: func() {
				e.attachment(func(ctx context.Context, c *api.Client) error {
					return c.Draft().Detach(ctx, &api.DraftDetachParams{ID: d.ID, AttachmentID: f.ID})
				})
			}})
		}
		return workspaceui.Stack(rows...)
	})
	sendNow := func(when *time.Time) {
		e.flush(func() {
			if e.changed != e.saved {
				e.fail(errors.New("Finish saving the draft before sending"))
				return
			}
			perform(w, func(ctx context.Context, c *api.Client) (*api.OutboxItem, error) {
				return c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID, SendAt: when})
			}, func(sent *api.OutboxItem, err error) {
				if err != nil {
					e.fail(err)
					return
				}
				w.closePanel()
				w.queued(sent)
			})
		})
	}
	remind := core.NewSignal(cx, false)
	var pendingTime *time.Time
	requestSend := func(at *time.Time) {
		var signature string
		for _, i := range identity.GetUntracked() {
			if i.ID == who.GetUntracked() {
				signature = i.SignatureHTML
				break
			}
		}
		if oldcompose.NeedsReminder(e.content.HTML, signature, len(e.files.GetUntracked())) {
			pendingTime = at
			remind.Set(true)
			return
		}
		sendNow(at)
	}
	reminder := core.Dynamic(func(*core.Scope) core.Node {
		if !remind.Get() {
			return nil
		}
		return workspaceui.Card{Children: []core.Node{core.Text{Content: "You mentioned an attachment. Send without one?", Style: ui.TypeLabel},
			workspaceui.Row(button("Keep editing", func() { remind.Set(false) }), button("Send without attachment", func() { remind.Set(false); sendNow(pendingTime) }))}}
	})
	scheduling := core.NewSignal(cx, false)
	schedule := core.Dynamic(func(*core.Scope) core.Node {
		if !scheduling.Get() {
			return nil
		}
		const layout = "2006-01-02 15:04"
		when := oldcompose.TomorrowMorning(time.Now()).Format(layout)
		return workspaceui.Row(stringField("Local date and time", &when, false), button("Schedule send", func() {
			at, err := time.ParseInLocation(layout, when, time.Local)
			if err != nil || !at.After(time.Now()) {
				e.status.Set("Enter a future local date and time (YYYY-MM-DD HH:MM).")
				return
			}
			scheduling.Set(false)
			requestSend(&at)
		}), button("Cancel schedule", func() { scheduling.Set(false) }))
	})
	var times []core.Node
	for _, c := range oldcompose.SendChoices(time.Now()) {
		times = append(times, ui.MenuItem{Text: c.Label, OnSelect: func() { t := c.At; requestSend(&t) }})
	}
	times = append(times, ui.MenuItem{Text: "Choose date and time", OnSelect: func() { scheduling.Set(true) }})
	snips, _ := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.SnippetChanged); return ok }, func() func(context.Context, *api.Client) ([]api.Snippet, error) {
		return func(ctx context.Context, c *api.Client) ([]api.Snippet, error) {
			return c.Snippet().List(ctx, &api.SnippetListParams{})
		}
	})
	insert := core.Dynamic(func(*core.Scope) core.Node {
		items := []core.Node{ui.MenuItem{Text: "Inline image", OnSelect: e.inlineImage}}
		for _, s := range snips.Get() {
			items = append(items, ui.MenuItem{Text: s.Name, OnSelect: func() { e.editor.Paste("", s.HTML); e.editor.Focus() }})
		}
		for _, i := range identity.Get() {
			if i.SignatureHTML != "" {
				items = append(items, ui.MenuItem{Text: "Signature · " + i.Email, OnSelect: func() { e.editor.Paste("", i.SignatureHTML); e.editor.Focus() }})
			}
		}
		return ui.Menu{Trigger: ui.Button{Text: "Insert", Disabled: e.busy.Get}, Children: items}
	})
	closeDraft := func() { e.flush(w.closePanel) }
	// Blocking edits during send keeps the transmitted snapshot identical to
	// the last saved draft.
	editor := core.Dynamic(func(*core.Scope) core.Node {
		if e.busy.Get() {
			return core.Text{Content: "Finishing draft…"}
		}
		return rich.View{Editor: e.editor, Label: "Message text", Placeholder: "Write a message", Style: core.Style{Height: core.Pt(205)}, NodeViews: map[string]richeditor.NodeView{"image": image, "inlineImage": image}}
	})
	dlg := w.dialog("Compose", []core.Node{from, e.recipient("To", &e.to), e.recipient("Cc", &e.cc), e.recipient("Bcc", &e.bcc), field("Subject", &e.content.Subject), workspaceui.Row(append(marks, insert)...), editor, fileList, schedule, reminder, core.Text{ContentFunc: e.status.Get, Style: ui.TypeCaption}}, ui.Button{Text: "Attach file", Disabled: e.busy.Get, OnPress: e.pick}, ui.Button{Text: "Save and close", Disabled: e.busy.Get, OnPress: closeDraft}, ui.Menu{Trigger: ui.Button{Text: "Send later", Variant: ui.ButtonSecondary}, Children: times}, ui.Button{Text: "Send", Disabled: e.busy.Get, OnPress: func() { requestSend(nil) }})
	dlg.OnOpenChange = func(open bool, _ base.Reason) {
		if !open {
			closeDraft()
		}
	}
	return dlg
}

// The current token may contain a quoted comma. Replacing it keeps all
// earlier tokens and defers quoting/validation to the shared address parser.
func (e *draftEditor) recipient(label string, dest *string) core.Node {
	value := core.NewSignal(e.cx, *dest)
	choices := core.NewSignalFunc(e.cx, []api.Address(nil), func(_, _ []api.Address) bool { return false })
	var timer func() bool
	var revision int
	change := func(text string) {
		revision++
		ticket := revision
		*dest = text
		value.Set(text)
		e.edit()
		choices.Set(nil)
		if timer != nil {
			timer()
		}
		_, prefix := oldcompose.RecipientPrefix(text)
		if prefix == "" || oldcompose.ValidAddress(prefix) {
			return
		}
		timer = e.cx.AfterFunc(180*time.Millisecond, func() {
			perform(e.w, func(ctx context.Context, c *api.Client) ([]api.Address, error) {
				limit := int64(5)
				return c.Address().Suggest(ctx, &api.AddressSuggestParams{Prefix: prefix, Limit: &limit})
			}, func(rows []api.Address, err error) {
				if err == nil {
					e.cx.Post(func() {
						if ticket == revision {
							choices.Set(rows)
						}
					})
				}
			})
		})
	}
	return workspaceui.Stack(ui.Input{Label: label, Placeholder: label, Value: value.Get, Disabled: e.busy.Get, OnChange: change}, core.Dynamic(func(*core.Scope) core.Node {
		var nodes []core.Node
		for _, a := range choices.Get() {
			token := oldcompose.Token(a)
			nodes = append(nodes, ui.Button{Text: token, Variant: ui.ButtonGhost, Disabled: e.busy.Get, OnPress: func() {
				prefix, _ := oldcompose.RecipientPrefix(*dest)
				if prefix != "" {
					prefix += " "
				}
				change(prefix + token + ", ")
			}})
		}
		if len(nodes) == 0 {
			return nil
		}
		return workspaceui.Stack(nodes...)
	}))
}

func (e *draftEditor) inlineImage() {
	e.cx.PickFiles(core.PickFilesOptions{Title: "Insert inline image"}, func(files []core.File, err error) {
		if err != nil {
			e.fail(err)
			return
		}
		if len(files) == 0 {
			return
		}
		f := files[0]
		e.busy.Set(true)
		go func() {
			data, err := readInline(f)
			e.cx.Post(func() {
				e.busy.Set(false)
				if err != nil {
					e.fail(err)
					return
				}
				if len(data)+len(e.editor.HTML()) > 8<<20 {
					e.fail(errors.New("The image would make this draft too large"))
					return
				}
				e.editor.RunNamed("setImage", richeditor.Attrs{"src": data, "alt": f.Name})
			})
		}()
	})
}

func readInline(file core.File) (string, error) {
	const maxSize = 6 << 20
	if file.Size <= 0 || file.Size > maxSize {
		return "", errors.New("Choose a PNG, JPEG or GIF smaller than 6 MiB")
	}
	reader, err := file.Open()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxSize+1))
	if err != nil {
		return "", err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || len(data) > maxSize || int64(config.Width)*int64(config.Height) > 32_000_000 {
		return "", errors.New("Choose a PNG, JPEG or GIF up to 32 megapixels")
	}
	return "data:" + http.DetectContentType(data) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (e *draftEditor) attachment(act func(context.Context, *api.Client) error) {
	if e.busy.GetUntracked() {
		return
	}
	e.busy.Set(true)
	perform(e.w, func(ctx context.Context, c *api.Client) (*api.Draft, error) {
		if err := act(ctx, c); err != nil {
			return nil, err
		}
		return c.Draft().Get(ctx, &api.DraftGetParams{ID: e.id})
	}, func(d *api.Draft, err error) {
		e.busy.Set(false)
		if err != nil {
			e.fail(err)
			return
		}
		e.files.Set(d.Attachments)
	})
}
func (e *draftEditor) pick() {
	e.cx.PickFiles(core.PickFilesOptions{Title: "Attach a file"}, func(files []core.File, err error) {
		if err != nil {
			e.fail(err)
			return
		}
		if len(files) == 0 {
			return
		}
		f := files[0]
		e.attachment(func(ctx context.Context, c *api.Client) error {
			if f.Size <= 0 || f.Size > 25<<20 {
				return errors.New("Choose a nonempty file of at most 25 MiB")
			}
			r, err := f.Open()
			if err != nil {
				return err
			}
			defer r.Close()
			var upload *string
			buf := make([]byte, min(int64(1<<20), f.Size))
			for offset := int64(0); offset < f.Size; {
				n, err := io.ReadFull(r, buf[:min(int64(len(buf)), f.Size-offset)])
				if err != nil {
					return err
				}
				result, err := c.Draft().AttachData(ctx, &api.DraftAttachDataParams{ID: e.id, Filename: f.Name, Size: f.Size, Offset: offset, Data: base64.StdEncoding.EncodeToString(buf[:n]), UploadID: upload})
				if err != nil {
					return err
				}
				if result.Received != offset+int64(n) {
					return errors.New("maild did not acknowledge the attachment chunk")
				}
				offset = result.Received
				upload = &result.UploadID
			}
			return nil
		})
	})
}

func (w *workspace) draftList() {
	perform(w, func(ctx context.Context, c *api.Client) ([]api.Draft, error) {
		return c.Draft().List(ctx, &api.DraftListParams{})
	}, func(drafts []api.Draft, err error) {
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.show(func(*core.Scope) core.Node {
			var rows []core.Node
			for _, d := range drafts {
				label := d.Content.Subject
				if label == "" {
					label = "Untitled draft"
				}
				rows = append(rows, workspaceui.Row(ui.Button{Text: label, OnPress: func() {
					perform(w, func(ctx context.Context, c *api.Client) (*api.Draft, error) {
						return c.Draft().Get(ctx, &api.DraftGetParams{ID: d.ID})
					}, w.editDraft)
				}}, ui.Button{Text: "Discard", Variant: ui.ButtonGhost, OnPress: func() {
					w.confirm("Discard draft?", label, "Discard", func() {
						w.change(func(ctx context.Context, c *api.Client) error {
							return c.Draft().Delete(ctx, &api.DraftDeleteParams{ID: d.ID})
						}, w.draftList)
					})
				}}))
			}
			if len(rows) == 0 {
				rows = append(rows, core.Text{Content: "No local drafts"})
			}
			return w.dialog("Drafts", []core.Node{core.Scroll{Children: rows, Style: core.Style{MaxHeight: core.Pt(430)}}})
		})
	})
}
func (w *workspace) outbox() {
	w.show(func(*core.Scope) core.Node {
		return w.dialog("Outbox", []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			w.state.Get()
			var rows []core.Node
			for _, item := range w.store.State().Outbox {
				row := []core.Node{core.Text{Content: item.Subject, Style: ui.TypeLabel}, core.Text{Content: string(item.State), Style: ui.TypeCaption}}
				if item.Error != nil {
					row = append(row, core.Text{Content: *item.Error})
				}
				if item.DraftID != nil {
					row = append(row, ui.Button{Text: "Cancel send and edit", OnPress: func() {
						perform(w, func(ctx context.Context, c *api.Client) (*api.Draft, error) {
							return c.Outbox().Cancel(ctx, &api.OutboxCancelParams{ID: item.ID})
						}, w.editDraft)
					}})
				}
				if item.State == api.OutboxStateFailed {
					row = append(row, ui.Button{Text: "Retry", OnPress: func() {
						w.call(func(ctx context.Context, c *api.Client) error {
							return c.Outbox().Retry(ctx, &api.OutboxRetryParams{ID: item.ID})
						})
					}})
				}
				rows = append(rows, workspaceui.Card{Children: row})
			}
			if len(rows) == 0 {
				return core.Text{Content: "Outbox is empty"}
			}
			return core.Scroll{Children: rows, Style: core.Style{MaxHeight: core.Pt(430)}}
		})})
	})
}

func (w *workspace) queued(item *api.OutboxItem) {
	title := "Message queued"
	delay := time.Duration(0)
	if item.SendAt != nil {
		delay = time.Until(*item.SendAt)
	}
	if item.Scheduled {
		title = "Send scheduled"
	}
	notice := ui.ToastContent{Title: title, Description: item.Subject, Type: ui.ToastSuccess}
	if item.DraftID != nil && delay > 0 {
		notice.Action = "Undo send"
		notice.Duration = min(delay, 30*time.Second)
		target := w.session.Target()
		notice.OnAction = func() {
			if !w.session.Target().Same(target) {
				return
			}
			cancel := func() {
				perform(w, func(ctx context.Context, c *api.Client) (*api.Draft, error) {
					return c.Outbox().Cancel(ctx, &api.OutboxCancelParams{ID: item.ID})
				}, w.editDraft)
			}
			if flush := w.beforeRoute; flush != nil {
				flush(cancel, func() {})
			} else {
				cancel()
			}
		}
	}
	w.toaster.Show(notice)
}
