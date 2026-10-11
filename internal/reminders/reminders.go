// Package reminders fires calendar reminders when their alarms come due
// (docs/design/pim.md, Reminders): it records them as fired, tells the app,
// and, while the app is away, shows desktop notifications. Each minute it
// also has maild fire the messages' Remind Me reminders (ADR-0025).
package reminders

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/notify"
	"github.com/frostyard/frostmail/internal/store"
)

// catchUp is how far back the first check after a start looks: alarms
// that came due while maild was stopped, up to a day ago, still remind.
const catchUp = 24 * time.Hour

// Notifier shows reminders on the desktop (notify.Desktop).
type Notifier interface {
	Remind(ctx context.Context, rs []notify.Reminder) error
	// Withdraw closes the notes of reminders not in active.
	Withdraw(ctx context.Context, active map[string]bool) error
}

// Config is what the scheduler needs; nil Now, Local and Log take
// time.Now, time.Local and a discarding logger.
type Config struct {
	DB *store.DB
	// List is calendar.reminders: the reminders to show now.
	List func(ctx context.Context) ([]api.Reminder, error)
	// Attended reports whether the app is connected (rpcserver).
	Attended func() bool
	// Notifier shows reminders while the app is away; nil shows none.
	Notifier Notifier
	// Messages fires the Remind Me reminders of messages due by now
	// (mailsync.Manager.FireReminders); nil fires none.
	Messages func(ctx context.Context, now time.Time) error
	Now      func() time.Time
	Local    *time.Location
	Log      *slog.Logger
}

// Scheduler checks for due reminders once a minute.
type Scheduler struct {
	cfg  Config
	last time.Time
	kick chan struct{}
}

// New returns a Scheduler.
func New(cfg Config) *Scheduler {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Local == nil {
		cfg.Local = time.Local
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	return &Scheduler{cfg: cfg, kick: make(chan struct{}, 1)}
}

// Kick asks for a check now: a reminder was snoozed or dismissed, or the
// app came or went.
func (s *Scheduler) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Run checks at each minute, and when kicked, until ctx ends.
func (s *Scheduler) Run(ctx context.Context) {
	for {
		if err := s.Check(ctx); err != nil && ctx.Err() == nil {
			s.cfg.Log.Warn("reminders not checked", "err", err)
		}
		now := s.cfg.Now()
		timer := time.NewTimer(now.Truncate(time.Minute).Add(time.Minute).Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.kick:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// Check fires the alarms due since the last check, and the snoozes that
// ended, and announces them: an event for the app, or notifications while
// it is away. It also withdraws the notes of reminders no longer shown.
// Run calls it; tests call it with their clock.
func (s *Scheduler) Check(ctx context.Context) error {
	now := s.cfg.Now()
	if s.cfg.Messages != nil {
		// Apart from the calendar's, so neither holds the other up.
		if err := s.cfg.Messages(ctx, now); err != nil && ctx.Err() == nil {
			s.cfg.Log.Warn("message reminders not fired", "err", err)
		}
	}
	since := s.last
	if since.IsZero() {
		var err error
		if since, err = s.resume(ctx, now); err != nil {
			return err
		}
	}
	due, err := s.cfg.DB.AlarmsDue(ctx, since, now, s.cfg.Local)
	if err != nil {
		return err
	}
	ended, err := s.cfg.DB.SnoozesEnded(ctx, since, now)
	if err != nil {
		return err
	}
	err = s.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.FireReminders(ctx, due, now); err != nil {
			return err
		}
		return tx.SetRemindersChecked(ctx, now)
	})
	if err != nil {
		return fmt.Errorf("fire reminders: %w", err)
	}
	s.last = now
	list, err := s.cfg.List(ctx)
	if err != nil {
		return err
	}
	fresh := map[string]bool{}
	for _, k := range append(due, ended...) {
		fresh[engine.ReminderID(k)] = true
	}
	return s.announce(ctx, list, fresh, now)
}

// resume is where the first check after a start looks from: the last
// check recorded before maild stopped, at most catchUp ago. A database
// that never checked has nothing to catch up on.
func (s *Scheduler) resume(ctx context.Context, now time.Time) (time.Time, error) {
	checked, ok, err := s.cfg.DB.RemindersChecked(ctx)
	if err != nil {
		return now, err
	}
	if !ok {
		return now, nil
	}
	if floor := now.Add(-catchUp); checked.Before(floor) {
		return floor, nil
	}
	return checked, nil
}

func (s *Scheduler) announce(ctx context.Context, list []api.Reminder, fresh map[string]bool, now time.Time) error {
	active := map[string]bool{}
	var notes []notify.Reminder
	for _, r := range list {
		active[r.ID] = true
		if fresh[r.ID] {
			notes = append(notes, notify.Reminder{ID: r.ID, Summary: r.Summary, Body: s.body(r, now)})
		}
	}
	if len(notes) > 0 {
		err := s.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
			return tx.Emit(ctx, api.CalendarReminders{Count: int64(len(list))})
		})
		if err != nil {
			return err
		}
	}
	if s.cfg.Notifier == nil {
		return nil
	}
	if len(notes) > 0 && (s.cfg.Attended == nil || !s.cfg.Attended()) {
		if err := s.cfg.Notifier.Remind(ctx, notes); err != nil {
			return err
		}
	}
	return s.cfg.Notifier.Withdraw(ctx, active)
}

// body is a note's second line: when, relative to today, and where.
func (s *Scheduler) body(r api.Reminder, now time.Time) string {
	start := r.Start.In(s.cfg.Local)
	when := "All day"
	if !r.AllDay {
		when = start.Format("3:04 PM")
	}
	day := r.StartDate
	if !r.AllDay {
		day = start.Format(time.DateOnly)
	}
	switch today := now.In(s.cfg.Local); day {
	case today.Format(time.DateOnly):
		when = "Today, " + when
	case today.AddDate(0, 0, 1).Format(time.DateOnly):
		when = "Tomorrow, " + when
	default:
		date, _ := time.Parse(time.DateOnly, day)
		when = date.Format("Mon Jan 2") + ", " + when
	}
	if r.Location != "" {
		return when + " · " + r.Location
	}
	return when
}
