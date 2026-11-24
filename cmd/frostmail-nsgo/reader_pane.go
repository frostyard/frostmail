package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The reader pane (plan 0016, Phase 3c): a header block, the subject as the
// pane's largest text, the kit's banners for what the pane has to say about
// the message, its attachments, and the message's page inside a hairline.

// messageReader is the reader pane.
func (w *workspace) messageReader(cx *core.Scope) core.Node {
	id := w.opened.Get()
	p := w.reader.Get()
	if p == nil {
		switch chosen := w.selection.Len(); {
		case id != 0:
			return workspaceui.Page(ui.Skeleton{Avatar: true, Lines: 3, Label: "Loading message"})
		case chosen > 1:
			return workspaceui.EmptyState{Icon: lucide.Mails, Title: strconv.Itoa(chosen) + " messages selected", Hint: "Delete, flag or move them from the toolbar."}
		}
		return workspaceui.EmptyState{Icon: lucide.MailOpen, Title: "No message selected", Hint: "Choose a message to read it here."}
	}
	m := p.message.Summary
	children := []core.Node{w.readerHeader(p.message),
		core.Text{Content: m.Subject, Selectable: core.SelectableOn, Style: ui.TypeHeading}}
	children = append(children, w.readerBanners(p)...)
	if files := w.attachments(p.message); files != nil {
		children = append(children, files)
	}
	children = append(children, pageFrame(cx, bodyView(w, p.rendering), ui.FillRest))
	return core.View{TestID: "reader", Style: ui.FillRest.Merge(core.Style{Padding: core.All(16), Gap: 12}), Children: children}
}

// readerHeader is who a message is from and to, and when it came.
func (w *workspace) readerHeader(m *api.Message) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		from := m.Summary.From
		name := mail.DisplayName(from)
		avatar := ui.Avatar{Name: name, Size: 40}
		// The photos arrive with the rows' signal.
		w.rows.Get()
		if w.showPhotos.Get() && w.photos != nil {
			w.photos.Want([]string{from.Address})
			if photo := w.photos.Get(from.Address); len(photo) > 0 {
				avatar.Image = core.ImageSource{Data: photo}
			}
		}
		cut := core.Style{FlexShrink: 1, MinWidth: core.Pt(0)}
		sender := []core.Node{core.Text{Content: name, MaxLines: 1, Selectable: core.SelectableOn, Style: ui.TypeLabel.Merge(cut).Merge(core.Style{FontWeight: ui.Semibold})}}
		if from.Address != "" && from.Address != name {
			sender = append(sender, core.Text{Content: from.Address, MaxLines: 1, Selectable: core.SelectableOn, Style: ui.TypeCaption.Merge(cut), StyleFunc: ui.Colored(colors.Muted)})
		}
		lines := []core.Node{core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 6}, Children: sender}}
		if to := recipients(m); to != "" {
			lines = append(lines, core.Text{Content: to, MaxLines: 1, Selectable: core.SelectableOn, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)})
		}
		when := ui.Tabular(ui.Text(m.Summary.Date.Local().Format("Mon, Jan 2, 2006 at 3:04 PM"), 12, ui.Regular, colors.Muted))
		when.TestID = "reader-date"
		return core.View{TestID: "reader-header", Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 12}, Children: []core.Node{
			avatar,
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: lines},
			when}}
	})
}

// recipients is a message's To and Cc as one line.
func recipients(m *api.Message) string {
	names := func(list []api.Address) string {
		out := make([]string, 0, len(list))
		for _, a := range list {
			out = append(out, mail.DisplayName(a))
		}
		return strings.Join(out, ", ")
	}
	var parts []string
	if to := names(m.To); to != "" {
		parts = append(parts, "To: "+to)
	}
	if cc := names(m.Cc); cc != "" {
		parts = append(parts, "Cc: "+cc)
	}
	return strings.Join(parts, "  ·  ")
}

