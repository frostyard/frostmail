package settings

import (
	"context"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
)

// signaturesPane is the Signatures pane: every account's identities.
type signaturesPane struct {
	request
	identities []api.Identity
	load       loader
	selected   int64

	// The form's fields, and the stored values they last took.
	synced              api.Identity
	name, replyTo, text string

	// The add form, open while adding is not 0.
	adding   int64
	addEmail string
	addName  string
	pending  *pendingAddress
}

// pendingAddress is an address just added, with the identities listed
// then: only one that appears after it closes the form.
type pendingAddress struct {
	accountID int64
	email     string
	ids       []int64
}

// loadIdentities reads the identities of the accounts shown.
func (w *Window) loadIdentities() {
	p := w.sig
	if p == nil {
		return
	}
	p.load.run(w, func(ctx context.Context, c *api.Client) func() {
		list, err := c.Identity().List(ctx, &api.IdentityListParams{})
		return func() {
			if err != nil {
				w.toast("Could not read the signatures", err)
				return
			}
			p.identities = list
			w.identitiesChanged(p)
		}
	})
}

// shownIdentities are the identities in list order: by account, and only
// those of an account shown.
func (w *Window) shownIdentities(p *signaturesPane) []api.Identity {
	var out []api.Identity
	for _, a := range w.data.accounts {
		for _, i := range p.identities {
			if i.AccountID == a.ID {
				out = append(out, i)
			}
		}
	}
	return out
}

// selectedIdentity is the identity shown: the one chosen, else the first.
func (w *Window) selectedIdentity(p *signaturesPane) *api.Identity {
	all := w.shownIdentities(p)
	if len(all) == 0 {
		return nil
	}
	if i := slices.IndexFunc(all, func(i api.Identity) bool { return i.ID == p.selected }); i >= 0 {
		return &all[i]
	}
	return &all[0]
}

// identitiesChanged closes the add form when its address appears.
func (w *Window) identitiesChanged(p *signaturesPane) {
	if p.pending == nil {
		return
	}
	for _, i := range p.identities {
		if !slices.Contains(p.pending.ids, i.ID) && i.AccountID == p.pending.accountID && strings.EqualFold(i.Email, p.pending.email) {
			p.selected = i.ID
			closeAddAddress(p)
			return
		}
	}
}

func closeAddAddress(p *signaturesPane) {
	p.adding, p.pending, p.addEmail, p.addName = 0, nil, "", ""
}

// syncIdentity shows the identity's stored values when it is selected and
// whenever they change.
func syncIdentity(p *signaturesPane, i api.Identity) {
	if p.synced != i {
		p.synced = i
		p.name, p.replyTo, p.text = i.Name, i.ReplyTo, signatureText(i.SignatureHTML)
	}
}

func (w *Window) saveIdentity(p *signaturesPane, id int64) {
	name, replyTo, html := p.name, p.replyTo, signatureHTML(p.text)
	w.run(&p.request, func(ctx context.Context, c *api.Client) error {
		if _, err := c.Identity().Update(ctx, &api.IdentityUpdateParams{ID: id, Name: &name, ReplyTo: &replyTo, SignatureHTML: &html}); err != nil {
			return err
		}
		w.o.Post(w.loadIdentities)
		return nil
	}, nil)
}

func (w *Window) addAddress(p *signaturesPane) {
	accountID, email, name := p.adding, strings.TrimSpace(p.addEmail), strings.TrimSpace(p.addName)
	ids := make([]int64, len(p.identities))
	for i, id := range p.identities {
		ids[i] = id.ID
	}
	p.pending = &pendingAddress{accountID: accountID, email: email, ids: ids}
	w.run(&p.request, func(ctx context.Context, c *api.Client) error {
		params := &api.IdentityCreateParams{AccountID: accountID, Email: email}
		if name != "" {
			params.Name = &name
		}
		if _, err := c.Identity().Create(ctx, params); err != nil {
			return err
		}
		w.o.Post(w.loadIdentities)
		return nil
	}, nil)
}

func (w *Window) removeAddress(p *signaturesPane, i api.Identity) {
	account := w.account(i.AccountID)
	if account == nil {
		return
	}
	w.ask(frostui.Confirm{Title: "Remove Address?", Body: "Remove " + i.Email + "? Drafts from it will be sent from " + account.Email + ".",
		Action: "Remove", Destructive: true, OnConfirm: func() {
			w.run(&p.request, func(ctx context.Context, c *api.Client) error {
				if err := c.Identity().Delete(ctx, &api.IdentityDeleteParams{ID: i.ID}); err != nil {
					return err
				}
				w.o.Post(w.loadIdentities)
				return nil
			}, nil)
		}})
}

