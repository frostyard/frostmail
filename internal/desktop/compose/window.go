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
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// saveDelay is how long edits wait before draft.update; attachmentLimit
// the size the strip warns about, maild's own; uploadChunk one chunk's
// bytes, whose base64 stays within maild's 4 MiB; suggestPerLetter how
// many known addresses the recipient fields offer for each first letter;
// reconnectDelay how often a save waits for a connection to come back.
const (
	saveDelay        = 500 * time.Millisecond
	reconnectDelay   = time.Second
	attachmentLimit  = 25_000_000
	uploadChunk      = 3 << 20
	suggestPerLetter = 60
	callTimeout      = 60 * time.Second
	suggestLetters   = "abcdefghijklmnopqrstuvwxyz0123456789"
)

// errDisconnected says there is no connection to maild just now.
var errDisconnected = errors.New("not connected to maild")

// Options configure a compose window.
type Options struct {
	// Client is maild's client now, nil while there is none. It runs on
	// the UI thread: a worker uses the client read for it there.
	Client func() *api.Client
	Draft  int64
	// Fresh says the draft was just made for this window: closed
	// untouched, it is deleted.
	Fresh bool
	Post  func(func())
	Now   func() time.Time
	// Close closes the window, and may run on a worker; Hide hides it
	// while its last save ends; SetTitle names it.
	Close    func()
	Hide     func()
	SetTitle func(string)
	// OpenFiles asks for files to attach; none when cancelled. It runs on
	// a worker.
	OpenFiles func() ([]string, error)
	// FilesAsBytes sends a file's bytes rather than its path: maild cannot
	// read this computer's paths behind lodge, or from the Flatpak. It says
	// so for the connection of the moment.
	FilesAsBytes func() bool
	// LoadPart reads a mailpart: image for the editor, through c, which is
	// nil while disconnected. It runs on a worker.
	LoadPart func(c *api.Client, src string) ([]byte, error)
	// Sent tells the main window a message went into the Outbox.
	Sent func(item *api.OutboxItem)
	// EditSnippets opens Settings on its Snippets pane.
	EditSnippets func()
	// SaveFailed says the save on closing failed, when the window is gone
	// and cannot say so itself. It runs on a worker.
	SaveFailed func(err error)
}

// pending is a file still being attached.
type pending struct{ key, name string }

// Window is one draft's window. Its state belongs to the UI thread.
type Window struct {
	o       Options
	shell   frostui.PaneShell
	table   *frostui.CommandTable
	dialogs frostui.Dialogs
	later   TimeSheet
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
	detaching     []int64
	busy          bool
	errText       string
	linkOpen      bool
	reminder      bool
	reminderAt    *time.Time
	linkURL       string
	seq           int

	touched bool
	token   int

	// closing says the save on closing has started; finished, that it has
	// ended and the window may go.
	closing  bool
	finished atomic.Bool

	// Saves go in order, off the UI thread: flushMu is held across the
	// call to maild, and saveMu, which the UI thread takes, only to read
	// or change these fields. rev counts the changes to latest, so that a
	// save clears dirty only if nothing changed while it ran.
	flushMu sync.Mutex
	saveMu  sync.Mutex
	latest  api.DraftContent
	rev     uint64
	dirty   bool
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
	c := w.o.Client()
	if c == nil {
		w.failure = errDisconnected.Error()
		return
	}
	load := w.partLoader(c)
	go func() {
		d, err := c.Draft().Get(ctx, &api.DraftGetParams{ID: w.o.Draft})
		if err != nil {
			w.o.Post(func() { w.failure = err.Error() })
			return
		}
		doc := richtext.NewDocument(ToText(d.Content.HTML, load))
		ids, ierr := c.Identity().List(ctx, &api.IdentityListParams{AccountID: &d.AccountID})
		if ierr != nil {
			slog.Warn("listing identities", "err", ierr) // a reminder too many is safe; a missed one is not
		}
		known := knownAddresses(ctx, c)
		snippets, _ := c.Snippet().List(ctx, &api.SnippetListParams{})
		w.o.Post(func() { w.loadedDraft(d, doc, ids, known, snippets) })
	}()
}

// partLoader reads images through c, for ToText on a worker.
func (w *Window) partLoader(c *api.Client) PartLoader {
	if w.o.LoadPart == nil {
		return nil
	}
	return func(src string) ([]byte, error) { return w.o.LoadPart(c, src) }
}

