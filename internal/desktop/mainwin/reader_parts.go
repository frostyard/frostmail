package mainwin

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// contactCard is the popover for an address in a header.
type contactCard struct {
	address api.Address
	open    bool
	card    *api.ContactCard
	photo   *ui.Bitmap
	adding  bool
	added   bool
	addErr  string
}

func (w *Window) header(c *ui.Context, cv *conv, m *openMsg) {
	s := m.message.Summary
	st := w.mail.State()
	row := ui.Row(c).Gap(frostui.SpaceM).Padding(frostui.SpaceL, frostui.SpaceXL).AlignItems(ui.Start)
	if len(cv.rows) > 1 && row.Clicked() {
		w.setExpanded(cv, s.ID, false)
	}
	row.Children(func() {
		ui.Avatar(c, mail.DisplayName(s.From), w.photo(s.From.Address)).Size(40, 40).Radius(c.Theme().Radius).AccessHidden(true)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).AlignItems(ui.Stretch).Children(func() {
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				w.addressButton(c.Key("from"), m, s.From, true)
				if mail.IsVIP(s.From.Address, st.VIPs) {
					frostui.Icon(c, "starred").FontSize(frostui.TextSize(c, 12)).Label("VIP")
				}
				ui.Spacer(c)
				frostui.Caption(c, w.timeLabel(s.Date)).SingleLine()
				ui.MenuButton(c, "", func(menu *ui.Menu) { w.moreMenu(menu, cv, m) }).Label("More Actions").Tooltip("More Actions").Children(func() {
					frostui.Icon(c, "view-more-horizontal")
				})
			})
			subject := strings.TrimSpace(s.Subject)
			if subject == "" {
				subject = "(No Subject)"
			}
			ui.Text(c, subject).FontSize(frostui.TextSize(c, 15)).Bold().Wrap()
			w.addressLine(c.Key("to"), m, "To", m.message.To)
			w.addressLine(c.Key("cc"), m, "Cc", m.message.Cc)
		})
	})
}

func (w *Window) addressLine(c *ui.Context, m *openMsg, label string, addresses []api.Address) {
	if len(addresses) == 0 {
		return
	}
	ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
		frostui.Muted(c, label+":").SingleLine()
		for i, a := range addresses[:min(3, len(addresses))] {
			w.addressButton(c.Key(i), m, a, false)
		}
		if n := len(addresses) - 3; n > 0 {
			frostui.Muted(c, fmt.Sprintf("& %d more", n)).SingleLine()
		}
	})
}

// addressButton is a name that opens the contact popover.
func (w *Window) addressButton(c *ui.Context, m *openMsg, a api.Address, strong bool) {
	name := mail.DisplayName(a)
	label := name
	if name != a.Address {
		label = name + ", " + a.Address
	}
	b := ui.ButtonBase(c).Row().Cursor(ui.CursorPointer).Label(label).Tooltip(a.Address)
	b.Children(func() {
		if strong {
			frostui.Strong(c, name).SingleLine()
		} else {
			frostui.Muted(c, name).SingleLine()
		}
	})
	if b.Clicked() {
		w.openContact(m, a)
	}
	if card := m.contact; card != nil && strings.EqualFold(card.address.Address, a.Address) {
		ui.Popover(c, b, &card.open, func() { w.contactBody(c, m, card) })
		if !card.open {
			m.contact = nil
		}
	}
}

func (w *Window) openContact(m *openMsg, a api.Address) {
	card := &contactCard{address: a, open: true}
	m.contact = card
	c := w.client
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		got, err := c.People().Card(ctx, &api.PeopleCardParams{Email: a.Address})
		if err != nil {
			return
		}
		var photo *ui.Bitmap
		if got.Person != nil && got.Person.HasPhoto {
			if p, err := c.People().Photo(ctx, &api.PeoplePhotoParams{ID: got.Person.ID}); err == nil {
				photo = decodePhoto(base64Decode(p.Data))
			}
		}
		w.o.Post(func() { card.card, card.photo = got, photo })
	}()
}

