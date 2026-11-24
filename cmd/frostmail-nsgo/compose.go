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
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/richeditor"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
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
	}, w.freshDraft)
}
func (w *workspace) freshDraft(d *api.Draft, err error) { w.presentDraft(d, true, err) }
func (w *workspace) editDraft(d *api.Draft, err error)  { w.presentDraft(d, false, err) }
func (w *workspace) presentDraft(d *api.Draft, fresh bool, err error) {
	if err != nil {
		w.problem.Set(err.Error())
		return
	}
	w.show(func(cx *core.Scope) core.Node { return w.compose(cx, d, fresh) })
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
	fresh          bool
	openedAt       time.Time
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
	e.fresh = false
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

// A draft created by this window can be discarded on Close only while it is
// untouched. A reopened draft, including one reached through a shared link,
// always belongs to the user.
func (e *draftEditor) leave(next func()) {
	if !e.fresh {
		e.flush(next)
		return
	}
	if e.busy.GetUntracked() {
		return
	}
	e.busy.Set(true)
	perform(e.w, func(ctx context.Context, c *api.Client) (struct{}, error) {
		current, err := c.Draft().Get(ctx, &api.DraftGetParams{ID: e.id})
		if err != nil {
			return struct{}{}, err
		}
		if !current.UpdatedAt.Equal(e.openedAt) {
			return struct{}{}, nil
		}
		return struct{}{}, c.Draft().Delete(ctx, &api.DraftDeleteParams{ID: e.id})
	}, func(_ struct{}, err error) {
		if err != nil {
			e.fail(err)
			return
		}
		next()
	})
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

func (w *workspace) compose(cx *core.Scope, d *api.Draft, fresh bool) core.Node {
	e := &draftEditor{w: w, cx: cx, fresh: fresh, openedAt: d.UpdatedAt, id: d.ID, content: d.Content, to: addressLine(d.Content.To), cc: addressLine(d.Content.Cc), bcc: addressLine(d.Content.Bcc), status: core.NewSignal(cx, "Saved"), busy: core.NewSignal(cx, false), files: core.NewSignalFunc(cx, d.Attachments, func(_, _ []api.DraftAttachment) bool { return false })}
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
		e.leave(func() { e.onFailure = nil; goTo() })
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
	colors := ui.ColorsOf(cx)
	// row is a line of the header: its name, then its field.
	row := func(label string, control core.Node) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 8}, Children: []core.Node{
			core.Text{Content: label, Style: ui.TypeCaption.Merge(core.Style{Width: core.Pt(56), Padding: core.Edges{Top: 6}}), StyleFunc: ui.Colored(colors.Muted)},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{control}}}}
	}
	subject := ui.Input{Label: "Subject", DefaultValue: e.content.Subject, Disabled: e.busy.Get, OnChange: func(v string) { e.content.Subject = v; e.edit() }}
	identity := core.NewSignalFunc(cx, []api.Identity(nil), func(_, _ []api.Identity) bool { return false })
	perform(w, func(ctx context.Context, c *api.Client) ([]api.Identity, error) {
		return c.Identity().List(ctx, &api.IdentityListParams{AccountID: &d.AccountID})
	}, func(v []api.Identity, err error) {
		if err == nil {
			cx.Post(func() { identity.Set(v) })
		}
	})
	who := core.NewSignal(cx, d.Content.IdentityID)
	from := ui.Select[int64]{Label: "From", Placeholder: "From", Style: core.Style{Width: core.Pct(100)}, Disabled: e.busy.Get, Value: who.Get,
		OnChange: func(id int64) { who.Set(id); e.content.IdentityID = id; e.edit() },
		OptionsFunc: func() []ui.Option[int64] {
			var out []ui.Option[int64]
			for _, i := range identity.Get() {
				out = append(out, ui.Option[int64]{Value: i.ID, Label: i.Email})
			}
			return out
		}}
	format, linkBar := e.formatting(cx)
	fileList := core.Dynamic(func(*core.Scope) core.Node {
		var rows []core.Node
		for _, f := range e.files.Get() {
			rows = append(rows, core.View{TestID: "draft-file", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
				ui.Icon(lucide.Paperclip, 14, colors.Muted),
				core.Text{Content: f.Filename, MaxLines: 1, Style: ui.TypeBody.Merge(core.Style{FlexShrink: 1, MinWidth: core.Pt(0)})},
				figures(cx, mail.FormatSize(f.Size)), ui.Spacer{},
				ui.IconButton{Label: "Remove " + f.Filename, Icon: lucide.X, Disabled: e.busy.Get, OnPress: func() {
					e.attachment(func(ctx context.Context, c *api.Client) error {
						return c.Draft().Detach(ctx, &api.DraftDetachParams{ID: d.ID, AttachmentID: f.ID})
					})
				}}}})
		}
		if len(rows) == 0 {
			return nil
		}
		return core.View{Style: core.Style{Gap: 2}, Children: rows}
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
	reminder := ui.AlertDialog{Title: "Send without an attachment?", Description: "The message mentions an attachment, and has none.", StyleFunc: workspaceui.DialogPanel(cx), Open: remind.Get,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				remind.Set(false)
			}
		},
		Actions: []ui.AlertAction{
			{Text: "Keep editing", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
			{Text: "Send without attachment", Default: true, OnPress: func() { remind.Set(false); sendNow(pendingTime) }}}}
	scheduling := core.NewSignal(cx, false)
	// A date value is a wall clock time in UTC; the draft is sent at that
	// wall clock time here.
	tomorrow := oldcompose.TomorrowMorning(time.Now())
	when := core.NewSignal(cx, base.DateValue{Start: time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), tomorrow.Hour(), tomorrow.Minute(), 0, 0, time.UTC)})
	schedule := core.Show{When: scheduling.Get, Then: func(*core.Scope) core.Node {
		return row("Send at", workspaceui.Row(ui.DatePicker{Label: "Send at", TimePrecision: base.TimeMinute, Value: when.Get, OnChange: when.Set},
			primary("Schedule send", func() {
				v := when.GetUntracked().Start
				at := time.Date(v.Year(), v.Month(), v.Day(), v.Hour(), v.Minute(), 0, 0, time.Local)
				if !at.After(time.Now()) {
					e.status.Set("Choose a time that is still to come.")
					return
				}
				scheduling.Set(false)
				requestSend(&at)
			}), button("Cancel schedule", func() { scheduling.Set(false) })))
	}}
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
		return ui.Menu{Label: "Insert", Trigger: ui.Button{Label: "Insert", Icon: lucide.Plus, Variant: ui.ButtonGhost}, Children: items}
	})
	closeDraft := func() { e.flush(w.closePanel) }
	// Blocking edits during send keeps the transmitted snapshot identical to
	// the last saved draft.
	editor := core.Dynamic(func(*core.Scope) core.Node {
		if e.busy.Get() {
			return core.Text{Content: "Finishing draft…"}
		}
		// The header and the footer take about 470 points of the window.
		height := min(420, max(180, cx.Env().Window().Height-470))
		return rich.View{Editor: e.editor, Label: "Message text", Placeholder: "Write a message", Style: core.Style{Height: core.Pt(height)}, NodeViews: map[string]richeditor.NodeView{"image": draftImage, "inlineImage": draftImage}}
	})
	discarding := core.NewSignal(cx, false)
	discard := ui.AlertDialog{Title: "Discard this draft?", Description: "The draft and its attachments are removed.", StyleFunc: workspaceui.DialogPanel(cx), Open: discarding.Get,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				discarding.Set(false)
			}
		},
		Actions: []ui.AlertAction{
			{Text: "Keep draft", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
			{Text: "Discard draft", Variant: ui.ButtonDanger, Default: true, OnPress: func() {
				discarding.Set(false)
				if e.saving {
					e.status.Set("Saving the draft. Try discarding it again in a moment.")
					return
				}
				if e.timer != nil {
					e.timer()
				}
				e.busy.Set(true)
				perform(w, func(ctx context.Context, c *api.Client) (struct{}, error) {
					return struct{}{}, c.Draft().Delete(ctx, &api.DraftDeleteParams{ID: e.id})
				}, func(_ struct{}, err error) {
					if err != nil {
						e.fail(err)
						return
					}
					w.closePanel()
				})
			}}}}

	toolbar := ui.Toolbar{Label: "Formatting", Children: append(format, ui.ToolbarSeparator{}, ui.ToolbarGroup{Label: "Insert and attach", Children: []core.Node{
		insert, ui.IconButton{Label: "Attach file", Icon: lucide.Paperclip, Disabled: e.busy.Get, OnPress: e.pick}}})}
	body := []core.Node{row("From", from), row("To", e.recipient("To", &e.to)), row("Cc", e.recipient("Cc", &e.cc)), row("Bcc", e.recipient("Bcc", &e.bcc)), row("Subject", subject),
		toolbar, linkBar, editor, fileList, schedule}
	status := core.Text{ContentFunc: e.status.Get, MaxLines: 1, Style: ui.TypeCaption.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}), StyleFunc: ui.Colored(colors.Muted)}
	dlg := w.dialog("Compose", []core.Node{core.Scroll{StyleFunc: func() core.Style {
		footer := core.Points(160)
		if ui.Touch(cx) {
			footer = 250
		}
		return core.Style{MaxHeight: core.Pt(max(110, cx.Env().Window().Height-footer)), FlexShrink: 1}
	}, Children: []core.Node{core.Dynamic(func(keys *core.Scope) core.Node {
		keys.OnKey("mod+enter", func() {
			if !e.busy.GetUntracked() {
				requestSend(nil)
			}
		})
		keys.OnKey("mod+shift+a", func() {
			if !e.busy.GetUntracked() {
				e.pick()
			}
		})
		keys.OnKey("mod+w", func() { e.leave(w.closePanel) })
		keys.OnKey("mod+backspace", func() {
			if !e.busy.GetUntracked() {
				discarding.Set(true)
			}
		})

		return workspaceui.Stack(body...)
	})}}}, status, ui.Button{Text: "Discard…", Variant: ui.ButtonGhost, Disabled: e.busy.Get, OnPress: func() { discarding.Set(true) }},
		ui.Button{Text: "Save and close", Variant: ui.ButtonSecondary, Disabled: e.busy.Get, OnPress: closeDraft},
		ui.Menu{Trigger: ui.Button{Text: "Send later", Variant: ui.ButtonSecondary}, Children: times},
		ui.Button{Text: "Send", Icon: lucide.Send, Disabled: e.busy.Get, OnPress: func() { requestSend(nil) }})
	// The window is wider than a form's: a message is written in it.
	dlg.Style = core.Style{Width: core.Pt(720)}
	dlg.Children = append(dlg.Children, reminder, discard)
	dlg.OnOpenChange = func(open bool, _ base.Reason) {
		if !open {
			e.leave(w.closePanel)
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
	return workspaceui.Stack(ui.Input{Label: label, Value: value.Get, Disabled: e.busy.Get, OnChange: change}, core.Dynamic(func(*core.Scope) core.Node {
		var rows []core.Node
		for _, a := range choices.Get() {
			token := oldcompose.Token(a)
			name := mail.DisplayName(a)
			rows = append(rows, ui.FormRow{Leading: ui.Avatar{Name: name, Size: 24}, Title: name, Detail: a.Address, Label: "Add " + token, Disabled: e.busy.Get, OnPress: func() {
				prefix, _ := oldcompose.RecipientPrefix(*dest)
				if prefix != "" {
					prefix += " "
				}
				change(prefix + token + ", ")
			}})
		}
		if len(rows) == 0 {
			return nil
		}
		return ui.FormSection{Children: rows}
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
		e.fresh = false
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
				rows = append(rows, workspaceui.Row(ui.Button{Text: label, Variant: ui.ButtonSecondary, OnPress: func() {
					perform(w, func(ctx context.Context, c *api.Client) (*api.Draft, error) {
						return c.Draft().Get(ctx, &api.DraftGetParams{ID: d.ID})
					}, w.editDraft)
				}}, ui.Button{Text: "Discard", Variant: ui.ButtonDanger, OnPress: func() {
					w.destroy("Discard draft?", label, "Discard", func() {
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
					row = append(row, ui.Button{Text: "Cancel send and edit", Variant: ui.ButtonSecondary, OnPress: func() {
						perform(w, func(ctx context.Context, c *api.Client) (*api.Draft, error) {
							return c.Outbox().Cancel(ctx, &api.OutboxCancelParams{ID: item.ID})
						}, w.editDraft)
					}})
				}
				if item.State == api.OutboxStateFailed {
					row = append(row, ui.Button{Text: "Retry", Variant: ui.ButtonSecondary, OnPress: func() {
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
