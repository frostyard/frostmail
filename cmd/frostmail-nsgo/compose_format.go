package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"strings"

	oldcompose "github.com/frostyard/frostmail/internal/desktop/compose"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/base/richeditor"
	"github.com/nsgo-dev/nsgo/core"
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

func (e *draftEditor) formatting(cx *core.Scope) (core.Node, core.Node) {
	link := core.NewSignal(cx, false)
	startLink := func() {
		if !e.busy.GetUntracked() {
			link.Set(true)
		}
	}
	cx.OnKey("mod+k", startLink)
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
		return workspaceui.Row(ui.Input{Label: "Link address", Placeholder: "https://", OnChange: func(v string) { url = v }, OnSubmit: apply}, button("Apply link", func() { apply(url) }), button("Cancel link", func() { link.Set(false) }))
	}}
	var items []core.Node
	for _, m := range []struct{ label, name string }{{"Bold", "toggleBold"}, {"Italic", "toggleItalic"}, {"Underline", "toggleUnderline"}, {"Strikethrough", "toggleStrike"}, {"Bulleted list", "toggleBulletList"}, {"Numbered list", "toggleOrderedList"}, {"Quote", "toggleBlockquote"}, {"Undo", "undo"}, {"Redo", "redo"}} {
		items = append(items, ui.MenuItem{Text: m.label, OnSelect: func() { e.editor.RunNamed(m.name, nil); e.editor.Focus() }})
	}
	items = append(items, ui.MenuItem{Text: "Link", OnSelect: startLink}, ui.MenuItem{Text: "Clear formatting", OnSelect: func() {
		state := e.editor.State()
		tr := state.Tr().RemoveMark(state.Selection.From(), state.Selection.To(), nil, nil).SetStoredMarks(nil)
		e.editor.Dispatch(tr)
		e.editor.Focus()
	}})
	return ui.Menu{Trigger: ui.Button{Text: "Format", Disabled: e.busy.Get}, Children: items}, bar
}