func (w *Window) contactBody(c *ui.Context, m *openMsg, card *contactCard) {
	st := w.mail.State()
	a := card.address
	name := strings.TrimSpace(a.Name)
	if card.card != nil && strings.TrimSpace(card.card.Name) != "" {
		name = strings.TrimSpace(card.card.Name)
	}
	if name == "" {
		name = a.Address
	}
	vip := mail.IsVIP(a.Address, st.VIPs)
	blocked := slicesHasBlocked(st.Blocked, a.Address)
	ui.Column(c).Width(320).Gap(frostui.SpaceM).Padding(frostui.SpaceL).AlignItems(ui.Stretch).Role(ui.RoleDialog).Label(name).Children(func() {
		ui.Row(c).Gap(frostui.SpaceM).Children(func() {
			ui.Avatar(c, name, card.photo).Size(48, 48).Radius(c.Theme().Radius)
			ui.Column(c).Shrink(1).MinWidth(0).Children(func() {
				frostui.Strong(c, name).Wrap()
				frostui.Caption(c, a.Address).Wrap()
				if card.card != nil && card.card.Person != nil && card.card.Person.Organization != "" {
					frostui.Caption(c, card.card.Person.Organization)
				}
			})
		})
		ui.Row(c).Gap(frostui.SpaceS).Wrap().Children(func() {
			if ui.PrimaryButton(c, "Message").Clicked() {
				w.composeTo(a.Address, name)
				card.open = false
			}
			if card.card != nil && card.card.CanAdd && !card.added && !card.adding && ui.Button(c, "Add to Contacts").Clicked() {
				w.addContact(card, name)
			}
			if card.adding {
				frostui.Muted(c, "Adding…")
			}
			if card.added {
				frostui.Muted(c, "Added")
			}
			if card.card != nil && card.card.Person != nil && ui.Button(c, "Open in People").Clicked() {
				w.showPerson(card.card.Person.ID)
				card.open = false
			}
			label := "Add to VIPs"
			if vip {
				label = "Remove from VIPs"
			}
			if ui.Button(c, label).Clicked() {
				w.toggleVIP(card, vip)
			}
			label = "Block"
			if blocked {
				label = "Unblock"
			}
			if ui.Button(c, label).Clicked() {
				w.toggleBlock(a.Address, blocked)
			}
		})
		if card.addErr != "" {
			frostui.Toned(c, card.addErr, frostui.Danger).Wrap()
		}
		if card.card != nil && len(card.card.Recent) > 0 {
			frostui.Caption(c, "Recent Mail")
			for _, r := range card.card.Recent {
				if ui.ButtonBase(c.Key(r.ID)).Row().Gap(frostui.SpaceS).Label(r.Subject).Cursor(ui.CursorPointer).Children(func() {
					frostui.Body(c, r.Subject).Grow(1).Shrink(1).MinWidth(0).SingleLine()
					frostui.Caption(c, mail.ListDate(r.Date, w.o.Now())).SingleLine()
				}).Clicked() {
					w.OpenMessage(r.ID)
					card.open = false
				}
			}
		}
		if card.card != nil && len(card.card.Upcoming) > 0 {
			frostui.Caption(c, "Upcoming")
			for i, o := range card.card.Upcoming {
				if ui.ButtonBase(c.Key(i)).Row().Gap(frostui.SpaceS).Label(o.Summary).Cursor(ui.CursorPointer).Children(func() {
					frostui.Body(c, orNoTitle(o.Summary)).Grow(1).Shrink(1).MinWidth(0).SingleLine()
					frostui.Caption(c, mail.ListDate(o.Start, w.o.Now())).SingleLine()
				}).Clicked() {
					w.showOccurrence(o)
					card.open = false
				}
			}
		}
	})
	_ = m
}

func orNoTitle(s string) string {
	if strings.TrimSpace(s) == "" {
		return "No Title"
	}
	return s
}

func slicesHasBlocked(list []api.BlockedSender, address string) bool {
	address = strings.ToLower(address)
	for _, b := range list {
		if b.Address == address {
			return true
		}
	}
	return false
}

