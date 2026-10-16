package notify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// The Windows sender (ADR-0038; docs/design/desktop.md, Notifications):
// maild runs frostmail-notify.exe, which the app's build ships beside it,
// once per change, with one JSON object (a toastOp) on stdin. The notifier
// shows and removes Windows toasts under Frostmail's AppUserModelID.
// Clicking a toast, or its Snooze and Dismiss, is the notifier's and the
// app's to handle (opening the message, calling calendar.snooze or
// calendar.dismiss over maild's pipe), so open and OnReminder go unused.

// NotifierName is the notifier's file name, looked for beside maild.exe.
const NotifierName = "frostmail-notify.exe"

// runLimit bounds one run of the notifier.
const runLimit = 10 * time.Second

// toastOp is what the notifier reads: show a toast, replacing one with the
// same tag and group, or remove one.
type toastOp struct {
	Op    string `json:"op"` // show or remove
	Tag   string `json:"tag"`
	Group string `json:"group"` // mail or reminders
	Title string `json:"title,omitzero"`
	Body  string `json:"body,omitzero"`
	// Open is the message a click opens; Reminder the calendar.reminders
	// id the toast's Snooze and Dismiss act on.
	Open     int64  `json:"open,omitzero"`
	Reminder string `json:"reminder,omitzero"`
}

// Desktop sends notifications through the notifier.
type Desktop struct {
	log     *slog.Logger
	command string // the notifier

	mu        sync.Mutex
	reminders map[string]bool // reminders with a toast up
}

// OpenDesktop finds the notifier: FROSTMAIL_NOTIFIER, else NotifierName
// beside maild. maild without one sends no notifications.
func OpenDesktop(_ context.Context, log *slog.Logger, _ func(messageID int64)) (*Desktop, error) {
	command := os.Getenv("FROSTMAIL_NOTIFIER")
	if command == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		command = filepath.Join(filepath.Dir(exe), NotifierName)
	}
	if _, err := os.Stat(command); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no notifier at %s", command)
		}
		return nil, err
	}
	return NewDesktop(command, log), nil
}

// NewDesktop sends through the notifier at command.
func NewDesktop(command string, log *slog.Logger) *Desktop {
	return &Desktop{log: log, command: command, reminders: map[string]bool{}}
}

// Close does nothing: each notification is a run of the notifier.
func (d *Desktop) Close() error { return nil }

// Announce shows the notes of one account's new mail (Notes). A group note
// replaces the account's previous group note.
func (d *Desktop) Announce(ctx context.Context, accountID int64, mail []Mail) error {
	for _, n := range Notes(mail) {
		tag := "m" + strconv.FormatInt(n.MessageID, 10)
		if n.MessageID == 0 {
			tag = "a" + strconv.FormatInt(accountID, 10)
		}
		if err := d.run(ctx, toastOp{Op: "show", Tag: tag, Group: "mail", Title: n.Summary, Body: n.Body, Open: n.MessageID}); err != nil {
			return err
		}
	}
	return nil
}

// OnReminder does nothing on Windows: the notifier calls maild's
// calendar.snooze and calendar.dismiss itself.
func (d *Desktop) OnReminder(func(id, action string)) {}

// Remind shows a toast per reminder, replacing one still shown for the same
// reminder.
func (d *Desktop) Remind(ctx context.Context, rs []Reminder) error {
	for _, r := range rs {
		if err := d.run(ctx, toastOp{Op: "show", Tag: reminderTag(r.ID), Group: "reminders", Title: r.Summary, Body: r.Body,
			Reminder: r.ID}); err != nil {
			return err
		}
		d.mu.Lock()
		d.reminders[r.ID] = true
		d.mu.Unlock()
	}
	return nil
}

// Withdraw removes the reminder toasts whose reminders are not in active:
// snoozed or dismissed elsewhere.
func (d *Desktop) Withdraw(ctx context.Context, active map[string]bool) error {
	d.mu.Lock()
	var gone []string
	for id := range d.reminders {
		if !active[id] {
			gone = append(gone, id)
			delete(d.reminders, id)
		}
	}
	d.mu.Unlock()
	for _, id := range gone {
		if err := d.run(ctx, toastOp{Op: "remove", Tag: reminderTag(id), Group: "reminders"}); err != nil {
			return err
		}
	}
	return nil
}

// reminderTag is a reminder's toast tag: a hash of its id, as a tag holds
// at most 64 characters.
func reminderTag(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "r" + hex.EncodeToString(sum[:16])
}

// run gives the notifier one operation, without a console window.
func (d *Desktop) run(ctx context.Context, op toastOp) error {
	in, err := json.Marshal(op)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, runLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.command)
	cmd.Stdin = bytes.NewReader(in)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("notify: %s: %w: %s", d.command, err, bytes.TrimSpace(out))
	}
	return nil
}
