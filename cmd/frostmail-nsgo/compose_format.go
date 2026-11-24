package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"strings"

	oldcompose "github.com/frostyard/frostmail/internal/desktop/compose"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base/richeditor"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Only image bytes embedded in the draft are eligible for display. URLs in
// quoted mail must never contact the sender while a reply is being edited.
func draftImage(_ *core.Scope, p richeditor.NodeProps) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		node := p.Node()
		alt := node.AttrStr("alt")
		if alt == "" {
			alt = "Inline image"
		}
		header, payload, ok := strings.Cut(node.AttrStr("src"), ",")
		if ok && len(payload) <= base64.StdEncoding.EncodedLen(6<<20) && (header == "data:image/png;base64" || header == "data:image/jpeg;base64" || header == "data:image/gif;base64") {
			if data, err := base64.StdEncoding.DecodeString(payload); err == nil {
				cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
				if err == nil && cfg.Width > 0 && cfg.Height > 0 && int64(cfg.Width)*int64(cfg.Height) <= 32_000_000 {
					scale := min(1., 240./float64(cfg.Width), 120./float64(cfg.Height))
					return core.Image{Label: alt, Source: core.ImageSource{Data: data}, Style: core.Style{Width: core.Pt(core.Points(float64(cfg.Width) * scale)), Height: core.Pt(core.Points(float64(cfg.Height) * scale))}}
				}
			}
		}
		return core.Text{Content: alt}
	})
}

// formatting is the formatting bar's groups of icon buttons, and the row
// that asks for a link's address while a link is being set.
func (e *draftEditor) formatting(cx *core.Scope) ([]core.Node, core.Node) {
	link := core.NewSignal(cx, false)
	startLink := func() {
		if !e.busy.GetUntracked() {
			link.Set(true)
		}
	}

	bar := core.Show{When: link.Get, Then: func(owner *core.Scope) core.Node {
		url := ""
		apply := func(v string) {
			href := strings.TrimSpace(v)
			if href == "" {
				e.editor.RunNamed("unsetLink", nil)
			} else if oldcompose.AllowedLink(href) {
				e.editor.RunNamed("setLink", richeditor.Attrs{"href": href})
			} else {
				e.status.Set("Use an https, http or mailto link.")
				return
			}
			link.Set(false)
			e.editor.Focus()
		}
		return workspaceui.Row(ui.Input{Label: "Link address", Placeholder: "https://", OnChange: func(v string) { url = v }, OnSubmit: apply}, primary("Apply link", func() { apply(url) }), button("Cancel link", func() { link.Set(false) }))
	}}
	run := func(name string) func() {
		return func() { e.editor.RunNamed(name, nil); e.editor.Focus() }
	}
	tool := func(label string, icon func() *core.SVG, press func()) core.Node {
		return ui.IconButton{Label: label, Icon: icon, Disabled: e.busy.Get, OnPress: press}
	}
	clear := func() {
		state := e.editor.State()
		tr := state.Tr().RemoveMark(state.Selection.From(), state.Selection.To(), nil, nil).SetStoredMarks(nil)
		e.editor.Dispatch(tr)
		e.editor.Focus()
	}
	// The first group also binds the link's keys while the bar is shown.
	text := core.Dynamic(func(keys *core.Scope) core.Node {
		keys.OnKey("mod+k", startLink)
		return ui.ToolbarGroup{Label: "Text", Children: []core.Node{
			tool("Bold", lucide.Bold, run("toggleBold")), tool("Italic", lucide.Italic, run("toggleItalic")),
			tool("Underline", lucide.Underline, run("toggleUnderline")), tool("Strikethrough", lucide.Strikethrough, run("toggleStrike"))}}
	})
	return []core.Node{text, ui.ToolbarSeparator{},
		ui.ToolbarGroup{Label: "Paragraph", Children: []core.Node{
			tool("Bulleted list", lucide.List, run("toggleBulletList")), tool("Numbered list", lucide.ListOrdered, run("toggleOrderedList")), tool("Quote", lucide.TextQuote, run("toggleBlockquote"))}},
		ui.ToolbarSeparator{},
		ui.ToolbarGroup{Label: "Link and clearing", Children: []core.Node{tool("Link", lucide.Link, startLink), tool("Clear formatting", lucide.RemoveFormatting, clear)}},
		ui.ToolbarSeparator{},
		ui.ToolbarGroup{Label: "History", Children: []core.Node{tool("Undo typing", lucide.Undo2, run("undo")), tool("Redo typing", lucide.Redo2, run("redo"))}},
	}, bar
}