func (w *Window) addContact(card *contactCard, name string) {
	card.adding, card.addErr = true, ""
	a := card.address
	c := w.client
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		params := &api.PeopleAddParams{Email: a.Address}
		if name != "" {
			params.Name = &name
		}
		_, err := c.People().Add(ctx, params)
		var fresh *api.ContactCard
		if err == nil {
			fresh, err = c.People().Card(ctx, &api.PeopleCardParams{Email: a.Address})
		}
		w.o.Post(func() {
			card.adding = false
			if err != nil {
				card.addErr = err.Error()
				return
			}
			card.card, card.added = fresh, true
		})
	}()
}

func (w *Window) toggleVIP(card *contactCard, vip bool) {
	address := card.address.Address
	var person *int64
	if card.card != nil && card.card.Person != nil {
		person = &card.card.Person.ID
	}
	w.run("change VIPs", func(ctx context.Context, c *api.Client) error {
		var err error
		switch {
		case vip && person != nil:
			err = c.Vip().Remove(ctx, &api.VipRemoveParams{PersonID: person})
		case vip:
			err = c.Vip().Remove(ctx, &api.VipRemoveParams{Addresses: []string{address}})
		case person != nil:
			_, err = c.Vip().Add(ctx, &api.VipAddParams{PersonID: person})
		default:
			_, err = c.Vip().Add(ctx, &api.VipAddParams{Addresses: []string{address}})
		}
		return err
	})
}

func (w *Window) toggleBlock(address string, blocked bool) {
	w.run("change blocked senders", func(ctx context.Context, c *api.Client) error {
		if blocked {
			return c.Block().Remove(ctx, &api.BlockRemoveParams{Addresses: []string{address}})
		}
		_, err := c.Block().Add(ctx, &api.BlockAddParams{Addresses: []string{address}})
		return err
	})
}

func (w *Window) moreMenu(menu *ui.Menu, cv *conv, m *openMsg) {
	label := "Show All Headers"
	if m.headers != nil {
		label = "Hide All Headers"
	}
	if menu.Item(label).Shortcut(ui.Cmd|ui.Shift, ui.KeyH).Chosen() {
		w.toggleHeaders(cv, m)
	}
	if menu.Item("Raw Source…").Shortcut(ui.Cmd|ui.Alt, ui.KeyU).Chosen() {
		w.showRawSource(cv, m)
	}
	menu.Separator()
	if menu.Item("Save As…").Chosen() {
		w.saveMessage(m.summary)
	}
	if menu.Item("Print…").Shortcut(ui.Cmd, ui.KeyP).Chosen() && m.frame != nil {
		m.frame.Print()
	}
}

func (w *Window) toggleHeaders(cv *conv, m *openMsg) {
	if m.headers != nil {
		m.headers = nil
		return
	}
	id := m.summary.ID
	go func() {
		src, err := cv.client.Message().Source(cv.ctx, &api.MessageSourceParams{ID: id})
		if err != nil {
			w.toast("Could not read the headers", err)
			return
		}
		w.o.Post(func() { m.headers = &src.Headers })
	}()
}

func (w *Window) showRawSource(cv *conv, m *openMsg) {
	id, summary := m.summary.ID, m.summary
	gen := w.gen // read here, on the UI thread
	go func() {
		src, err := cv.client.Message().Source(cv.ctx, &api.MessageSourceParams{ID: id})
		if err != nil {
			w.toast("Could not read the message", err)
			return
		}
		w.o.Post(func() {
			if gen == w.gen { // not after a change of maild
				w.rawSource = &rawSourceSheet{summary: summary, source: src}
			}
		})
	}()
}

// rawSourceSheet is the Raw Source dialog's content.
type rawSourceSheet struct {
	summary api.MessageSummary
	source  *api.MessageSource
}