// knownAddresses asks maild for what the recipient fields offer, by first
// letter: MyGo's TokenField does not say what is being typed, and
// address.suggest answers nothing for an empty prefix. Each letter's list
// comes best first, and the lists are merged by rank, so contacts and the
// most used come before the rest.
func knownAddresses(ctx context.Context, c *api.Client) []api.Address {
	limit := int64(suggestPerLetter)
	var lists [][]api.Address
	for _, letter := range suggestLetters {
		list, err := c.Address().Suggest(ctx, &api.AddressSuggestParams{Prefix: string(letter), Limit: &limit})
		if err != nil {
			slog.Warn("listing known addresses", "err", err)
			return nil
		}
		lists = append(lists, list)
	}
	var out []api.Address
	seen := map[string]bool{}
	for rank := range suggestPerLetter {
		for _, list := range lists {
			if rank >= len(list) || seen[strings.ToLower(list[rank].Address)] {
				continue
			}
			seen[strings.ToLower(list[rank].Address)] = true
			out = append(out, list[rank])
		}
	}
	return out
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

// recipients reads a field for the draft. What does not parse stays, as an
// address that is still invalid when the draft reopens: maild takes a
// half-typed address until sending, and canSend holds the send back.
func recipients(list []string) []api.Address {
	good, bad := addresses(list)
	out := orEmpty(good)
	for _, piece := range bad {
		out = append(out, api.Address{Address: piece})
	}
	return out
}

func (w *Window) content() api.DraftContent {
	identity := w.draftIdentity
	for _, id := range w.identities {
		if identityLabel(id) == w.identity {
			identity = id.ID
		}
	}
	return api.DraftContent{IdentityID: identity, To: recipients(w.to), Cc: recipients(w.cc), Bcc: recipients(w.bcc), Subject: w.subject, HTML: ToHTML(w.doc.Text())}
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
	w.stage()
	w.token++
	w.saveAfter(saveDelay)
}

// stage makes the editor's content what the next flush writes.
func (w *Window) stage() {
	content := w.content()
	w.saveMu.Lock()
	defer w.saveMu.Unlock()
	w.latest, w.dirty = content, true
	w.rev++
}

// saveAfter autosaves after delay, unless the window moved on by then.
func (w *Window) saveAfter(delay time.Duration) {
	token := w.token
	time.AfterFunc(delay, func() {
		w.o.Post(func() {
			if token == w.token {
				w.autosave()
			}
		})
	})
}

// autosave flushes while the window is open, and shows a failure in it.
// Without a connection it tries again: nothing tells this window of a
// reconnect.
func (w *Window) autosave() {
	c := w.o.Client()
	if c == nil {
		w.saveAfter(reconnectDelay)
		return
	}
	go func() {
		err := w.flush(c)
		if err == nil {
			return
		}
		select {
		case <-c.Done():
			w.o.Post(func() { w.saveAfter(reconnectDelay) })
		default:
			w.o.Post(func() { w.errText = "The draft was not saved: " + err.Error() })
		}
	}()
}

// flush writes what is left through c; it waits, so call it from a worker.
func (w *Window) flush(c *api.Client) error {
	w.flushMu.Lock()
	defer w.flushMu.Unlock()
	w.saveMu.Lock()
	content, rev, dirty := w.latest, w.rev, w.dirty
	w.saveMu.Unlock()
	if !dirty {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	if _, err := c.Draft().Update(ctx, &api.DraftUpdateParams{ID: w.o.Draft, Content: content}); err != nil {
		return err
	}
	w.saveMu.Lock()
	defer w.saveMu.Unlock()
	if w.rev == rev {
		w.dirty = false
	}
	return nil
}

// canSend: at least one valid recipient, none invalid, nothing still being
// attached or removed, and the signature known for the reminder.
func (w *Window) canSend() bool {
	if !w.loaded || !w.settled || w.busy || len(w.files)+len(w.detaching) > 0 {
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

// sendNow sends what the window holds now; content1 takes no edits until
// the send ends, as the draft goes with this content.
func (w *Window) sendNow(at *time.Time) {
	c := w.o.Client()
	if c == nil {
		w.errText = "The message was not sent: " + errDisconnected.Error()
		return
	}
	w.busy = true
	w.touched = true
	w.stage()
	w.token++
	go func() {
		err := w.flush(c)
		var item *api.OutboxItem
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
			defer cancel()
			item, err = c.Draft().Send(ctx, &api.DraftSendParams{ID: w.o.Draft, SendAt: at})
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
		strings.TrimSpace(w.doc.Text().PlainText()) == "" && !hasImage(w.doc.Text())
}

func (w *Window) deleteDraft() {
	if !w.loaded || w.busy {
		return
	}
	if w.empty() {
		w.doDelete()
		return
	}
	w.dialogs.Ask(frostui.Confirm{Title: "Delete Draft?", Body: "This draft will be deleted.", Action: "Delete", Destructive: true, OnConfirm: w.doDelete})
}

func (w *Window) doDelete() {
	c := w.o.Client()
	if c == nil {
		w.errText = "The draft was not deleted: " + errDisconnected.Error()
		return
	}
	w.token++
	w.saveMu.Lock()
	w.dirty = false
	w.saveMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		w.flushMu.Lock() // after a save still running
		err := c.Draft().Delete(ctx, &api.DraftDeleteParams{ID: w.o.Draft})
		w.flushMu.Unlock()
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

// RequestClose says whether the window may go now. Asked first, it says no:
// it hides the window and starts the save on closing, whose end closes the
// window again through Options.Close, and only then does it say yes. So
// the app, which quits with its last window, outlives the save.
func (w *Window) RequestClose() bool {
	if w.finished.Load() {
		return true
	}
	if w.closing {
		return false
	}
	w.closing = true
	if w.o.Hide != nil {
		w.o.Hide()
	}
	w.saveOnClose(func() {
		w.finished.Store(true)
		w.o.Close()
	})
	return false
}

// saveOnClose saves what is left, telling SaveFailed if that fails, or
// deletes a draft made for this window and never touched; then it calls
// done, on a worker.
func (w *Window) saveOnClose(done func()) {
	w.token++
	fresh := w.o.Fresh && !w.touched
	c := w.o.Client()
	go func() {
		defer done()
		switch {
		case fresh && c != nil:
			ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
			defer cancel()
			_ = c.Draft().Delete(ctx, &api.DraftDeleteParams{ID: w.o.Draft})
		case fresh:
		case c == nil:
			w.saveMu.Lock()
			dirty := w.dirty
			w.saveMu.Unlock()
			if dirty && w.o.SaveFailed != nil {
				w.o.SaveFailed(errDisconnected)
			}
		default:
			if err := w.flush(c); err != nil && w.o.SaveFailed != nil {
				w.o.SaveFailed(err)
			}
		}
	}()
}

func (w *Window) attachFiles() {
	if w.o.OpenFiles == nil || w.busy {
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
	c := w.o.Client()
	if c == nil {
		w.errText = "Nothing was attached: " + errDisconnected.Error()
		return
	}
	asBytes := w.o.FilesAsBytes != nil && w.o.FilesAsBytes()
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
			if asBytes {
				a, err = upload(ctx, c, w.o.Draft, path)
			} else {
				a, err = c.Draft().Attach(ctx, &api.DraftAttachParams{ID: w.o.Draft, Path: path})
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
	if slices.Contains(w.detaching, a.ID) {
		return
	}
	c := w.o.Client()
	if c == nil {
		w.errText = a.Filename + " was not removed: " + errDisconnected.Error()
		return
	}
	w.touched = true
	w.detaching = append(w.detaching, a.ID)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		err := c.Draft().Detach(ctx, &api.DraftDetachParams{ID: w.o.Draft, AttachmentID: a.ID})
		w.o.Post(func() {
			w.detaching = slices.DeleteFunc(w.detaching, func(id int64) bool { return id == a.ID })
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
	// First, so that what a palette command changes shows in this frame:
	// the rest of View reads that state before the shell would run it.
	w.shell.Palette.RunPending()
	frostui.Adapt(c)
	w.table.Add(w.commands()...)
	// The top bar holds the title and the palette field. The actions have
	// a row of their own under it: beside the field and the window
	// controls, a 720-wide window has no room for them.
	pane := frostui.Pane{ID: "message", Label: "Message", Content: func() { w.content1(c) },
		Bar: func() { frostui.Strong(c, w.title()).Shrink(1).MinWidth(0).SingleLine() }}
	w.shell.View(c, frostui.PaneFrame{Table: w.table, Panes: []frostui.Pane{pane}})
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
	// Disabled while sending: what the message goes with is settled.
	root := ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Disabled(w.busy)
	if dropped := root.DroppedFiles(); len(dropped) > 0 && !w.busy {
		w.attachPaths(dropped)
	}
	root.Children(func() {
		w.toolbar(c)
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
			w.keepPasted(c)
			w.change()
		}
		w.attachmentRow(c)
	})
}

// keepPasted gives a picture just pasted its source, from the clipboard
// it came from: the editor keeps only its bitmap, which cannot be saved.
func (w *Window) keepPasted(c *ui.Context) {
	t := w.doc.Text()
	if !hasUnsavedImage(t) {
		return
	}
	html, err := c.Services().ReadClipboardData(transfer.HTML).Read(transfer.HTML)
	if err != nil {
		html = nil
	}
	if KeepPastedImages(t, string(html)) > 0 {
		w.errText = "A pasted picture cannot be kept in the message. Attach it as a file."
	}
}

// toolbar is the row of the window's actions.
func (w *Window) toolbar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(40).Shrink(0).Padding(0, frostui.SpaceS).Gap(frostui.SpaceXS).AlignItems(ui.Center).Background(frostui.Chrome(t)).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Role(ui.RoleToolbar).Label("Message actions").Children(func() {
		if frostui.IconButton(c, "mail-send", w.table.Tooltip("send")).Disabled(!w.canSend()).Clicked() {
			w.send(nil)
		}
		ui.MenuButton(c, "", w.laterMenu).Disabled(!w.canSend()).Label("Send Later").Tooltip("Send Later")
		ui.Spacer(c)
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
	})
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
			w.insertSnippet(s.HTML)
		}
	}
	m.Separator()
	if m.Item("Edit Snippets…").Chosen() && w.o.EditSnippets != nil {
		w.o.EditSnippets()
	}
}

// insertSnippet reads a snippet on a worker, as its images may come over
// the network, and puts it in at the cursor.
func (w *Window) insertSnippet(src string) {
	load := w.partLoader(w.o.Client())
	go func() {
		text := ToText(src, load)
		w.o.Post(func() {
			w.doc.Insert(text)
			w.change()
		})
	}()
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