func (w *Window) signaturesBody(c *ui.Context) {
	p := w.sig
	if p == nil {
		return
	}
	selected := w.selectedIdentity(p)
	if selected == nil {
		frostui.Muted(c, "Add an account to edit its signature.").Wrap()
		return
	}
	ui.Column(c).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Children(func() {
		w.identityList(c, p, selected)
		if p.adding != 0 {
			w.addAddressForm(c, p)
		} else {
			ui.Column(c.Key(selected.ID)).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Children(func() { w.identityForm(c, p, *selected) })
		}
	})
}

func (w *Window) identityList(c *ui.Context, p *signaturesPane, selected *api.Identity) {
	ui.Column(c).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).Children(func() {
		listBox(c, "Identities", func() {
			for _, a := range w.data.accounts {
				first := true
				for _, i := range p.identities {
					if i.AccountID != a.ID {
						continue
					}
					if first {
						first = false
						frostui.Caption(c.Key([2]any{"account", a.ID}), a.Email).Padding(frostui.SpaceS, frostui.SpaceM, 2, frostui.SpaceM).SingleLine()
					}
					title := i.Name
					if strings.TrimSpace(title) == "" {
						title = i.Email
					}
					if listRow(c.Key(i.ID), title, i.Email, i.ID == selected.ID, false).Clicked() {
						p.selected = i.ID
						closeAddAddress(p)
					}
				}
			}
		})
		add, remove := addRemove(c, "Add Address", "Remove Address", p.busy || p.adding != 0, p.busy || p.adding != 0 || selected.IsDefault)
		if add {
			p.adding, p.err = selected.AccountID, ""
		}
		if remove {
			w.removeAddress(p, *selected)
		}
	})
}

func (w *Window) addAddressForm(c *ui.Context, p *signaturesPane) {
	var options []option[int64]
	placeholder := ""
	for _, a := range w.data.accounts {
		if slices.ContainsFunc(p.identities, func(i api.Identity) bool { return i.AccountID == a.ID }) {
			options = append(options, option[int64]{a.ID, a.Email})
		}
	}
	for _, i := range p.identities {
		if i.AccountID == p.adding && i.IsDefault {
			placeholder = i.Name
		}
	}
	off := p.busy || !validEmail(strings.TrimSpace(p.addEmail))
	submit := func() {
		if !off {
			w.addAddress(p)
		}
	}
	frostui.Heading(c, "Add Address").Role(ui.RoleHeading).Level(2)
	ui.Form(c, func() {
		ui.Field(c.Key("account"), "Account:", func() {
			choose(c, "Account", p.adding, options, false, func(id int64) { p.adding = id })
		})
		ui.Field(c.Key("email"), "Email Address:", func() {
			if ui.TextInput(c, &p.addEmail).Submitted() {
				submit()
			}
		})
		ui.Field(c.Key("name"), "Full Name:", func() {
			if ui.TextInput(c, &p.addName).Placeholder(placeholder).Submitted() {
				submit()
			}
		}).Description("An address your provider delivers to this account, such as an alias or your own domain. Frostmail sends from it and answers invitations to it.")
	})
	alert(c, p.err)
	buttons(c, func() {
		if ui.Button(c, "Cancel").Clicked() {
			closeAddAddress(p)
		}
		if ui.PrimaryButton(c, "Add").Disabled(off).Clicked() {
			submit()
		}
	})
}

func (w *Window) identityForm(c *ui.Context, p *signaturesPane, i api.Identity) {
	syncIdentity(p, i)
	dirty := p.name != i.Name || p.replyTo != i.ReplyTo || p.text != signatureText(i.SignatureHTML)
	off := p.busy || !dirty
	submit := func() {
		if !off {
			w.saveIdentity(p, i.ID)
		}
	}
	ui.Form(c, func() {
		ui.Field(c.Key("name"), "Name:", func() {
			if ui.TextInput(c, &p.name).Submitted() {
				submit()
			}
		})
		ui.Field(c.Key("email"), "Email:", func() { frostui.Body(c, i.Email).Selectable() })
		ui.Field(c.Key("reply"), "Reply-To:", func() {
			if ui.TextInput(c, &p.replyTo).Placeholder("None").Submitted() {
				submit()
			}
		})
		ui.Field(c.Key("signature"), "Signature:", func() { ui.TextArea(c, &p.text).Height(160) })
	})
	alert(c, p.err)
	buttons(c, func() {
		if ui.PrimaryButton(c, "Save").Disabled(off).Clicked() {
			submit()
		}
	})
}