func (w *Window) rawSourceView(c *ui.Context) {
	s := w.rawSource
	if s == nil {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(720).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Role(ui.RoleDialog).Label("Raw Source").Children(func() {
			frostui.Heading(c, "Raw Source")
			ui.Scroll(c).Height(480).Fill().Children(func() { frostui.Mono(c, s.source.Text).Wrap() })
			if s.source.Truncated {
				frostui.Muted(c, "The message is longer than what is shown.")
			}
			ui.Row(c).Gap(frostui.SpaceS).Justify(ui.End).Children(func() {
				if ui.Button(c, "Save As…").Clicked() {
					w.saveMessage(s.summary)
				}
				if ui.PrimaryButton(c, "Close").Clicked() {
					open = false
				}
			})
		})
	})
	if !open {
		w.rawSource = nil
	}
}

// saveMessage asks where to save a message, then writes it there.
func (w *Window) saveMessage(s api.MessageSummary) {
	if w.o.SaveDialog == nil || w.client == nil {
		return
	}
	c := w.client
	lodge := w.state.Target.Lodge
	go func() {
		path, err := w.o.SaveDialog("Save Message", mail.SaveName(s.Subject), "Email Message", "eml")
		if err != nil || path == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		if lodge != nil {
			content := true
			src, err := c.Message().Source(ctx, &api.MessageSourceParams{ID: s.ID, Content: &content})
			if err == nil && src.ContentPath != nil {
				err = lodge.SaveTo(ctx, *src.ContentPath, path)
			} else if err == nil {
				err = errors.New("lodge gave no content path")
			}
			if err != nil {
				w.toast("Could not save the message", err)
			}
			return
		}
		if err := c.Message().Save(ctx, &api.MessageSaveParams{ID: s.ID, Path: path}); err != nil {
			w.toast("Could not save the message", err)
		}
	}()
}

func (w *Window) toast(title string, err error) { w.notice(title, err.Error()) }

// notice shows a toast saying why something did not happen.
func (w *Window) notice(title, text string) {
	w.o.Post(func() { w.toasts = append(w.toasts, ui.Toast{Title: title, Description: text, Type: "error"}) })
}

// banners draws the message's banners, in the spec's order.
func (w *Window) banners(c *ui.Context, cv *conv, m *openMsg) {
	s := m.summary
	st := w.mail.State()
	readOnly := w.readOnly(s.AccountID)
	if m.message.SenderCheck != nil && *m.message.SenderCheck == api.SenderCheckFail {
		action := "Show Details"
		if m.headers != nil {
			action = "Hide Details"
		}
		if w.banner(c.Key("sender"), mail.SenderCheckText(s.From.Address), action, false, "dialog-warning") {
			w.toggleHeaders(cv, m)
		}
	}
	if s.RemindAt != nil {
		w.reminderBanner(c.Key("remind"), m, readOnly)
	}
	if slicesHasBlocked(st.Blocked, s.From.Address) {
		if w.banner(c.Key("blocked"), "This message is from a blocked sender.", "Unblock", false, "") {
			w.toggleBlock(s.From.Address, true)
		}
	}
	w.unsubscribeBanner(c.Key("unsub"), cv, m, readOnly)
	if m.invitation != nil {
		w.invitationCard(c.Key("invite"), cv, m)
	}
	if r := m.rendering; r != nil && (r.Remote > 0 || r.Trackers > 0) {
		var words []string
		if r.Remote > 0 {
			words = append(words, "This message contains remote content.")
		}
		if r.Trackers > 0 {
			words = append(words, plural(r.Trackers, "tracker", "trackers")+" blocked.")
		}
		action := ""
		if r.Remote > 0 {
			action = "Load Remote Content"
			if m.loadingRemote {
				action = "Loading…"
			}
		}
		if w.banner(c.Key("remote"), strings.Join(words, " "), action, m.loadingRemote, "") {
			w.loadRemote(cv, m)
		}
	}
}

