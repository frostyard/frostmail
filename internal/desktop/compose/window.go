package compose

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// saveDelay is how long edits wait before draft.update; attachmentLimit
// the size the strip warns about, maild's own; uploadChunk one chunk's
// bytes, whose base64 stays within maild's 4 MiB; suggestLimit how many
// known addresses the recipient fields offer.
const (
	saveDelay       = 500 * time.Millisecond
	attachmentLimit = 25_000_000
	uploadChunk     = 3 << 20
	suggestLimit    = 2000
	callTimeout     = 60 * time.Second
)

// Options configure a compose window.
type Options struct {
	Client *api.Client
	Draft  int64
	// Fresh says the draft was just made for this window: closed
	// untouched, it is deleted.
	Fresh bool
	Post  func(func())
	Now   func() time.Time
	// Close closes the window; SetTitle names it.
	Close    func()
	SetTitle func(string)
	// OpenFiles asks for files to attach; none when cancelled. It runs on
	// a worker.
	OpenFiles func() ([]string, error)
	// FilesAsBytes sends a file's bytes rather than its path: maild cannot
	// read this computer's paths behind lodge, or from the Flatpak.
	FilesAsBytes bool
	// LoadPart reads a mailpart: image for the editor.
	LoadPart PartLoader
	// Sent tells the main window a message went into the Outbox.
	Sent func(item *api.OutboxItem)
	// EditSnippets opens Settings on its Snippets pane.
	EditSnippets func()
}

// pending is a file still being attached.
type pending struct{ key, name string }

// Window is one draft's window. Its state belongs to the UI thread.
type Window struct {
	o       Options
	shell   frostui.PaneShell
	table   *frostui.CommandTable
	dialogs frostui.Dialogs
	later   uikit.TimeSheet
	editor  ui.Handle

	loaded        bool
	failure       string
	draft         *api.Draft
	doc           *richtext.Document
	to, cc        []string
	bcc           []string
	subject       string
	identity      string
	draftIdentity int64
	identities    []api.Identity
	settled       bool
	known         []string
	snippets      []api.Snippet
	showBcc       bool
	formatBar     bool
	attached      []api.DraftAttachment
	files         []pending
	busy          bool
	errText       string
	linkOpen      bool
	reminder      bool
	reminderAt    *time.Time
	linkURL       string
	seq           int

	touched bool
	token   int

	// Saves go in order, off the UI thread.
	saveMu sync.Mutex
	latest api.DraftContent
	dirty  bool
}

// New returns a window that has not loaded its draft; Start loads it.
func New(o Options) *Window {
	if o.Now == nil {
		o.Now = time.Now
	}
	w := &Window{o: o, table: frostui.NewCommandTable()}
	w.shell.Dialogs = &w.dialogs
	w.table.Editing = func(c *ui.Context) bool { return c.FocusWithin(w.editor) }
	return w
}

// Start loads the draft and what its fields offer.
func (w *Window) Start(ctx context.Context) {
	go func() {
		d, err := w.o.Client.Draft().Get(ctx, &api.DraftGetParams{ID: w.o.Draft})
		if err != nil {
			w.o.Post(func() { w.failure = err.Error() })
			return
		}
		doc := richtext.NewDocument(ToText(d.Content.HTML, w.o.LoadPart))
		ids, ierr := w.o.Client.Identity().List(ctx, &api.IdentityListParams{AccountID: &d.AccountID})
		if ierr != nil {
			slog.Warn("listing identities", "err", ierr) // a reminder too many is safe; a missed one is not
		}
		limit := int64(suggestLimit)
		known, _ := w.o.Client.Address().Suggest(ctx, &api.AddressSuggestParams{Prefix: "", Limit: &limit})
		snippets, _ := w.o.Client.Snippet().List(ctx, &api.SnippetListParams{})
		w.o.Post(func() { w.loadedDraft(d, doc, ids, known, snippets) })
	}()
}

func tokens(list []api.Address) []string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, Token(a))
	}
	return out
}

