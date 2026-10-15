// Command schedprobe finds out whether a CalDAV provider tells invitees
// about an event a client writes as its organizer, and whether
// SCHEDULE-AGENT keeps it quiet (ADR-0034's SchedulesOrganizer; plan 0009,
// Phase 5). It writes a probe event with the given invitees into one of an
// account's calendars, then moves it an hour, gives it an alert and Free
// (what invitees do not see), removes invitees or deletes it, and the
// invitees' inboxes show what the server sent for each step.
//
// A probe's UID starts with "fm-probe-" and its summary with "FM probe";
// update and delete touch nothing else. The account is the calendar's
// organizer. It reads a snapshot of maild's database, removed when it ends,
// and the keyring, and writes only to the server: maild's next pass brings
// the probe in.
//
//	go run ./tools/schedprobe -calendar 218 -agent client -invite a@example.com create "FM probe client"
//	go run ./tools/schedprobe -calendar 218 update fm-probe-…
//	go run ./tools/schedprobe -calendar 218 quiet fm-probe-…
//	go run ./tools/schedprobe -calendar 218 -invite a@example.com uninvite fm-probe-…
//	go run ./tools/schedprobe -calendar 218 delete fm-probe-…
//	go run ./tools/schedprobe -calendar 218 show fm-probe-…
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/contentline"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "schedprobe:", err)
		os.Exit(1)
	}
}

type invites []string

func (i *invites) String() string { return strings.Join(*i, ",") }

func (i *invites) Set(v string) error {
	*i = append(*i, strings.ToLower(strings.TrimSpace(v)))
	return nil
}

type options struct {
	calendar int64
	agent    string
	invite   invites
	start    string
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("schedprobe", flag.ContinueOnError)
	var o options
	fs.Int64Var(&o.calendar, "calendar", 0, "the calendar (a collection ID) to write in (required)")
	fs.StringVar(&o.agent, "agent", "server", "create: server (no SCHEDULE-AGENT), client or none on each ATTENDEE")
	fs.Var(&o.invite, "invite", "create and uninvite: an invitee's address (repeatable)")
	fs.StringVar(&o.start, "start", "20261027T150000Z", "create: the start, in UTC; the probe lasts an hour")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if o.calendar == 0 || fs.NArg() != 2 {
		return errors.New("usage: schedprobe -calendar ID [flags] create SUMMARY | update UID | quiet UID | uninvite UID | delete UID | show UID")
	}
	verb, arg := fs.Arg(0), fs.Arg(1)
	p, cleanup, err := open(ctx, o.calendar)
	if err != nil {
		return err
	}
	defer cleanup()
	switch verb {
	case "create":
		return p.create(ctx, out, arg, o)
	case "update":
		return p.update(ctx, out, arg)
	case "delete":
		return p.delete(ctx, out, arg)
	case "quiet":
		return p.edit(ctx, out, arg, "added an alert and Free", quiet)
	case "uninvite":
		return p.edit(ctx, out, arg, "removed "+o.invite.String(), func(lines []string) []string { return uninvite(lines, o.invite) })
	case "show":
		obj, err := p.probeObject(ctx, arg)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s (%s):\n%s", obj.Href, obj.ETag, obj.Data)
		return nil
	}
	return fmt.Errorf("unknown step %q", verb)
}

type probe struct {
	acct store.Account
	col  store.Collection
	dav  *davx.Client
}

// open reads the calendar, its account and their credentials from a
// snapshot of maild's database.
func open(ctx context.Context, calendarID int64) (*probe, func(), error) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return nil, nil, err
	}
	tmp, err := os.MkdirTemp("", "schedprobe-")
	if err != nil {
		return nil, nil, err
	}
	var db *store.DB
	cleanup := func() {
		if db != nil {
			_ = db.Close()
		}
		_ = os.RemoveAll(tmp)
	}
	fail := func(err error) (*probe, func(), error) {
		cleanup()
		return nil, nil, err
	}
	snap := filepath.Join(tmp, "snapshot.db")
	if err := snapshot(ctx, paths.DB, snap); err != nil {
		return fail(err)
	}
	if db, err = store.Open(ctx, snap); err != nil {
		return fail(err)
	}
	col, err := db.GetCollection(ctx, calendarID)
	if err != nil {
		return fail(fmt.Errorf("calendar %d: %w", calendarID, err))
	}
	if col.Kind != api.CollectionKindCalendar || col.ReadOnly {
		return fail(fmt.Errorf("collection %d is not a writable calendar", calendarID))
	}
	acct, err := db.GetAccount(ctx, col.AccountID)
	if err != nil {
		return fail(err)
	}
	services, err := db.Services(ctx, acct.ID)
	if err != nil {
		return fail(err)
	}
	home := ""
	for _, s := range services {
		if s.Service == api.ServiceKindCalendar {
			home = s.Home
		}
	}
	if home == "" {
		return fail(fmt.Errorf("account %d's calendar home is not discovered yet", acct.ID))
	}
	// The secrets file is the snapshot's: what Open would move from it is
	// nothing, so the user's keyring is only read.
	sec, err := secrets.Open(ctx, filepath.Join(tmp, "secrets.json"), log)
	if err != nil {
		return fail(err)
	}
	profile := providers.ForKind(acct.Kind)
	dav, err := davx.New(home, davx.Options{
		Authorization: authorization(db, sec, acct, log),
		Trusted:       func(u *url.URL) bool { return profile.TrustsHost(u.Hostname()) },
		UserAgent:     "frostmail-schedprobe",
	})
	if err != nil {
		return fail(err)
	}
	return &probe{acct: acct, col: col, dav: dav}, cleanup, nil
}