// banner draws one line above the message's body and reports its button.
func (w *Window) banner(c *ui.Context, text, action string, disabled bool, icon string) bool {
	t := c.Theme()
	clicked := false
	ui.Row(c).Gap(frostui.SpaceS).Padding(frostui.SpaceS, frostui.SpaceXL).AlignItems(ui.Center).Background(frostui.Chrome(t)).Role(ui.RoleStatus).Children(func() {
		if icon != "" {
			frostui.Icon(c, icon).TextColor(t.Warning)
		}
		ui.Text(c, text).FontSize(frostui.TextSize(c, 12)).Wrap().Grow(1).Shrink(1).MinWidth(0)
		if action != "" {
			clicked = ui.Button(c, action).Disabled(disabled).Clicked()
		}
	})
	return clicked
}

func (w *Window) reminderBanner(c *ui.Context, m *openMsg, readOnly bool) {
	t := c.Theme()
	when := mail.WhenText(*m.summary.RemindAt, w.o.Now())
	ui.Row(c).Gap(frostui.SpaceS).Padding(frostui.SpaceS, frostui.SpaceXL).AlignItems(ui.Center).Background(frostui.Chrome(t)).Role(ui.RoleStatus).Children(func() {
		ui.Text(c, "Remind Me: "+when).FontSize(frostui.TextSize(c, 12)).Grow(1)
		if ui.Button(c, "Change…").Disabled(readOnly).Clicked() {
			w.askRemind([]int64{m.summary.ID}, *m.summary.RemindAt)
		}
		if ui.Button(c, "Clear").Disabled(readOnly).Clicked() {
			w.remind([]int64{m.summary.ID}, nil)
		}
	})
}

func (w *Window) unsubscribeBanner(c *ui.Context, cv *conv, m *openMsg, readOnly bool) {
	info := m.unsub
	if info == nil {
		return
	}
	methods := mail.UnsubscribeMethods(info, readOnly)
	if info.Done {
		w.banner(c, fmt.Sprintf("You unsubscribed from the mailing list %s.", info.List), "", false, "")
		return
	}
	if len(info.Methods) == 0 {
		return
	}
	text := fmt.Sprintf("This message is from the mailing list %s.", info.List)
	if m.unsubFailed {
		text += " Couldn't unsubscribe."
	}
	action := "Unsubscribe"
	if m.unsubBusy {
		action = "Unsubscribing…"
	}
	if w.banner(c, text, action, m.unsubBusy || len(methods) == 0, "") {
		w.shell.Dialogs.Ask(frostui.Confirm{Title: fmt.Sprintf("Unsubscribe from %s?", info.List),
			Body: mail.UnsubscribeSentence(info, methods[0]), Action: "Unsubscribe", OnConfirm: func() { w.unsubscribe(cv, m, methods) }})
	}
}

func (w *Window) unsubscribe(cv *conv, m *openMsg, methods []api.UnsubscribeMethod) {
	m.unsubBusy, m.unsubFailed = true, false
	id := m.summary.ID
	go func() {
		res, err := tryUnsubscribe(cv.ctx, methods, func(ctx context.Context, method api.UnsubscribeMethod) (*api.UnsubscribeResult, error) {
			return cv.client.Message().Unsubscribe(ctx, &api.MessageUnsubscribeParams{ID: id, Method: method})
		})
		failed := err != nil
		if !failed && res.URL != nil {
			w.OpenLink(*res.URL)
		}
		var info *api.Unsubscribe
		if !failed {
			info, _ = cv.client.Message().UnsubscribeInfo(cv.ctx, &api.MessageUnsubscribeInfoParams{ID: id})
		}
		w.o.Post(func() {
			m.unsubBusy, m.unsubFailed = false, failed
			if info != nil {
				m.unsub = info
			}
		})
	}()
}

// tryUnsubscribe tries the methods in turn until one works. A cancel ends
// it: the client sends a call even on a cancelled context, and maild would
// carry out a method the user was not asked about, for a message no longer
// shown.
func tryUnsubscribe(ctx context.Context, methods []api.UnsubscribeMethod, call func(context.Context, api.UnsubscribeMethod) (*api.UnsubscribeResult, error)) (*api.UnsubscribeResult, error) {
	err := errors.New("no method to unsubscribe with")
	for _, method := range methods {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var res *api.UnsubscribeResult
		if res, err = call(ctx, method); err == nil {
			return res, nil
		}
	}
	return nil, err
}