func (w *Window) loadedDraft(d *api.Draft, doc *richtext.Document, ids []api.Identity, known []api.Address, snippets []api.Snippet) {
	w.draft, w.doc, w.identities, w.settled, w.snippets = d, doc, ids, true, snippets
	w.known = tokens(known)
	w.to, w.cc, w.bcc = tokens(d.Content.To), tokens(d.Content.Cc), tokens(d.Content.Bcc)
	w.subject, w.attached, w.showBcc = d.Content.Subject, d.Attachments, len(d.Content.Bcc) > 0
	for _, id := range ids {
		if id.ID == d.Content.IdentityID {
			w.identity = identityLabel(id)
		}
	}
	w.draftIdentity = d.Content.IdentityID
	w.saveMu.Lock()
	w.latest = d.Content
	w.saveMu.Unlock()
	w.loaded = true
	w.applyTitle()
}

func (w *Window) applyTitle() {
	if w.o.SetTitle != nil {
		w.o.SetTitle(w.title())
	}
}

func (w *Window) title() string {
	if strings.TrimSpace(w.subject) == "" {
		return "New Message"
	}
	return w.subject
}

// addresses reads a field's tokens; what does not parse is invalid.
func addresses(list []string) (out []api.Address, invalid []string) {
	return ParseAddresses(strings.Join(list, ","))
}

// normalize splits tokens typed with several addresses in them.
func normalize(list *[]string) {
	var out []string
	for _, tok := range *list {
		good, bad := ParseAddresses(tok)
		if len(good)+len(bad) <= 1 {
			out = append(out, tok)
			continue
		}
		out = append(out, tokens(good)...)
		out = append(out, bad...)
	}
	*list = slices.Compact(out)
}

func (w *Window) content() api.DraftContent {
	to, _ := addresses(w.to)
	cc, _ := addresses(w.cc)
	bcc, _ := addresses(w.bcc)
	identity := w.draftIdentity
	for _, id := range w.identities {
		if identityLabel(id) == w.identity {
			identity = id.ID
		}
	}
	return api.DraftContent{IdentityID: identity, To: orEmpty(to), Cc: orEmpty(cc), Bcc: orEmpty(bcc), Subject: w.subject, HTML: ToHTML(w.doc.Text())}
}

func identityLabel(id api.Identity) string { return id.Name + " <" + id.Email + ">" }

func orEmpty(list []api.Address) []api.Address {
	if list == nil {
		return []api.Address{}
	}
	return list
}

// change saves the draft soon after an edit.
func (w *Window) change() {
	w.touched = true
	content := w.content()
	w.saveMu.Lock()
	w.latest, w.dirty = content, true
	w.saveMu.Unlock()
	w.token++
	token := w.token
	time.AfterFunc(saveDelay, func() {
		w.o.Post(func() {
			if token == w.token {
				go func() { _ = w.flush() }()
			}
		})
	})
}

// flush writes what is left; it waits, so call it from a worker.
func (w *Window) flush() error {
	w.saveMu.Lock()
	defer w.saveMu.Unlock()
	if !w.dirty {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	if _, err := w.o.Client.Draft().Update(ctx, &api.DraftUpdateParams{ID: w.o.Draft, Content: w.latest}); err != nil {
		w.o.Post(func() { w.errText = "The draft was not saved: " + err.Error() })
		return err
	}
	w.dirty = false
	return nil
}

// canSend: at least one valid recipient, none invalid, nothing still being
// attached, and the signature known for the reminder.
func (w *Window) canSend() bool {
	if !w.loaded || !w.settled || w.busy || len(w.files) > 0 {
		return false
	}
	var all []string
	all = append(append(append(all, w.to...), w.cc...), w.bcc...)
	good, bad := addresses(all)
	return len(good) > 0 && len(bad) == 0
}

func (w *Window) signature() string {
	content := w.content()
	for _, id := range w.identities {
		if id.ID == content.IdentityID {
			return id.SignatureHTML
		}
	}
	return ""
}

// send sends now, or at the time given, after the attachment reminder.
func (w *Window) send(at *time.Time) {
	if !w.canSend() {
		return
	}
	if NeedsReminder(ToHTML(w.doc.Text()), w.signature(), len(w.attached)) {
		w.reminder, w.reminderAt = true, at
		return
	}
	w.sendNow(at)
}

func (w *Window) sendNow(at *time.Time) {
	w.busy = true
	w.touched = true
	content := w.content()
	w.saveMu.Lock()
	w.latest, w.dirty = content, true
	w.saveMu.Unlock()
	go func() {
		err := w.flush()
		var item *api.OutboxItem
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
			defer cancel()
			item, err = w.o.Client.Draft().Send(ctx, &api.DraftSendParams{ID: w.o.Draft, SendAt: at})
		}
		w.o.Post(func() {
			w.busy = false
			if err != nil {
				w.errText = "The message was not sent: " + err.Error()
				return
			}
			if w.o.Sent != nil {
				w.o.Sent(item)
			}
			w.o.Close()
		})
	}()
}