func authorization(db *store.DB, sec secrets.Store, acct store.Account, log *slog.Logger) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		if acct.Auth == api.AuthKindOAuth2 {
			tok, err := (&oauth.Manager{DB: db, Secrets: sec, Log: log}).AccessToken(ctx, acct.ID)
			return "Bearer " + tok, err
		}
		password, err := sec.Get(ctx, secrets.AccountPassword(acct.ID))
		if err != nil {
			return "", err
		}
		user := acct.IMAP.Username
		if user == "" {
			user = acct.Email
		}
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password)), nil
	}
}

// snapshot copies the live database with VACUUM INTO over a read-only
// connection, so maild's database is neither migrated nor written.
func snapshot(ctx context.Context, live, to string) error {
	src, err := sql.Open("sqlite", "file:"+live+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	if _, err := src.ExecContext(ctx, `VACUUM INTO ?`, to); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	return nil
}

var agents = map[string]string{"server": "", "client": "CLIENT", "none": "NONE"}

func (p *probe) create(ctx context.Context, out io.Writer, summary string, o options) error {
	if !strings.HasPrefix(summary, "FM probe") {
		return errors.New(`a probe's summary starts with "FM probe"`)
	}
	agent, ok := agents[o.agent]
	if !ok {
		return fmt.Errorf("agent %q is not server, client or none", o.agent)
	}
	if len(o.invite) == 0 {
		return errors.New("a probe invites someone: -invite")
	}
	start, err := time.Parse("20060102T150405Z", o.start)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	uid := "fm-probe-" + hex.EncodeToString(b[:])
	stamp := time.Now().UTC().Format("20060102T150405Z")
	props := []contentline.Prop{
		{Name: "BEGIN", Value: "VCALENDAR"}, {Name: "VERSION", Value: "2.0"},
		{Name: "PRODID", Value: "-//Frostyard//Frostmail schedprobe//EN"},
		{Name: "BEGIN", Value: "VEVENT"}, {Name: "UID", Value: uid},
		{Name: "DTSTAMP", Value: stamp}, {Name: "CREATED", Value: stamp}, {Name: "LAST-MODIFIED", Value: stamp},
		{Name: "SEQUENCE", Value: "0"},
		{Name: "DTSTART", Value: start.Format("20060102T150405Z")},
		{Name: "DTEND", Value: start.Add(time.Hour).Format("20060102T150405Z")},
		{Name: "SUMMARY", Value: contentline.EscapeText(summary)},
		{Name: "DESCRIPTION", Value: contentline.EscapeText("Frostmail's scheduling probe (agent " + o.agent + "); it deletes this event.")},
		{Name: "ORGANIZER", Params: cn(p.acct.DisplayName), Value: "mailto:" + p.acct.Email},
	}
	for _, addr := range o.invite {
		params := []contentline.Param{{Name: "ROLE", Values: []string{"REQ-PARTICIPANT"}},
			{Name: "PARTSTAT", Values: []string{"NEEDS-ACTION"}}, {Name: "RSVP", Values: []string{"TRUE"}}}
		if agent != "" {
			params = append(params, contentline.Param{Name: "SCHEDULE-AGENT", Values: []string{agent}})
		}
		props = append(props, contentline.Prop{Name: "ATTENDEE", Params: params, Value: "mailto:" + addr})
	}
	props = append(props, contentline.Prop{Name: "END", Value: "VEVENT"}, contentline.Prop{Name: "END", Value: "VCALENDAR"})
	var raw strings.Builder
	for _, prop := range props {
		raw.WriteString(prop.Encode("\r\n"))
	}
	href := strings.TrimSuffix(p.col.Href, "/") + "/" + uid + ".ics"
	w, err := p.dav.Put(ctx, href, []byte(raw.String()), davx.Calendars, "")
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s in %q (account %d, %s) at %s, agent %s, inviting %s\n", uid, p.col.Name, p.acct.ID, p.acct.Email,
		start.Format(time.RFC3339), o.agent, strings.Join(o.invite, ", "))
	return p.show(ctx, out, cmpOr(w.Href, href))
}

func cn(name string) []contentline.Param {
	if name == "" {
		return nil
	}
	return []contentline.Param{{Name: "CN", Values: []string{name}}}
}