func (w *Window) invitationCard(c *ui.Context, cv *conv, m *openMsg) {
	t := c.Theme()
	inv := m.invitation
	e := inv.Event
	lines := mail.InvitationLines(inv, w.o.Now().Location())
	ui.Row(c).Gap(frostui.SpaceM).Margin(frostui.SpaceS, frostui.SpaceM).Padding(frostui.SpaceM).Radius(frostui.RadiusCard).Border(1, t.Border).Background(frostui.Chrome(t)).Label("Invitation").Children(func() {
		ui.Column(c).Width(40).Height(44).Radius(t.Radius).Background(t.Background).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, lines.Month).FontSize(frostui.TextSize(c, 10)).Bold().TextColor(t.Danger)
			ui.Text(c, lines.Day).FontSize(frostui.TextSize(c, 18)).Bold()
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).AlignItems(ui.Stretch).Children(func() {
			ui.Row(c).Gap(frostui.SpaceM).Children(func() {
				ui.Text(c, orNoTitle(e.Summary)).FontSize(frostui.TextSize(c, 15)).Bold().Grow(1).Shrink(1).MinWidth(0).Wrap()
				tone := frostui.Neutral
				if inv.Method == api.ITIPMethodCancel {
					tone = frostui.Danger
				}
				frostui.Toned(c, lines.Badge, tone).FontSize(frostui.TextSize(c, 11)).Bold()
			})
			frostui.Body(c, lines.When).Wrap()
			if lines.Who != "" {
				frostui.Muted(c, lines.Who).Wrap()
			}
			if lines.Conflicts != "" {
				frostui.Toned(c, lines.Conflicts, frostui.Warning).Wrap()
			}
			if lines.Adjacent != "" {
				frostui.Caption(c, lines.Adjacent).Wrap()
			}
			ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
				if inv.CanRespond {
					for _, a := range mail.Answers {
						on := inv.Answer != nil && *inv.Answer == a.Answer
						b := ui.Button(c.Key(a.Label), a.Label).Disabled(m.inviteBusy).Selected(on).Role(ui.RoleToggleButton).Checked(on)
						if b.Clicked() {
							w.answerInvitation(cv, m, a.Answer)
						}
					}
				} else {
					frostui.Muted(c, lines.Status)
				}
				ui.Spacer(c)
				if ui.Button(c, "Show in Calendar").Clicked() {
					w.showInvitation(inv)
				}
			})
		})
	})
}

func (w *Window) answerInvitation(cv *conv, m *openMsg, answer api.PartStat) {
	if m.inviteBusy {
		return
	}
	m.inviteBusy = true
	id := m.summary.ID
	go func() {
		_, err := cv.client.Calendar().Respond(cv.ctx, &api.CalendarRespondParams{MessageID: &id, Answer: answer})
		if err != nil {
			w.toast("Could not answer the invitation", err)
		}
		w.o.Post(func() {
			m.inviteBusy = false
			if w.conv == cv {
				w.loadInvitation(cv, m)
			}
		})
	}()
}