// empty reports a draft with nothing of the user's in it.
func (w *Window) empty() bool {
	return len(w.to)+len(w.cc)+len(w.bcc) == 0 && strings.TrimSpace(w.subject) == "" && len(w.attached) == 0 &&
		strings.TrimSpace(w.doc.Text().PlainText()) == ""
}

func (w *Window) deleteDraft() {
	if !w.loaded {
		return
	}
	if w.empty() {
		w.doDelete()
		return
	}
	w.dialogs.Ask(frostui.Confirm{Title: "Delete Draft?", Body: "This draft will be deleted.", Action: "Delete", Destructive: true, OnConfirm: w.doDelete})
}

func (w *Window) doDelete() {
	w.token++
	w.saveMu.Lock()
	w.dirty = false
	w.saveMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		err := w.o.Client.Draft().Delete(ctx, &api.DraftDeleteParams{ID: w.o.Draft})
		w.o.Post(func() {
			if err != nil {
				w.errText = "The draft was not deleted: " + err.Error()
				return
			}
			w.touched = false
			w.o.Close()
		})
	}()
}

// Closing runs as the window goes: it saves what is left, or deletes a
// draft made for this window and never touched.
func (w *Window) Closing() {
	w.token++
	fresh := w.o.Fresh && !w.touched
	go func() {
		if !fresh {
			_ = w.flush()
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		_ = w.o.Client.Draft().Delete(ctx, &api.DraftDeleteParams{ID: w.o.Draft})
	}()
}

func (w *Window) attachFiles() {
	if w.o.OpenFiles == nil {
		return
	}
	go func() {
		paths, err := w.o.OpenFiles()
		if err != nil || len(paths) == 0 {
			return
		}
		w.o.Post(func() { w.attachPaths(paths) })
	}()
}

func (w *Window) attachPaths(paths []string) {
	for _, path := range paths {
		w.seq++
		key := fmt.Sprint(w.seq)
		name := filepath.Base(path)
		w.touched = true
		w.files = append(w.files, pending{key, name})
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			var a *api.DraftAttachment
			var err error
			if w.o.FilesAsBytes {
				a, err = upload(ctx, w.o.Client, w.o.Draft, path)
			} else {
				a, err = w.o.Client.Draft().Attach(ctx, &api.DraftAttachParams{ID: w.o.Draft, Path: path})
			}
			w.o.Post(func() {
				w.files = slices.DeleteFunc(w.files, func(p pending) bool { return p.key == key })
				if err != nil {
					w.errText = name + " was not attached: " + err.Error()
					return
				}
				w.attached = append(w.attached, *a)
			})
		}()
	}
}

// upload sends a file's bytes in chunks, in order.
func upload(ctx context.Context, c *api.Client, draft int64, path string) (*api.DraftAttachment, error) {
	f, err := os.Open(path) //nolint:gosec // the file the user chose
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, errors.New("the file is empty")
	}
	var id *string
	var offset int64
	buf := make([]byte, uploadChunk)
	for {
		n, err := io.ReadFull(f, buf)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return nil, err
		}
		r, err := c.Draft().AttachData(ctx, &api.DraftAttachDataParams{ID: draft, Filename: filepath.Base(path), Size: size, Offset: offset,
			Data: base64.StdEncoding.EncodeToString(buf[:n]), UploadID: id})
		if err != nil {
			return nil, err
		}
		if r.Attachment != nil {
			return r.Attachment, nil
		}
		if r.Received <= offset {
			return nil, fmt.Errorf("maild took nothing at %d", offset)
		}
		id, offset = &r.UploadID, r.Received
	}
}

