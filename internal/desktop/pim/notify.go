package pim

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
)

// noticeOf reads the invite notice from a notifyRequired error.
func noticeOf(err error) *api.InviteNotice {
	var e *api.Error
	if !errors.As(err, &e) || e.Code != api.CodeNotifyRequired {
		return nil
	}
	var n api.InviteNotice
	if json.Unmarshal(e.Data, &n) != nil || n.Count < 1 {
		return nil
	}
	return &n
}

func noticeTitle(k api.NoticeKind) string {
	switch k {
	case api.NoticeKindInvitations:
		return "Send Invitations?"
	case api.NoticeKindCancellations:
		return "Send Cancellations?"
	}
	return "Send Updates?"
}

func noticeText(count int64) string {
	if count == 1 {
		return "The invitee will get it by email."
	}
	return fmt.Sprintf("%d invitees will get it by email.", count)
}

// writeOutcome is how a write through the notify sheet ended.
type writeOutcome struct {
	restore   string
	err       error
	cancelled bool
}

// withNotify runs a write, asking on d first if it would tell invitees;
// done runs on the UI thread. send gets notify nil, then true.
func (m *Module) withNotify(d *frostui.Dialogs, send func(ctx context.Context, c *api.Client, notify *bool) (string, error), done func(writeOutcome)) {
	c := m.client
	if c == nil {
		done(writeOutcome{err: errors.New("not connected")})
		return
	}
	call := func(notify *bool, then func(string, error)) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
			defer cancel()
			restore, err := send(ctx, c, notify)
			m.o.Post(func() { then(restore, err) })
		}()
	}
	call(nil, func(restore string, err error) {
		notice := noticeOf(err)
		if notice == nil {
			done(writeOutcome{restore: restore, err: err})
			return
		}
		d.Ask(frostui.Confirm{Title: noticeTitle(notice.Kind), Body: noticeText(notice.Count), Action: "Send",
			OnConfirm: func() {
				call(ptr(true), func(restore string, err error) { done(writeOutcome{restore: restore, err: err}) })
			},
			OnCancel: func() { done(writeOutcome{cancelled: true}) },
		})
	})
}

// rememberCalendar keeps a calendar write's undo, as remembered by the
// main window whichever window wrote it.
func (m *Module) rememberCalendar(restore string) {
	m.cal.undo = remember(m.cal.undo, restore)
}

func remember(list []string, restore string) []string {
	if restore == "" {
		return list
	}
	list = append(list, restore)
	if len(list) > undoLimit {
		list = list[len(list)-undoLimit:]
	}
	return list
}

// undoCalendar restores the newest calendar write; a restore that would
// tell invitees asks first, and Cancel keeps the entry.
func (m *Module) undoCalendar() {
	n := len(m.cal.undo)
	if n == 0 || m.client == nil {
		return
	}
	restore := m.cal.undo[n-1]
	m.cal.undo = m.cal.undo[:n-1]
	m.withNotify(m.o.Dialogs, func(ctx context.Context, c *api.Client, notify *bool) (string, error) {
		return "", c.Calendar().Restore(ctx, &api.CalendarRestoreParams{RestoreID: restore, Notify: notify})
	}, func(o writeOutcome) {
		switch {
		case o.cancelled:
			m.cal.undo = remember(m.cal.undo, restore)
		case o.err != nil:
			slog.Warn("undo event", "err", o.err)
			m.o.Toast("Could not undo", o.err)
		}
	})
}

// scopeSheet asks which occurrences a change or a delete affects.
type scopeSheet struct {
	delete bool
	first  bool
	choose func(api.EventScope)
	cancel func()
}

// drawScope draws s; it clears *slot when answered.
func drawScope(c *ui.Context, slot **scopeSheet) {
	s := *slot
	if s == nil {
		return
	}
	verb := "Change"
	if s.delete {
		verb = "Delete"
	}
	var chosen *api.EventScope
	cancelled := false
	dismissed := sheetFrame(c, verb+" Repeating Event", func() {
		frostui.Body(c, verb+" only this event, this and all future events, or every event in the series?").Wrap()
		ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.End).Children(func() {
			if ui.PrimaryButton(c, "This Event").AutoFocus().Clicked() {
				chosen = ptr(api.EventScopeThis)
			}
			if !s.first && ui.Button(c, "All Future Events").Clicked() {
				chosen = ptr(api.EventScopeFuture)
			}
			if ui.Button(c, "All Events").Clicked() {
				chosen = ptr(api.EventScopeAll)
			}
			if ui.Button(c, "Cancel").Clicked() {
				cancelled = true
			}
		})
	})
	switch {
	case chosen != nil:
		*slot = nil
		s.choose(*chosen)
	case cancelled || dismissed:
		*slot = nil
		if s.cancel != nil {
			s.cancel()
		}
	}
}
