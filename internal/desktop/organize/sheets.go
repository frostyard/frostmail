package organize

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
)

// callTimeout bounds one save.
const callTimeout = 60 * time.Second

// Deps are what the sheets need from their window.
type Deps struct {
	Post    func(func())
	Client  func() *api.Client
	Context func() Context
}

// save runs fn off the UI thread and reports its error back on it.
func (d Deps) save(fn func(ctx context.Context, c *api.Client) error, done func(err error)) {
	c := d.Client()
	if c == nil {
		done(fmt.Errorf("not connected to maild"))
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		err := fn(ctx, c)
		d.Post(func() { done(err) })
	}()
}

func buttons(c *ui.Context, ok string, disabled bool, cancel, confirm func()) {
	ui.Row(c).Gap(frostui.SpaceS).Justify(ui.End).Children(func() {
		if ui.Button(c, "Cancel").Clicked() {
			cancel()
		}
		if ui.PrimaryButton(c, ok).Disabled(disabled).Clicked() {
			confirm()
		}
	})
}

func alert(c *ui.Context, text string) {
	if text != "" {
		frostui.Toned(c, text, frostui.Danger).Wrap().Role(ui.RoleAlert)
	}
}

// sheet is a sheet's column: its title, rows that scroll when the window is
// too short for them, and foot, always in view under them.
func sheet(c *ui.Context, width float32, title string, rows, foot func()) {
	t := c.Theme()
	_, height := c.Size()
	// The modal's panel keeps a margin of Space(5) on each side, and pads
	// its content by as much.
	ui.Column(c).Width(width).MaxHeight(height - t.Space(20)).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Role(ui.RoleDialog).Label(title).Children(func() {
		frostui.Heading(c, title)
		ui.Scroll(c).Shrink(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(frostui.SpaceXS).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Children(rows)
		})
		foot()
	})
}

// cloneConditions copies conditions, which the editors change in place: an
// original that a list holds, or a save under way, keeps what it had.
func cloneConditions(conds api.Conditions) api.Conditions {
	conds.Conditions = slices.Clone(conds.Conditions)
	return conds
}

// cloneActions copies actions, as cloneConditions does.
func cloneActions(actions []api.RuleAction) []api.RuleAction {
	out := slices.Clone(actions)
	for i := range out {
		out[i].MailboxID = clonePointer(out[i].MailboxID)
		out[i].Color = clonePointer(out[i].Color)
	}
	return out
}

func clonePointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// SmartSheet makes or edits a smart mailbox.
type SmartSheet struct {
	Deps
	// Created runs with a new smart mailbox, which becomes the list's
	// source.
	Created func(id int64)

	open        bool
	id          int64
	name        string
	conds       api.Conditions
	trash, sent bool
	busy        bool
	failure     string
}

// New opens the sheet on a new smart mailbox.
func (s *SmartSheet) New() {
	s.open, s.id, s.name, s.trash, s.sent, s.busy, s.failure = true, 0, "Smart Mailbox", false, false, false, ""
	s.conds = api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{NewCondition()}}
}

// FromSearch opens the sheet on what a search would save: named after its
// text, with both boxes checked.
func (s *SmartSheet) FromSearch(text string, conds api.Conditions) {
	s.New()
	s.name, s.conds, s.trash, s.sent = text, cloneConditions(conds), true, true
}

// Edit opens the sheet on a smart mailbox.
func (s *SmartSheet) Edit(m api.SmartMailbox) {
	s.open, s.id, s.name, s.conds, s.trash, s.sent, s.busy, s.failure = true, m.ID, m.Name, cloneConditions(m.Conditions), m.IncludeTrash, m.IncludeSent, false, ""
}

// View draws the sheet while it is open.
func (s *SmartSheet) View(c *ui.Context) {
	if !s.open {
		return
	}
	title := "New Smart Mailbox"
	if s.id != 0 {
		title = "Edit Smart Mailbox"
	}
	ui.Modal(c, &s.open, func() {
		sheet(c, 560, title, func() {
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				frostui.Body(c, "Smart Mailbox Name:")
				ui.TextInput(c, &s.name).Label("Smart Mailbox Name").MaxLength(100).AutoFocus().Grow(1)
			})
			ConditionEditor(c, &s.conds, s.Context(), "Contains messages that match", "of the following conditions:")
			ui.Checkbox(c, &s.trash, "Include messages from Trash")
			ui.Checkbox(c, &s.sent, "Include messages from Sent")
		}, func() {
			alert(c, s.failure)
			buttons(c, "OK", strings.TrimSpace(s.name) == "" || s.busy, func() { s.open = false }, s.save1)
		})
	})
}

func (s *SmartSheet) save1() {
	s.busy, s.failure = true, ""
	id, name, conds, trash, sent := s.id, strings.TrimSpace(s.name), cloneConditions(s.conds), s.trash, s.sent
	var created int64
	s.save(func(ctx context.Context, c *api.Client) error {
		if id != 0 {
			_, err := c.Smart().Update(ctx, &api.SmartUpdateParams{ID: id, Name: &name, Conditions: &conds, IncludeTrash: &trash, IncludeSent: &sent})
			return err
		}
		m, err := c.Smart().Create(ctx, &api.SmartCreateParams{Name: name, Conditions: conds, IncludeTrash: &trash, IncludeSent: &sent})
		if err == nil {
			created = m.ID
		}
		return err
	}, func(err error) {
		s.busy = false
		if err != nil {
			s.failure = err.Error()
			return
		}
		s.open = false
		if created != 0 && s.Created != nil {
			s.Created(created)
		}
	})
}