func (w *Window) detach(a api.DraftAttachment) {
	w.touched = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		err := w.o.Client.Draft().Detach(ctx, &api.DraftDetachParams{ID: w.o.Draft, AttachmentID: a.ID})
		w.o.Post(func() {
			if err != nil {
				w.errText = a.Filename + " was not removed: " + err.Error()
				return
			}
			w.attached = slices.DeleteFunc(w.attached, func(x api.DraftAttachment) bool { return x.ID == a.ID })
		})
	}()
}

func (w *Window) commands() []frostui.Command {
	key := func(mods ui.Modifiers, k ui.Key) []frostui.KeyBinding {
		return []frostui.KeyBinding{{Mods: mods, Key: k, Scope: frostui.Everywhere}}
	}
	return []frostui.Command{
		{ID: "send", Title: "Send", Group: "Message", Bindings: key(ui.Cmd, ui.KeyEnter), Run: func() { w.send(nil) },
			Enabled: func() (bool, string) { return w.canSend(), "Add a valid recipient first" }},
		{ID: "attach", Title: "Attach Files…", Group: "Message", Bindings: key(ui.Cmd|ui.Shift, ui.KeyA), Run: w.attachFiles},
		{ID: "bcc", Title: "Show/Hide Bcc", Group: "View", Bindings: key(ui.Cmd|ui.Shift, ui.KeyB), Run: func() { w.showBcc = !w.showBcc }},
		{ID: "format", Title: "Show/Hide Format Bar", Group: "View", Run: func() { w.formatBar = !w.formatBar }},
		{ID: "delete", Title: "Delete Draft", Group: "Message", Bindings: key(ui.Cmd, ui.KeyBackspace), Run: w.deleteDraft},
		{ID: "close", Title: "Close", Group: "Message", Bindings: key(ui.Cmd, ui.KeyW), Run: w.o.Close},
	}
}

// View draws the window.
func (w *Window) View(c *ui.Context) {
	frostui.Adapt(c)
	w.table.Add(w.commands()...)
	frame := frostui.PaneFrame{Table: w.table, Content: func() { w.content1(c) }}
	if w.loaded {
		frame.Start = func() { w.barStart(c) }
		frame.End = func() { w.barEnd(c) }
	}
	w.shell.View(c, frame)
	w.later.View(c, w.o.Now())
	w.reminderSheet(c)
}

// reminderSheet asks before a message that mentions an attachment goes
// without one: Cancel, Attach Files… (which has the focus) or Send Anyway.
func (w *Window) reminderSheet(c *ui.Context) {
	if !w.reminder {
		return
	}
	title := "Send without an attachment?"
	ui.Modal(c, &w.reminder, func() {
		ui.Column(c).Width(360).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Role(ui.RoleAlertDialog).Label(title).Children(func() {
			frostui.Heading(c, title)
			frostui.Body(c, "Your message mentions an attachment, but nothing is attached.").Wrap()
			ui.Row(c).Gap(frostui.SpaceS).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					w.reminder = false
				}
				if ui.PrimaryButton(c, "Attach Files…").AutoFocus().Clicked() {
					w.reminder = false
					w.attachFiles()
				}
				if ui.Button(c, "Send Anyway").Clicked() {
					w.reminder = false
					w.sendNow(w.reminderAt)
				}
			})
		})
	})
}

func (w *Window) content1(c *ui.Context) {
	if !w.loaded {
		text := "Opening draft…"
		if w.failure != "" {
			text = "This draft cannot be opened: " + w.failure
		}
		frostui.Empty(c, "mail-message-new", text, "").Grow(1)
		return
	}
	root := ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
	if dropped := root.DroppedFiles(); len(dropped) > 0 {
		w.attachPaths(dropped)
	}
	root.Children(func() {
		w.header(c)
		if w.formatBar {
			w.formatRow(c)
		}
		if w.linkOpen {
			w.linkRow(c)
		}
		if w.errText != "" {
			if _, closed := frostui.Banner(c, frostui.Danger, "dialog-warning", w.errText, "", true); closed {
				w.errText = ""
			}
		}
		ed := richtext.Editor(c, w.doc, richtext.EditorOptions{Label: "Message body", NoMarkdown: true, OnLink: func(string) {}, OnLinkCommand: w.askLink}).
			Grow(1).Padding(16, 20).Bind(&w.editor)
		if ed.Changed() {
			w.change()
		}
		w.attachmentRow(c)
	})
}

