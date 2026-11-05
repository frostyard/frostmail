//go:build linux

package problemstest

import (
	"fmt"
	"testing"

	"github.com/frostyard/std/problems"
)

// fatalTB is a testing.TB whose Fatal and Fatalf only record that they ran,
// so a test can see None fail.
type fatalTB struct {
	testing.TB
	failed []string
}

func (f *fatalTB) Helper() {}

func (f *fatalTB) Fatal(args ...any) { f.failed = append(f.failed, fmt.Sprint(args...)) }

func (f *fatalTB) Fatalf(format string, args ...any) {
	f.failed = append(f.failed, fmt.Sprintf(format, args...))
}

// None fails when a record waits in the socket, and passes when none does:
// every negative check of a problem record rests on it.
func TestNoneSeesAWaitingRecord(t *testing.T) {
	j := Listen(t)
	empty := &fatalTB{TB: t}
	j.None(empty)
	if len(empty.failed) != 0 {
		t.Fatalf("None of an empty socket failed: %q", empty.failed)
	}

	j.Recorder(t, problems.ComponentDaemon).Error("test.none_guard", nil)
	waiting := &fatalTB{TB: t}
	j.None(waiting)
	if len(waiting.failed) != 1 {
		t.Fatalf("None with a waiting record failed %d times, want once: %q", len(waiting.failed), waiting.failed)
	}

	// None took the record: the socket is empty again.
	again := &fatalTB{TB: t}
	j.None(again)
	if len(again.failed) != 0 {
		t.Fatalf("None after the record was read failed: %q", again.failed)
	}
}