// update moves the probe an hour later and raises its SEQUENCE.
func (p *probe) update(ctx context.Context, out io.Writer, uid string) error {
	obj, err := p.probeObject(ctx, uid)
	if err != nil {
		return err
	}
	src := string(obj.Data)
	later := func(prefix string) error {
		m := regexp.MustCompile(`(?m)^` + prefix + `:(\d{8}T\d{6}Z)\r?$`).FindStringSubmatchIndex(src)
		if m == nil {
			return fmt.Errorf("the probe has no UTC %s", prefix)
		}
		t, err := time.Parse("20060102T150405Z", src[m[2]:m[3]])
		if err != nil {
			return err
		}
		src = src[:m[2]] + t.Add(time.Hour).Format("20060102T150405Z") + src[m[3]:]
		return nil
	}
	if err := later("DTSTART"); err != nil {
		return err
	}
	if err := later("DTEND"); err != nil {
		return err
	}
	if m := regexp.MustCompile(`(?m)^SEQUENCE:(\d+)\r?$`).FindStringSubmatchIndex(src); m != nil {
		n, _ := strconv.Atoi(src[m[2]:m[3]])
		src = src[:m[2]] + strconv.Itoa(n+1) + src[m[3]:]
	}
	if _, err := p.dav.Put(ctx, obj.Href, []byte(src), davx.Calendars, obj.ETag); err != nil {
		return err
	}
	fmt.Fprintf(out, "moved %s an hour later\n", uid)
	return p.show(ctx, out, obj.Href)
}

// edit rewrites the probe's unfolded lines with change, without raising
// its SEQUENCE.
func (p *probe) edit(ctx context.Context, out io.Writer, uid, what string, change func([]string) []string) error {
	obj, err := p.probeObject(ctx, uid)
	if err != nil {
		return err
	}
	unfold := strings.NewReplacer("\r\n ", "", "\r\n\t", "")
	lines := change(strings.Split(strings.TrimRight(unfold.Replace(string(obj.Data)), "\r\n"), "\r\n"))
	var src strings.Builder
	for _, line := range lines {
		src.WriteString(contentline.Fold(line, "\r\n"))
	}
	if _, err := p.dav.Put(ctx, obj.Href, []byte(src.String()), davx.Calendars, obj.ETag); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: %s\n", uid, what)
	return p.show(ctx, out, obj.Href)
}

// quiet gives the event a display alert and makes it Free, the changes
// ADR-0034 says tell nobody.
func quiet(lines []string) []string {
	var out []string
	for _, line := range lines {
		if strings.HasPrefix(line, "TRANSP:") {
			continue
		}
		if line == "END:VEVENT" {
			out = append(out, "TRANSP:TRANSPARENT", "BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:FM probe",
				"TRIGGER:-PT15M", "END:VALARM")
		}
		out = append(out, line)
	}
	return out
}

// uninvite removes the ATTENDEE lines of the addresses.
func uninvite(lines []string, addrs []string) []string {
	var out []string
	for _, line := range lines {
		if strings.HasPrefix(line, "ATTENDEE") {
			at := strings.LastIndex(strings.ToLower(line), "mailto:")
			if at >= 0 && slices.Contains(addrs, strings.ToLower(line[at+len("mailto:"):])) {
				continue
			}
		}
		out = append(out, line)
	}
	return out
}

func (p *probe) delete(ctx context.Context, out io.Writer, uid string) error {
	obj, err := p.probeObject(ctx, uid)
	if err != nil {
		return err
	}
	if err := p.dav.Delete(ctx, obj.Href, obj.ETag); err != nil {
		return err
	}
	fmt.Fprintf(out, "deleted %s\n", uid)
	return nil
}

// probeObject is a probe this tool wrote, read from the server.
func (p *probe) probeObject(ctx context.Context, uid string) (davx.Object, error) {
	if !regexp.MustCompile(`^fm-probe-[0-9a-f]{16}$`).MatchString(uid) {
		return davx.Object{}, errors.New("only a probe's UID (fm-probe-…) is updated or deleted")
	}
	obj, err := p.dav.Get(ctx, strings.TrimSuffix(p.col.Href, "/")+"/"+uid+".ics")
	if err != nil {
		return davx.Object{}, err
	}
	if !strings.Contains(string(obj.Data), "SUMMARY:FM probe") {
		return davx.Object{}, fmt.Errorf("%s is not a probe", uid)
	}
	return obj, nil
}

// show prints the server's copy's scheduling lines, to see what it kept.
func (p *probe) show(ctx context.Context, out io.Writer, href string) error {
	obj, err := p.dav.Get(ctx, href)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "the server's copy (%s):\n", obj.ETag)
	unfold := strings.NewReplacer("\r\n ", "", "\r\n\t", "")
	for _, line := range strings.Split(unfold.Replace(string(obj.Data)), "\r\n") {
		for _, name := range []string{"ORGANIZER", "ATTENDEE", "SEQUENCE", "DTSTART", "X-"} {
			if strings.HasPrefix(line, name) {
				fmt.Fprintln(out, "  "+line)
				break
			}
		}
	}
	return nil
}

func cmpOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