func (w *Window) attachments(c *ui.Context, cv *conv, m *openMsg) {
	parts := mail.Attachments(m.message.Parts, m.invitation != nil)
	if len(parts) == 0 {
		return
	}
	t := c.Theme()
	ui.Row(c).Gap(frostui.SpaceS).Padding(frostui.SpaceS, frostui.SpaceXL, frostui.SpaceL, frostui.SpaceXL).Wrap().AlignItems(ui.End).Label("Attachments").Children(func() {
		for _, p := range parts {
			name := p.Filename
			if name == "" {
				name = "Untitled"
			}
			// A picture shows as its thumbnail once that is made, and as a
			// chip until then and when it cannot be made.
			thumb := m.thumbs[p.Path]
			item := ui.ButtonBase(c.Key(p.Path)).Radius(frostui.RadiusChip).Background(frostui.Chrome(t)).Cursor(ui.CursorPointer).Label(name).Tooltip(name)
			if thumb == nil {
				item = item.Row().Height(32).Gap(frostui.SpaceS).Padding(0, frostui.SpaceS).AlignItems(ui.Center)
			} else {
				item = item.Column().Gap(frostui.SpaceXS).Padding(frostui.SpaceXS).AlignItems(ui.Start)
			}
			if item.Clicked() {
				w.openPart(cv, m.summary.ID, p)
			}
			item.ContextMenu(func(menu *ui.Menu) {
				if menu.Item("Open").Chosen() {
					w.openPart(cv, m.summary.ID, p)
				}
				if menu.Item("Save As…").Chosen() {
					w.savePart(cv, m.summary.ID, p)
				}
			})
			item.Children(func() {
				if thumb == nil {
					frostui.Icon(c, "mail-attachment").TextColor(t.TextMuted)
					ui.Text(c, name).FontSize(frostui.TextSize(c, 12)).MaxWidth(200).SingleLine()
					frostui.Muted(c, mail.FormatSize(p.Size)).SingleLine()
					return
				}
				bw, bh := thumb.Size()
				width, height := mail.PreviewSize(bw, bh, thumbWidth, thumbHeight)
				ui.Image(c, thumb).Size(width, height).Radius(t.Radius).Shrink(0).Label("Preview of " + name)
				ui.Row(c).Gap(frostui.SpaceS).MaxWidth(max(width, thumbCaption)).Children(func() {
					ui.Text(c, name).FontSize(frostui.TextSize(c, 12)).Shrink(1).MinWidth(0).SingleLine()
					frostui.Muted(c, mail.FormatSize(p.Size)).SingleLine()
				})
			})
		}
	})
}

// A thumbnail shows inside thumbWidth × thumbHeight DIPs, made at twice
// that for dense screens; its caption is never narrower than thumbCaption.
const (
	thumbWidth   = 220
	thumbHeight  = 120
	thumbCaption = 96
)

// loadThumbnails makes the thumbnails of a message's picture attachments,
// one after another, off the UI thread. The pictures are the message's own
// parts, from maild: nothing is fetched from anywhere else. One that
// cannot be read or decoded stays a chip, which still opens and saves it.
func (w *Window) loadThumbnails(cv *conv, m *openMsg) {
	parts := mail.Previews(mail.Attachments(m.message.Parts, false))
	if len(parts) == 0 {
		return
	}
	id := m.summary.ID
	go func() {
		for _, p := range parts {
			if cv.ctx.Err() != nil {
				return
			}
			img, err := w.thumbnail(cv, id, p)
			if err != nil {
				slog.Debug("no thumbnail of an attachment", "message", id, "part", p.Path, "err", err)
				continue
			}
			w.o.Post(func() {
				if w.conv != cv || cv.msgs[id] != m {
					return
				}
				if m.thumbs == nil {
					m.thumbs = map[string]*ui.Bitmap{}
				}
				m.thumbs[p.Path] = ui.NewBitmap(img)
			})
		}
	}()
}

// thumbnail reads a part and scales it down. The window decodes one
// picture at a time: a large one takes its full size in memory meanwhile.
func (w *Window) thumbnail(cv *conv, id int64, p api.Part) (*image.RGBA, error) {
	ctx, cancel := context.WithTimeout(cv.ctx, downloadTimeout)
	defer cancel()
	data, err := w.partBytes(ctx, cv, id, p)
	if err != nil {
		return nil, err
	}
	w.thumbGate.Lock()
	defer w.thumbGate.Unlock()
	return uikit.Thumbnail(data, 2*thumbWidth, 2*thumbHeight)
}

