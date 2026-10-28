// Command davsession records a scripted write session (internal/davsession)
// against a provider's servers, for a replay test (docs/design/testing.md,
// DAV and Tasks recordings). It signs in as one of maild's accounts, with
// the credentials a snapshot of maild's database and the keyring hold, but
// syncs into a new store of its own, so the trace starts with a first
// sync, as a replay test's account does. Each step is one write and the
// pass that sends it, on a clock stopped at the start.
//
// The raw trace holds the account's contacts, calendars and tasks: scrub
// it with tools/davrec before it leaves the machine. The session deletes
// what it makes; when it fails partway, it says what may be left.
//
//	go run ./tools/davsession -account 6 -o /tmp/session.trace google
//	go run ./tools/davsession -account 7 -o /tmp/session.trace microsoft
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/davsession"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/httprec"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "davsession:", err)
		os.Exit(1)
	}
}

// sessions are the scripts by name, with the account kind each needs.
var sessions = map[string]struct {
	kind   api.AccountKind
	zone   string
	script func(context.Context, davsession.Session) error
}{
	"google":    {api.AccountKindGmail, davsession.GoogleZone, davsession.Google},
	"microsoft": {api.AccountKindMicrosoft, davsession.MicrosoftZone, davsession.Microsoft},
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("davsession", flag.ContinueOnError)
	account := fs.Int64("account", 0, "maild's account to sign in as (required)")
	trace := fs.String("o", "", "the raw trace to write (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	session, ok := sessions[fs.Arg(0)]
	if *account == 0 || *trace == "" || fs.NArg() != 1 || !ok {
		return errors.New("usage: davsession -account ID -o TRACE google|microsoft")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	tmp, err := os.MkdirTemp("", "davsession-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return err
	}
	snap := filepath.Join(tmp, "snapshot.db")
	if err := snapshot(ctx, paths.DB, snap); err != nil {
		return err
	}
	creds, err := store.Open(ctx, snap)
	if err != nil {
		return err
	}
	defer func() { _ = creds.Close() }()
	acct, err := creds.GetAccount(ctx, *account)
	if err != nil {
		return fmt.Errorf("account %d: %w", *account, err)
	}
	if acct.Kind != session.kind || acct.Auth != api.AuthKindOAuth2 {
		return fmt.Errorf("account %d is %s with %s sign-in; the session needs %s with OAuth", acct.ID, acct.Kind, acct.Auth, session.kind)
	}
	// The secrets file is the snapshot's: the user's keyring is only read.
	// A maild that keeps its secrets in a file (FROSTMAIL_SECRETS=file, a
	// dev maild's) has them copied.
	if os.Getenv("FROSTMAIL_SECRETS") == "file" {
		if err := copyFile(filepath.Join(paths.DataDir, "secrets.json"), filepath.Join(tmp, "secrets.json")); err != nil {
			return err
		}
	}
	sec, err := secrets.Open(ctx, filepath.Join(tmp, "secrets.json"), log, secrets.HostOptions())
	if err != nil {
		return err
	}
	tokens := signedIn{m: &oauth.Manager{DB: creds, Secrets: sec, Log: log}, id: acct.ID}

	db, err := store.Open(ctx, filepath.Join(tmp, "session.db"))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	own, err := newAccount(ctx, db, acct)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(*trace, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	start := time.Now().UTC().Truncate(time.Second)
	local, err := time.LoadLocation(session.zone)
	if err != nil {
		return err
	}
	transport := &httprec.Transport{W: f, OnError: func(err error) { log.Warn("trace", "err", err) }}
	m := pimsync.New(db, secrets.NewFile(filepath.Join(tmp, "session-secrets.json")), log, pimsync.Config{
		Tokens:    tokens,
		HTTP:      &http.Client{Timeout: time.Minute, Transport: transport},
		UserAgent: "Frostmail/davsession",
		Now:       func() time.Time { return start },
		Local:     local,
	})
	eng := engine.New(engine.Deps{DB: db, PIM: m})
	fmt.Fprintf(out, "recording %s as %s, the clock at %s\n", fs.Arg(0), acct.Email, start.Format(time.RFC3339))
	err = session.script(ctx, davsession.Session{
		Calendar: eng.Calendar(),
		People:   eng.People(),
		Pass:     func(ctx context.Context) error { return m.Pass(ctx, own) },
		Step:     func(name string) { fmt.Fprintln(out, "  "+name) },
	})
	if err != nil {
		return fmt.Errorf("%w (the account may keep \"Replay\" events or a contact it made: delete them in Frostmail)", err)
	}
	fmt.Fprintf(out, "wrote %s; scrub it with:\n  go run ./tools/davrec -account %s -note %q -o SCRUBBED %s\n",
		*trace, acct.Email, "recorded "+start.Format(time.RFC3339), *trace)
	return nil
}

// signedIn signs the session's own account in as maild's.
type signedIn struct {
	m  *oauth.Manager
	id int64
}

func (a signedIn) AccessToken(ctx context.Context, _ int64) (string, error) {
	return a.m.AccessToken(ctx, a.id)
}

func (a signedIn) AccessTokenFor(ctx context.Context, _ int64, audience string) (string, error) {
	return a.m.AccessTokenFor(ctx, a.id, audience)
}

func (a signedIn) Invalidate(int64) { a.m.Invalidate(a.id) }

// newAccount adds the account to the session's store with its services on
// at the provider's URLs (Graph's for a Microsoft account) and not yet
// discovered, as a replay test's is.
func newAccount(ctx context.Context, db *store.DB, from store.Account) (int64, error) {
	dav := providers.ForKind(from.Kind).DAV
	var id int64
	err := db.Tx(ctx, func(tx *store.Tx) error {
		acct, err := tx.InsertAccount(ctx, store.Account{Kind: from.Kind, Email: from.Email, Auth: from.Auth,
			IMAP: from.IMAP, SMTP: from.SMTP, DisplayName: from.DisplayName})
		if err != nil {
			return err
		}
		id = acct.ID
		if err := tx.SetGrantedScopes(ctx, id, from.GrantedScopes); err != nil {
			return err
		}
		starts := map[api.ServiceKind]string{api.ServiceKindContacts: dav.Contacts, api.ServiceKindCalendar: dav.Calendar}
		if dav.Graph {
			starts = map[api.ServiceKind]string{api.ServiceKindContacts: providers.GraphURL, api.ServiceKindCalendar: providers.GraphURL}
		}
		if dav.Tasks == "google" {
			starts[api.ServiceKindTasks] = providers.GoogleTasksURL
		}
		for s, u := range starts {
			if err := tx.SetService(ctx, id, s, true, u); err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o600)
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