// readerBanners are what the pane says about its message: each is the
// kit's banner in its real tone, with at most one action.
func (w *workspace) readerBanners(p *messagePage) []core.Node {
	m := p.message
	id := m.Summary.ID
	var out []core.Node
	if m.SenderCheck != nil && *m.SenderCheck == api.SenderCheckFail {
		out = append(out, workspaceui.Banner{Tone: workspaceui.Danger, Icon: lucide.ShieldAlert, Text: mail.SenderCheckText(m.Summary.From.Address)})
	}
	// The reminder follows the message as maild reports it.
	out = append(out, core.Dynamic(func(*core.Scope) core.Node {
		current := w.currentMessage.Get()
		if current == nil || current.ID != id || current.RemindAt == nil {
			return nil
		}
		return workspaceui.Banner{Tone: workspaceui.Accent, Icon: lucide.Clock, Text: "Reminder: " + mail.WhenText(*current.RemindAt, time.Now()) + ".",
			Action: "Remove reminder", OnAction: func() { w.remind(id, nil) }}
	}))
	if n := p.rendering.Remote; n > 0 {
		text := strconv.FormatInt(n, 10) + " remote images are not shown."
		if n == 1 {
			text = "1 remote image is not shown."
		}
		out = append(out, workspaceui.Banner{Icon: lucide.ImageOff, Text: text, Action: "Load remote images", OnAction: func() { w.openMessage(id, true) }})
	}
	invitation := mail.InvitationPart(m.Parts) != nil
	if invitation {
		out = append(out, workspaceui.Banner{Tone: workspaceui.Accent, Icon: lucide.CalendarDays, Text: "This message is a calendar invitation.",
			Action: "Open invitation", OnAction: func() { w.invitation(id) }})
	}
	if m.ListUnsubscribe != "" {
		out = append(out, workspaceui.Banner{Icon: lucide.MailMinus, Text: "This message is from a mailing list.",
			Action: "Unsubscribe", OnAction: func() { w.subscription(id) }})
	}
	if n := len(p.thread); n > 1 {
		out = append(out, workspaceui.Banner{Icon: lucide.Mails, Text: fmt.Sprintf("This message is one of %d in a conversation.", n),
			Action: fmt.Sprintf("View conversation (%d messages)", n), OnAction: func() { w.conversation(p) }})
	}
	return out
}

// attachments lists a message's files under a caption that counts them: a
// name, a size, and Open and Save. It is nil for a message with none.
func (w *workspace) attachments(m *api.Message) core.Node {
	parts := mail.Attachments(m.Parts, mail.InvitationPart(m.Parts) != nil)
	if len(parts) == 0 {
		return nil
	}
	id := m.Summary.ID
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		previews := mail.Previews(parts)
		caption := strconv.Itoa(len(parts)) + " attachments"
		if len(parts) == 1 {
			caption = "1 attachment"
		}
		files := []core.Node{core.Text{Content: caption, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)}}
		for _, p := range parts {
			name := p.Filename
			if name == "" {
				name = p.ContentType
			}
			if slices.ContainsFunc(previews, func(preview api.Part) bool { return preview.Path == p.Path }) {
				if picture := w.partThumbnail(cx, id, p); picture != nil {
					files = append(files, picture)
				}
			}
			row := []core.Node{ui.Icon(lucide.Paperclip, 14, colors.Muted),
				core.Text{Content: name, MaxLines: 1, Style: ui.TypeBody.Merge(core.Style{FlexShrink: 1, MinWidth: core.Pt(0)})},
				ui.Tabular(ui.Text(mail.FormatSize(p.Size), 12, ui.Regular, colors.Muted)), ui.Spacer{}}
			if desktopServer || p.ContentType == "application/pdf" || mail.Previewable(p) {
				row = append(row, ui.IconButton{Label: "Open " + name, Icon: lucide.ExternalLink, OnPress: func() { w.openAttachment(id, p) }})
			}
			row = append(row, ui.IconButton{Label: "Save " + name, Icon: lucide.Download, OnPress: func() { w.savePart(id, p) }})
			files = append(files, core.View{TestID: "attachment", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: row})
		}
		return core.View{TestID: "attachments", Style: core.Style{Gap: 2}, Children: files}
	})
}

// pageFrame sets a message's page inside a hairline with rounded corners,
// so that in the dark theme it reads as a page placed in the window. The
// page is the sender's and keeps its own colors.
func pageFrame(cx *core.Scope, page core.Node, size core.Style) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: "page-frame", Style: size.Merge(core.Style{BorderWidth: 1, CornerRadius: ui.RadiusInset, Overflow: core.Hidden}),
		StyleFunc: func() core.Style { return core.Style{BorderColor: colors.Border()} }, Children: []core.Node{page}}
}