// partBytes reads a part into memory: from maild's parts cache, or over
// lodge's content node. It reads no more than a thumbnail's part may hold.
func (w *Window) partBytes(ctx context.Context, cv *conv, id int64, p api.Part) ([]byte, error) {
	f, err := cv.client.Message().Part(ctx, &api.MessagePartParams{ID: id, Path: p.Path})
	if err != nil {
		return nil, err
	}
	if lodge := w.state.Target.Lodge; lodge != nil {
		if f.ContentPath == nil {
			return nil, errors.New("lodge gave no content path")
		}
		return lodge.Read(ctx, *f.ContentPath, f.Filename)
	}
	if !filepath.IsLocal(filepath.FromSlash(f.Path)) {
		return nil, fmt.Errorf("not a path in the parts cache: %s", f.Path)
	}
	file, err := os.Open(filepath.Join(w.o.CacheDir, "parts", filepath.FromSlash(f.Path))) //nolint:gosec // a part of the cache
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, mail.MaxPreviewBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > mail.MaxPreviewBytes {
		return nil, fmt.Errorf("the part is over %d MiB", mail.MaxPreviewBytes>>20)
	}
	return data, nil
}

// partFile fetches a part to a local file: from maild's parts cache, or
// over lodge's content node into the app's cache.
func (w *Window) partFile(ctx context.Context, cv *conv, id int64, p api.Part) (string, string, error) {
	f, err := cv.client.Message().Part(ctx, &api.MessagePartParams{ID: id, Path: p.Path})
	if err != nil {
		return "", "", err
	}
	if lodge := w.state.Target.Lodge; lodge != nil {
		if f.ContentPath == nil {
			return "", "", errors.New("lodge gave no content path")
		}
		dir, err := operationDir(w.o.DownloadDir, w.o.Now())
		if err != nil {
			return "", "", err
		}
		path, err := lodge.Download(ctx, dir, *f.ContentPath, f.Filename)
		return path, w.o.DownloadDir, err
	}
	root := filepath.Join(w.o.CacheDir, "parts")
	return filepath.Join(root, filepath.FromSlash(f.Path)), root, nil
}

// Each attachment opened from lodge gets a directory of its own, named with
// operationPrefix, so that 2 of one name never share a path; one older than
// operationKeep goes when the next is made: the viewer has had its file.
const (
	operationPrefix = "open-"
	operationKeep   = 24 * time.Hour
)

func operationDir(root string, now time.Time) (string, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), operationPrefix) {
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > operationKeep {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
	return os.MkdirTemp(root, operationPrefix+"*")
}

// savePartTo writes a part to the file the user chose: straight from lodge,
// or from maild's parts cache.
func (w *Window) savePartTo(ctx context.Context, cv *conv, id int64, p api.Part, dest string) error {
	lodge := w.state.Target.Lodge
	if lodge == nil {
		path, _, err := w.partFile(ctx, cv, id, p)
		if err != nil {
			return err
		}
		return copyFile(path, dest)
	}
	f, err := cv.client.Message().Part(ctx, &api.MessagePartParams{ID: id, Path: p.Path})
	if err != nil {
		return err
	}
	if f.ContentPath == nil {
		return errors.New("lodge gave no content path")
	}
	return lodge.SaveTo(ctx, *f.ContentPath, dest)
}

func (w *Window) openPart(cv *conv, id int64, p api.Part) {
	go func() {
		ctx, cancel := context.WithTimeout(cv.ctx, downloadTimeout)
		defer cancel()
		path, root, err := w.partFile(ctx, cv, id, p)
		if err == nil {
			err = mail.AttachmentOpener(root, w.o.LaunchFile).OpenFile(path)
		}
		if errors.Is(err, frostui.ErrNotAllowed) {
			w.notice("Frostmail does not open this kind of file", `Use "Save As…" in the attachment's menu to keep it.`)
			return
		}
		if err != nil {
			w.toast("Could not open the attachment", err)
		}
	}()
}

func (w *Window) savePart(cv *conv, id int64, p api.Part) {
	if w.o.SaveDialog == nil {
		return
	}
	go func() {
		name := p.Filename
		if name == "" {
			name = "attachment"
		}
		dest, err := w.o.SaveDialog("Save Attachment", name, "", "")
		if err != nil || dest == "" {
			return
		}
		ctx, cancel := context.WithTimeout(cv.ctx, downloadTimeout)
		defer cancel()
		if err := w.savePartTo(ctx, cv, id, p, dest); err != nil {
			w.toast("Could not save the attachment", err)
		}
	}()
}
