package settings

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
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

func TestLoaderDropsAStaleAnswer(t *testing.T) {
	loop := frostuitest.NewLoop()
	cn := &session{}
	cn.ctx, cn.cancel = context.WithCancel(t.Context())
	defer cn.cancel()
	w := &Window{o: Options{Post: loop.Post}, cn: cn}
	var l loader
	release := make(chan struct{})
	var applied []string
	l.run(w, func(context.Context, *api.Client) func() {
		<-release
		return func() { applied = append(applied, "old") }
	})
	l.run(w, func(context.Context, *api.Client) func() {
		return func() { applied = append(applied, "new") }
	})
	close(release)
	loop.RunUntil(t, func() bool { return len(applied) > 0 })
	if !slices.Equal(applied, []string{"new"}) {
		t.Errorf("applied = %q", applied)
	}
}

func TestSaveStatus(t *testing.T) {
	for _, c := range []struct {
		flatpak  bool
		from, to string
		want     string
	}{
		{true, "", testLodge, statusStopping},
		{true, testLodge, "", statusStarting},
		{true, testLodge, "https://other.ts.net", ""},
		{true, "", "", ""},
		{false, "", testLodge, ""},
		{false, testLodge, "", ""},
	} {
		if got := saveStatus(c.flatpak, c.from, c.to); got != c.want {
			t.Errorf("saveStatus(%v, %q, %q) = %q, want %q", c.flatpak, c.from, c.to, got, c.want)
		}
	}
}

func TestSignatureHTML(t *testing.T) {
	for in, want := range map[string]string{
		"":          "",
		"   \n  ":   "",
		"Ann":       "<p>Ann</p>",
		"a\nb":      "<p>a</p><p>b</p>",
		"a\r\nb\rc": "<p>a</p><p>b</p><p>c</p>",
		"a\n\nb":    "<p>a</p><p><br></p><p>b</p>",
		"a\n\n  \n": "<p>a</p>",
		`<&>"`:      "<p>&lt;&amp;&gt;&quot;</p>",
	} {
		if got := signatureHTML(in); got != want {
			t.Errorf("signatureHTML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSignatureText(t *testing.T) {
	for in, want := range map[string]string{
		"":                                  "",
		"<p>a</p><p>b</p>":                  "a\nb",
		"<p>a</p><p><br></p><p>b</p>":       "a\n\nb",
		"<div>a<br>b</div>":                 "a\nb",
		"plain <b>bold</b> text":            "plain bold text",
		"inline<br>next":                    "inline\nnext",
		"<p>&lt;&amp;</p>":                  "<&",
		"before<p>block</p>after":           "before\nblock\nafter",
		"<ul><li>one</li><li>two</li></ul>": "onetwo",
		"<h1>Title</h1><blockquote>q</blockquote><pre>x</pre>": "Title\nq\nx",
	} {
		if got := signatureText(in); got != want {
			t.Errorf("signatureText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSignatureRoundTrip(t *testing.T) {
	for _, text := range []string{"Ann", "a\n\nb", "x < y & z"} {
		if got := signatureText(signatureHTML(text)); got != text {
			t.Errorf("round trip of %q = %q", text, got)
		}
	}
}

func TestDefaultPorts(t *testing.T) {
	for _, c := range []struct {
		id   string
		tls  api.TLSMode
		want int64
	}{
		{"imap", api.TLSModeTLS, 993}, {"imap", api.TLSModeStartTLS, 143}, {"imap", api.TLSModeInsecure, 143},
		{"smtp", api.TLSModeTLS, 465}, {"smtp", api.TLSModeStartTLS, 587}, {"smtp", api.TLSModeInsecure, 25},
	} {
		if got := defaultPort(c.id, c.tls); got != c.want {
			t.Errorf("defaultPort(%s, %s) = %d, want %d", c.id, c.tls, got, c.want)
		}
	}
}

func TestWithTLSMovesOnlyADefaultPort(t *testing.T) {
	s := api.ServerConfig{Port: 993, TLS: api.TLSModeTLS}
	if got := withTLS("imap", s, api.TLSModeStartTLS); got.Port != 143 || got.TLS != api.TLSModeStartTLS {
		t.Errorf("from a default: %+v", got)
	}
	s.Port = 0
	if got := withTLS("imap", s, api.TLSModeStartTLS); got.Port != 143 {
		t.Errorf("from 0: %+v", got)
	}
	s.Port = 1143
	if got := withTLS("imap", s, api.TLSModeStartTLS); got.Port != 1143 {
		t.Errorf("a port of the user's: %+v", got)
	}
}

func TestParsePort(t *testing.T) {
	for in, want := range map[string]int64{"": 0, "143": 143, "99a": 99, "x": 0, " 25 ": 25} {
		if got := parsePort(in); got != want {
			t.Errorf("parsePort(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestServicesByKind(t *testing.T) {
	all := []api.ServiceKind{api.ServiceKindContacts, api.ServiceKindCalendar, api.ServiceKindTasks}
	two := all[:2]
	for _, c := range []struct {
		kind              api.AccountKind
		offered, defaults []api.ServiceKind
	}{
		{api.AccountKindIMAP, all, nil},
		{api.AccountKindGmail, all, all},
		{api.AccountKindICloud, two, two},
		{api.AccountKindMicrosoft, two, two},
	} {
		if got := offeredServices(c.kind); !slices.Equal(got, c.offered) {
			t.Errorf("offeredServices(%s) = %v", c.kind, got)
		}
		if got := defaultServices(c.kind); !slices.Equal(got, c.defaults) {
			t.Errorf("defaultServices(%s) = %v", c.kind, got)
		}
	}
}

func TestValidEmail(t *testing.T) {
	for in, want := range map[string]bool{"a@b": true, "a b@c": false, "@b": false, "a@": false, "a@b@c": false, "plain": false, "": false} {
		if got := validEmail(in); got != want {
			t.Errorf("validEmail(%q) = %v", in, got)
		}
	}
}

func TestJoinNames(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{"Contacts"}, "Contacts"},
		{[]string{"Contacts", "Calendars"}, "Contacts and Calendars"},
		{[]string{"Contacts", "Calendars", "Tasks"}, "Contacts, Calendars and Tasks"},
	} {
		if got := joinNames(c.in); got != c.want {
			t.Errorf("joinNames(%v) = %q", c.in, got)
		}
	}
}
