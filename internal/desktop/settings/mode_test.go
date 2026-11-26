package settings

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/conn"
)

// fakeSteps record a change's calls, ported from mode.rs's tests.
type fakeSteps struct {
	prev    conn.AppConfig
	loadErr error
	stop    error
	start   error
	saves   []error // each save's error, in order; none writes
	calls   []string
	saved   []conn.AppConfig
}

func (f *fakeSteps) load() (conn.AppConfig, error) {
	f.calls = append(f.calls, "load")
	return f.prev, f.loadErr
}

func (f *fakeSteps) save(cfg conn.AppConfig) error {
	f.calls = append(f.calls, "save")
	f.saved = append(f.saved, cfg)
	if len(f.saves) == 0 {
		return nil
	}
	err := f.saves[0]
	f.saves = f.saves[1:]
	return err
}

func (f *fakeSteps) leaveMaild() { f.calls = append(f.calls, "leave") }

func (f *fakeSteps) stopMaild() error {
	f.calls = append(f.calls, "stop")
	return f.stop
}

func (f *fakeSteps) startMaild() error {
	f.calls = append(f.calls, "start")
	return f.start
}

const testLodge = "https://lodge.example.ts.net"

func lodgeCfg(url string) conn.AppConfig { return conn.AppConfig{Lodge: url} }

// M7: the compose windows save and close while this computer's maild
// still runs, before it stops.
func TestChangeLocalToLodgeWritesThenStopsMaild(t *testing.T) {
	f := &fakeSteps{}
	out := change(f, true, lodgeCfg(testLodge))
	if out.err != "" || !out.moved {
		t.Errorf("outcome = %+v", out)
	}
	if !slices.Equal(f.calls, []string{"load", "save", "leave", "stop"}) {
		t.Errorf("calls = %q", f.calls)
	}
	if !slices.Equal(f.saved, []conn.AppConfig{lodgeCfg(testLodge)}) {
		t.Errorf("saved = %v", f.saved)
	}
}

func TestChangeAMaildThatDoesNotStopKeepsThisComputer(t *testing.T) {
	f := &fakeSteps{stop: errors.New("maild still holds the lock")}
	out := change(f, true, lodgeCfg(testLodge))
	if out.err != didNotStop || out.moved {
		t.Errorf("outcome = %+v", out)
	}
	if !slices.Equal(f.calls, []string{"load", "save", "leave", "stop", "save", "start"}) {
		t.Errorf("calls = %q", f.calls)
	}
	if !slices.Equal(f.saved, []conn.AppConfig{lodgeCfg(testLodge), {}}) {
		t.Errorf("saved = %v", f.saved)
	}
}

func TestChangeAWriteBackThatFailsIsAddedToTheWords(t *testing.T) {
	f := &fakeSteps{stop: errors.New("maild still holds the lock"), saves: []error{nil, errors.New("app.json: read-only file system")}}
	out := change(f, true, lodgeCfg(testLodge))
	want := "This computer's maild did not stop. Frostmail still uses it. app.json: read-only file system"
	if out.err != want || out.moved {
		t.Errorf("outcome = %+v", out)
	}
	if !slices.Equal(f.calls, []string{"load", "save", "leave", "stop", "save"}) {
		t.Errorf("calls = %q", f.calls)
	}
}

func TestChangeLodgeToLocalWritesThenStartsMaild(t *testing.T) {
	f := &fakeSteps{prev: lodgeCfg(testLodge)}
	out := change(f, true, conn.AppConfig{})
	if out.err != "" || !out.moved {
		t.Errorf("outcome = %+v", out)
	}
	if !slices.Equal(f.calls, []string{"load", "save", "start"}) {
		t.Errorf("calls = %q", f.calls)
	}
}

func TestChangeAMaildThatDoesNotStartMovesAndSaysWhy(t *testing.T) {
	line := "frostmail: maild did not start through the Flatpak portal (flatpak-spawn); see /state/maild.log"
	f := &fakeSteps{prev: lodgeCfg(testLodge), start: errors.New(line)}
	out := change(f, true, conn.AppConfig{})
	if !out.moved || out.err != "This computer's maild did not start: "+line {
		t.Errorf("outcome = %+v", out)
	}
	if !slices.Equal(f.calls, []string{"load", "save", "start"}) {
		t.Errorf("calls = %q", f.calls)
	}
}

func TestChangeANewLodgeAddressOnlyWrites(t *testing.T) {
	f := &fakeSteps{prev: lodgeCfg(testLodge)}
	out := change(f, true, lodgeCfg("https://lodge.example.test.ts.net"))
	if out.err != "" || !out.moved {
		t.Errorf("outcome = %+v", out)
	}
	if !slices.Equal(f.calls, []string{"load", "save"}) {
		t.Errorf("calls = %q", f.calls)
	}
}

func TestChangeNoChangeOfModeOnlyWrites(t *testing.T) {
	f := &fakeSteps{}
	out := change(f, true, conn.AppConfig{})
	if out.err != "" || !out.moved || !slices.Equal(f.calls, []string{"load", "save"}) {
		t.Errorf("outcome %+v, calls %q", out, f.calls)
	}
}

func TestChangeOutsideTheFlatpakOnlyWrites(t *testing.T) {
	for _, next := range []conn.AppConfig{lodgeCfg(testLodge), {}} {
		f := &fakeSteps{prev: lodgeCfg(testLodge)}
		out := change(f, false, next)
		if out.err != "" || !out.moved || !slices.Equal(f.calls, []string{"save"}) || !slices.Equal(f.saved, []conn.AppConfig{next}) {
			t.Errorf("outcome %+v, calls %q, saved %v", out, f.calls, f.saved)
		}
	}
}

func TestChangeABadAddressChangesNothing(t *testing.T) {
	for _, flatpak := range []bool{true, false} {
		f := &fakeSteps{}
		out := change(f, flatpak, lodgeCfg("ftp://lodge.example.ts.net"))
		if out.moved || !strings.Contains(out.err, "lodge's address") || len(f.calls) != 0 {
			t.Errorf("outcome %+v, calls %q", out, f.calls)
		}
	}
}

func TestChangeAFailedWriteChangesNothing(t *testing.T) {
	f := &fakeSteps{saves: []error{errors.New("app.json: read-only file system")}}
	out := change(f, true, lodgeCfg(testLodge))
	if out.err != "app.json: read-only file system" || out.moved || !slices.Equal(f.calls, []string{"load", "save"}) {
		t.Errorf("outcome %+v, calls %q", out, f.calls)
	}
}

func TestChangeThatCannotReadTheChoiceChangesNothing(t *testing.T) {
	f := &fakeSteps{loadErr: errors.New("app.json: permission denied")}
	out := change(f, true, lodgeCfg(testLodge))
	if out.err != "app.json: permission denied" || out.moved || !slices.Equal(f.calls, []string{"load"}) {
		t.Errorf("outcome %+v, calls %q", out, f.calls)
	}
}
