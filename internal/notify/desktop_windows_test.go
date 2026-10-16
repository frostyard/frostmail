package notify

import (
	"bufio"
	"encoding/json/v2"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestMain doubles as a stand-in notifier: run with FM_FAKE_NOTIFIER set,
// the test binary appends the operation it reads to that file.
func TestMain(m *testing.M) {
	if out := os.Getenv("FM_FAKE_NOTIFIER"); out != "" {
		op, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(2)
		}
		f, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(3)
		}
		_, _ = f.Write(append(op, '\n'))
		_ = f.Close()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeNotifier(t *testing.T) (*Desktop, func() []toastOp) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "ops.jsonl")
	t.Setenv("FM_FAKE_NOTIFIER", out)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return NewDesktop(exe, slog.New(slog.DiscardHandler)), func() []toastOp {
		f, err := os.Open(out)
		if err != nil {
			return nil
		}
		defer func() { _ = f.Close() }()
		var ops []toastOp
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var op toastOp
			if err := json.Unmarshal(sc.Bytes(), &op); err != nil {
				t.Fatal(err)
			}
			ops = append(ops, op)
		}
		return ops
	}
}

func TestNotifierShowsMailAndReminders(t *testing.T) {
	d, ops := fakeNotifier(t)
	ctx := t.Context()
	if err := d.Announce(ctx, 7, []Mail{{ID: 41, FromName: "Ann", Subject: "Lunch"}}); err != nil {
		t.Fatal(err)
	}
	many := []Mail{{ID: 1, FromName: "A"}, {ID: 2, FromName: "B"}, {ID: 3, FromName: "C"}, {ID: 4, FromName: "D"}}
	if err := d.Announce(ctx, 7, many); err != nil {
		t.Fatal(err)
	}
	if err := d.Remind(ctx, []Reminder{{ID: "2026-10-20T13:00:00Z/event-9", Summary: "Standup", Body: "9:00 AM"}, {ID: "r2", Summary: "Review"}}); err != nil {
		t.Fatal(err)
	}
	if err := d.Withdraw(ctx, map[string]bool{"r2": true}); err != nil {
		t.Fatal(err)
	}
	got := ops()
	want := []toastOp{
		{Op: "show", Tag: "m41", Group: "mail", Title: "Ann", Body: "Lunch", Open: 41},
		{Op: "show", Tag: "a7", Group: "mail", Title: "4 new messages", Body: Notes(many)[0].Body},
		{Op: "show", Tag: reminderTag("2026-10-20T13:00:00Z/event-9"), Group: "reminders", Title: "Standup", Body: "9:00 AM",
			Reminder: "2026-10-20T13:00:00Z/event-9"},
		{Op: "show", Tag: reminderTag("r2"), Group: "reminders", Title: "Review", Reminder: "r2"},
		{Op: "remove", Tag: reminderTag("2026-10-20T13:00:00Z/event-9"), Group: "reminders"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("ops = %+v\nwant %+v", got, want)
	}
	for _, op := range got {
		if len(op.Tag) > 64 {
			t.Errorf("tag %q is longer than a toast's 64", op.Tag)
		}
	}
}

func TestNoNotifierNoNotifications(t *testing.T) {
	t.Setenv("FROSTMAIL_NOTIFIER", filepath.Join(t.TempDir(), "missing.exe"))
	if d, err := OpenDesktop(t.Context(), slog.New(slog.DiscardHandler), nil); err == nil {
		t.Fatalf("OpenDesktop without a notifier = %+v", d)
	}
}
