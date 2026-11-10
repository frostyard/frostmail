package settings

import (
	"context"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
)

// snippetsPane is the Snippets pane (plan 0014 Phase 3).
type snippetsPane struct {
	request
	snippets []api.Snippet
	load     loader
	selected int64

	// The form's fields, and the stored snippet they last took.
	synced     api.Snippet
	name, text string

	adding  bool
	pending *pendingSnippet
}

// pendingSnippet is the name just added, with the snippets listed then:
// only one that appears after it, not one already there under that name,
// closes the form.
type pendingSnippet struct {
	name string
	ids  []int64
}

func (w *Window) loadSnippets() {
	p := w.snip
	if p == nil {
		return
	}
	p.load.run(w, func(ctx context.Context, c *api.Client) func() {
		list, err := c.Snippet().List(ctx, &api.SnippetListParams{})
		return func() {
			if err != nil {
				w.toast("Could not read the snippets", err)
				return
			}
			p.snippets = list
			p.listChanged()
		}
	})
}

// listChanged selects a snippet just added, or the first when the selected
// one left the list.
func (p *snippetsPane) listChanged() {
	if p.pending != nil {
		for _, s := range p.snippets {
			if !slices.Contains(p.pending.ids, s.ID) && strings.EqualFold(s.Name, p.pending.name) {
				p.selected = s.ID
				p.closeNew()
				return
			}
		}
		return
	}
	if p.selected != 0 && !slices.ContainsFunc(p.snippets, func(s api.Snippet) bool { return s.ID == p.selected }) {
		p.selected = 0
	}
}

func (p *snippetsPane) closeNew() {
	p.adding, p.pending = false, nil
	p.synced = api.Snippet{}
	p.name, p.text = "", ""
}

func (p *snippetsPane) current() *api.Snippet {
	if len(p.snippets) == 0 {
		return nil
	}
	if i := slices.IndexFunc(p.snippets, func(s api.Snippet) bool { return s.ID == p.selected }); i >= 0 {
		return &p.snippets[i]
	}
	return &p.snippets[0]
}

func (p *snippetsPane) openNew() {
	p.adding, p.pending, p.err = true, nil, ""
	p.synced = api.Snippet{}
	p.name, p.text = "", ""
}

func (w *Window) createSnippet(p *snippetsPane) {
	name, html := strings.TrimSpace(p.name), signatureHTML(p.text)
	ids := make([]int64, len(p.snippets))
	for i, s := range p.snippets {
		ids[i] = s.ID
	}
	p.pending = &pendingSnippet{name: name, ids: ids}
	w.run(&p.request, func(ctx context.Context, c *api.Client) error {
		_, err := c.Snippet().Create(ctx, &api.SnippetCreateParams{Name: name, HTML: html})
		return err
	}, nil)
}

func (w *Window) saveSnippet(p *snippetsPane, id int64) {
	name, html := strings.TrimSpace(p.name), signatureHTML(p.text)
	w.run(&p.request, func(ctx context.Context, c *api.Client) error {
		_, err := c.Snippet().Update(ctx, &api.SnippetUpdateParams{ID: id, Name: &name, HTML: &html})
		return err
	}, nil)
}

func (w *Window) removeSnippet(p *snippetsPane, s api.Snippet) {
	w.ask(frostui.Confirm{Title: "Remove Snippet?", Body: "Remove the snippet “" + s.Name + "”?", Action: "Remove", Destructive: true, OnConfirm: func() {
		w.run(&p.request, func(ctx context.Context, c *api.Client) error {
			return c.Snippet().Delete(ctx, &api.SnippetDeleteParams{ID: s.ID})
		}, nil)
	}})
}

func (w *Window) snippetsBody(c *ui.Context) {
	p := w.snip
	if p == nil {
		return
	}
	selected := p.current()
	ui.Column(c).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).Children(func() {
			listBox(c, "Snippets", func() {
				for _, s := range p.snippets {
					if listRow(c.Key(s.ID), s.Name, "", selected != nil && s.ID == selected.ID, false).Clicked() {
						p.selected = s.ID
						p.closeNew()
					}
				}
			})
			add, remove := addRemove(c, "Add Snippet", "Remove Snippet", p.busy || p.adding, p.busy || p.adding || selected == nil)
			if add {
				p.openNew()
			}
			if remove && selected != nil {
				w.removeSnippet(p, *selected)
			}
		})
		switch {
		case p.adding:
			ui.Column(c.Key("new")).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Children(func() { w.snippetForm(c, p, nil) })
		case selected != nil:
			ui.Column(c.Key(selected.ID)).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Children(func() { w.snippetForm(c, p, selected) })
		default:
			frostui.Muted(c, "Snippets are text you insert in a message with Insert Snippet in the compose window.").Wrap()
		}
	})
}

// snippetForm edits s, or a new snippet for nil.
func (w *Window) snippetForm(c *ui.Context, p *snippetsPane, s *api.Snippet) {
	title := "New Snippet"
	dirty := true
	if s != nil {
		if p.synced != *s {
			p.synced = *s
			p.name, p.text = s.Name, signatureText(s.HTML)
		}
		title = s.Name
		dirty = strings.TrimSpace(p.name) != s.Name || p.text != signatureText(s.HTML)
	}
	off := p.busy || strings.TrimSpace(p.name) == "" || !dirty
	submit := func() {
		switch {
		case off:
		case s == nil:
			w.createSnippet(p)
		default:
			w.saveSnippet(p, s.ID)
		}
	}
	frostui.Heading(c, title).Role(ui.RoleHeading).Level(2)
	ui.Form(c, func() {
		ui.Field(c.Key("name"), "Name:", func() {
			if ui.TextInput(c, &p.name).MaxLength(60).Submitted() {
				submit()
			}
		})
		ui.Field(c.Key("text"), "Text:", func() { ui.TextArea(c, &p.text).Height(160) })
	})
	alert(c, p.err)
	buttons(c, func() {
		label := "Save"
		if s == nil {
			label = "Add"
			if ui.Button(c, "Cancel").Clicked() {
				p.closeNew()
			}
		}
		if ui.PrimaryButton(c, label).Disabled(off).Clicked() {
			submit()
		}
	})
}