func (w *Window) barStart(c *ui.Context) {
	if frostui.IconButton(c, "mail-send", w.table.Tooltip("send")).Disabled(!w.canSend()).Clicked() {
		w.send(nil)
	}
	ui.MenuButton(c, "", w.laterMenu).Disabled(!w.canSend()).Label("Send Later").Tooltip("Send Later")
	frostui.Strong(c, w.title()).Shrink(1).MinWidth(0).SingleLine()
}

func (w *Window) barEnd(c *ui.Context) {
	ui.MenuButton(c, "", w.snippetMenu).Label("Insert Snippet").Tooltip("Insert Snippet").Children(func() {
		frostui.Icon(c, "text-x-generic")
	})
	if frostui.IconButton(c, "mail-attachment", w.table.Tooltip("attach")).Clicked() {
		w.attachFiles()
	}
	if frostui.IconButton(c, "format-text-bold", "Show Format Bar").Selected(w.formatBar).Role(ui.RoleToggleButton).Checked(w.formatBar).Clicked() {
		w.formatBar = !w.formatBar
	}
	if frostui.IconButton(c, "user-trash", w.table.Tooltip("delete")).Clicked() {
		w.deleteDraft()
	}
}

func (w *Window) laterMenu(m *ui.Menu) {
	for _, ch := range SendChoices(w.o.Now()) {
		if m.Item(ch.Label).Chosen() {
			at := ch.At
			w.send(&at)
		}
	}
	m.Separator()
	if m.Item("Send Later…").Chosen() {
		w.later.Show("Send Later", TomorrowMorning(w.o.Now()), func(at time.Time) { w.send(&at) })
	}
}

func (w *Window) snippetMenu(m *ui.Menu) {
	if len(w.snippets) == 0 {
		m.Item("No Snippets").Disabled(true)
	}
	for _, s := range w.snippets {
		if m.Item(s.Name).Chosen() {
			w.doc.Insert(ToText(s.HTML, w.o.LoadPart))
			w.change()
		}
	}
	m.Separator()
	if m.Item("Edit Snippets…").Chosen() && w.o.EditSnippets != nil {
		w.o.EditSnippets()
	}
}

func (w *Window) field(c *ui.Context, label string, draw func()) {
	t := c.Theme()
	ui.Row(c.Key(label)).Padding(4, 16).Gap(frostui.SpaceS).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, label+":").Width(64).TextColor(t.TextMuted).TextAlign(ui.End).SingleLine()
		draw()
	})
}

func (w *Window) recipients(c *ui.Context, label string, list *[]string) {
	w.field(c, label, func() {
		f := ui.TokenField(c, list, w.known).Label(label).Grow(1)
		if f.Changed() {
			normalize(list)
			w.change()
		}
	})
}

func (w *Window) header(c *ui.Context) {
	w.recipients(c, "To", &w.to)
	w.recipients(c, "Cc", &w.cc)
	if w.showBcc {
		w.recipients(c, "Bcc", &w.bcc)
	}
	w.field(c, "Subject", func() {
		if ui.TextInput(c, &w.subject).Label("Subject").Grow(1).Changed() {
			w.applyTitle()
			w.change()
		}
	})
	if len(w.identities) > 1 {
		w.field(c, "From", func() {
			choices := make([]string, 0, len(w.identities))
			for _, id := range w.identities {
				choices = append(choices, identityLabel(id))
			}
			if ui.Select(c, &w.identity, choices).Label("From").Grow(1).Changed() {
				w.change()
			}
		})
	}
}