// RuleSheet makes or edits a rule.
type RuleSheet struct {
	Deps

	open    bool
	id      int64
	name    string
	conds   api.Conditions
	actions []api.RuleAction
	busy    bool
	failure string
}

// New opens the sheet on a new rule, the nth.
func (s *RuleSheet) New(n int) {
	s.open, s.id, s.busy, s.failure = true, 0, false, ""
	s.name = "Rule " + strconv.Itoa(n)
	s.conds = api.Conditions{Match: api.ConditionMatchAny, Conditions: []api.Condition{NewCondition()}}
	s.actions = []api.RuleAction{{Kind: api.RuleActionKindMove}}
}

// Edit opens the sheet on a rule.
func (s *RuleSheet) Edit(r api.Rule) {
	s.open, s.id, s.name, s.conds, s.actions, s.busy, s.failure = true, r.ID, r.Name, cloneConditions(r.Conditions), cloneActions(r.Actions), false, ""
}

// View draws the sheet while it is open.
func (s *RuleSheet) View(c *ui.Context) {
	if !s.open {
		return
	}
	title := "New Rule"
	if s.id != 0 {
		title = "Edit Rule"
	}
	ctx := s.Context()
	ui.Modal(c, &s.open, func() {
		sheet(c, 640, title, func() {
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				frostui.Body(c, "Description:")
				ui.TextInput(c, &s.name).Label("Description").MaxLength(100).AutoFocus().Grow(1)
			})
			ConditionEditor(c, &s.conds, ctx, "If", "of the following conditions are met:")
			ActionEditor(c, &s.actions, ctx)
		}, func() {
			alert(c, s.failure)
			disabled := strings.TrimSpace(s.name) == "" || !ActionsComplete(s.actions, ctx.Mailboxes) || s.busy
			buttons(c, "OK", disabled, func() { s.open = false }, s.save1)
		})
	})
}

func (s *RuleSheet) save1() {
	s.busy, s.failure = true, ""
	id, name, conds, actions := s.id, strings.TrimSpace(s.name), cloneConditions(s.conds), cloneActions(s.actions)
	s.save(func(ctx context.Context, c *api.Client) error {
		if id != 0 {
			_, err := c.Rule().Update(ctx, &api.RuleUpdateParams{ID: id, Name: &name, Conditions: &conds, Actions: actions})
			return err
		}
		_, err := c.Rule().Create(ctx, &api.RuleCreateParams{Name: name, Conditions: conds, Actions: actions})
		return err
	}, func(err error) {
		s.busy = false
		if err != nil {
			s.failure = err.Error()
			return
		}
		s.open = false
	})
}

// MailboxMode is what the mailbox sheet does.
type MailboxMode int

// The mailbox sheet's modes.
const (
	MailboxNew MailboxMode = iota
	MailboxRename
	MailboxMove
)

// MailboxSheet makes, renames or moves a mailbox.
type MailboxSheet struct {
	Deps

	open      bool
	mode      MailboxMode
	account   int64
	id        int64
	name      string
	parent    string
	locations []api.Mailbox
	busy      bool
	failure   string
}

// Show opens the sheet. For a new mailbox, account says where; for the
// others, mailbox is the one to change. locations are its possible parents.
func (s *MailboxSheet) Show(mode MailboxMode, account int64, mailbox *api.Mailbox, parent *int64, locations []api.Mailbox) {
	s.open, s.mode, s.account, s.id, s.name, s.parent, s.locations, s.busy, s.failure = true, mode, account, 0, "", "", locations, false, ""
	if mailbox != nil {
		s.id, s.name = mailbox.ID, mailbox.Name
	}
	if parent != nil {
		s.parent = strconv.FormatInt(*parent, 10)
	}
}

// View draws the sheet while it is open.
func (s *MailboxSheet) View(c *ui.Context) {
	if !s.open {
		return
	}
	title := [...]string{"New Mailbox", "Rename Mailbox", "Move Mailbox"}[s.mode]
	ui.Modal(c, &s.open, func() {
		ui.Column(c).Width(400).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Role(ui.RoleDialog).Label(title).Children(func() {
			frostui.Heading(c, title)
			if s.mode != MailboxMove {
				ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
					frostui.Body(c, "Name:")
					ui.TextInput(c, &s.name).Label("Name").MaxLength(100).AutoFocus().Grow(1)
				})
			}
			if s.mode != MailboxRename {
				ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
					frostui.Body(c, "Location:")
					choices := []choice{{"", "Top Level"}}
					for _, m := range s.locations {
						choices = append(choices, choice{strconv.FormatInt(m.ID, 10), m.Path})
					}
					picker(c, "Location", choices, &s.parent)
				})
			}
			alert(c, s.failure)
			buttons(c, "OK", strings.TrimSpace(s.name) == "" || s.busy, func() { s.open = false }, s.save1)
		})
	})
}

func (s *MailboxSheet) save1() {
	s.busy, s.failure = true, ""
	mode, account, id, name := s.mode, s.account, s.id, strings.TrimSpace(s.name)
	var parent *int64
	if v, err := strconv.ParseInt(s.parent, 10, 64); err == nil {
		parent = &v
	}
	s.save(func(ctx context.Context, c *api.Client) error {
		var err error
		switch mode {
		case MailboxNew:
			_, err = c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: account, Name: name, ParentID: parent})
		case MailboxRename:
			_, err = c.Mailbox().Rename(ctx, &api.MailboxRenameParams{ID: id, Name: name})
		case MailboxMove:
			_, err = c.Mailbox().Move(ctx, &api.MailboxMoveParams{ID: id, ParentID: parent})
		}
		return err
	}, func(err error) {
		s.busy = false
		if err != nil {
			s.failure = err.Error()
			return
		}
		s.open = false
	})
}