func (w *Window) formatRow(c *ui.Context) {
	f := w.doc.Formatting()
	toggle := func(icon, label string, on bool, run func()) {
		if frostui.IconButton(c.Key(label), icon, label).Selected(on).Role(ui.RoleToggleButton).Checked(on).Clicked() {
			run()
			w.change()
		}
	}
	ui.Row(c).Height(32).Padding(0, frostui.SpaceS).Gap(2).AlignItems(ui.Center).Label("Format").Children(func() {
		toggle("format-text-bold", "Bold", f.Bold == richtext.FormatOn, w.doc.ToggleBold)
		toggle("format-text-italic", "Italic", f.Italic == richtext.FormatOn, w.doc.ToggleItalic)
		toggle("format-text-underline", "Underline", f.Underline == richtext.FormatOn, w.doc.ToggleUnderline)
		toggle("format-text-strikethrough", "Strikethrough", f.Strikethrough == richtext.FormatOn, w.doc.ToggleStrikethrough)
		toggle("view-list", "Bulleted List", f.Paragraph.List == richtext.Bullets, func() { w.doc.ToggleList(richtext.Bullets) })
		toggle("view-grid", "Numbered List", f.Paragraph.List == richtext.Numbers, func() { w.doc.ToggleList(richtext.Numbers) })
		toggle("mail-reply-sender", "Quote", f.Paragraph.Quote, w.doc.ToggleQuote)
		toggle("insert-link", "Link", f.Link != "", w.askLink)
		toggle("edit-clear-all", "Clear Formatting", false, w.clearFormatting)
	})
}

func (w *Window) clearFormatting() {
	w.doc.Format(func(a *richtext.Attributes) {
		attachment := a.Attachment
		*a = richtext.Attributes{Attachment: attachment}
	})
	w.doc.FormatParagraphs(func(p *richtext.ParagraphStyle) { *p = richtext.ParagraphStyle{ID: p.ID} })
}

func (w *Window) askLink() {
	w.linkOpen, w.linkURL = true, w.doc.Formatting().Link
}

func (w *Window) linkRow(c *ui.Context) {
	ui.Row(c).Padding(4, 16).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
		in := ui.TextInput(c, &w.linkURL).Label("Link address").Placeholder("https://").AutoFocus().Grow(1)
		done := ui.Button(c, "Done").Clicked()
		if done || in.Submitted() {
			url := strings.TrimSpace(w.linkURL)
			if url == "" || allowedLink(url) {
				w.doc.SetLink(url)
				w.change()
				w.linkOpen = false
			}
		}
		if ui.Button(c, "Cancel").Clicked() {
			w.linkOpen = false
		}
	})
}

func (w *Window) attachmentRow(c *ui.Context) {
	if len(w.attached)+len(w.files) == 0 {
		return
	}
	t := c.Theme()
	var total int64
	for _, a := range w.attached {
		total += a.Size
	}
	ui.Row(c).Gap(frostui.SpaceS).Padding(frostui.SpaceS, frostui.SpaceM).Wrap().AlignItems(ui.Center).Background(frostui.Chrome(t)).Label("Attachments").Children(func() {
		for _, a := range w.attached {
			ui.Row(c.Key(a.ID)).Height(32).Gap(6).Padding(0, frostui.SpaceS).Radius(6).Border(1, t.Border).Background(t.Background).AlignItems(ui.Center).Children(func() {
				frostui.Icon(c, "mail-attachment").TextColor(t.TextMuted)
				ui.Text(c, a.Filename).MaxWidth(200).SingleLine()
				ui.Text(c, mail.FormatSize(a.Size)).FontSize(12).TextColor(t.TextMuted).SingleLine()
				if frostui.IconButton(c, "window-close", "Remove "+a.Filename).Clicked() {
					w.detach(a)
				}
			})
		}
		for _, p := range w.files {
			ui.Row(c.Key(p.key)).Height(32).Gap(6).Padding(0, frostui.SpaceS).Radius(6).Border(1, t.Border).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(14, 14).Label("Attaching")
				ui.Text(c, p.name).MaxWidth(200).SingleLine()
			})
		}
		ui.Spacer(c)
		if len(w.attached) > 0 {
			text := fmt.Sprintf("%d files, %s", len(w.attached), mail.FormatSize(total))
			if len(w.attached) == 1 {
				text = "1 file, " + mail.FormatSize(total)
			}
			color := t.TextMuted
			if total > attachmentLimit {
				text += " — over the 25 MB limit"
				color = t.Danger
			}
			ui.Text(c, text).FontSize(12).TextColor(color).SingleLine()
		}
	})
}
