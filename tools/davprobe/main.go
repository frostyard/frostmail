// Command davprobe compares, read-only, what a DAV server says of one of an
// account's collections each way maild asks (docs/design/pim.md,
// Operational notes): a PROPFIND at depth 1 (verify's listing), a
// sync-collection from no token and from the stored token (sync's), and a
// multiget of what each sync calls deleted. It counts each answer against
// the store and lists the hrefs a sync calls deleted that the server still
// serves. Hrefs given as arguments are also shown in each answer, with
// their multiget and GET (-raw prints the multiget's data).
//
// It reads a snapshot of maild's database, removed when it ends, and the
// keyring, and sends the server only PROPFIND, REPORT and GET.
//
//	go run ./tools/davprobe -collection 342
//	go run ./tools/davprobe -collection 342 -token TOKEN -raw HREF...
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"

	_ "modernc.org/sqlite"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// batch is how many hrefs one multiget asks for, as maild's.
const batch = 100

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "davprobe:", err)
		os.Exit(1)
	}
}

type options struct {
	collection int64
	token      string
	raw        bool
	hrefs      []string
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("davprobe", flag.ContinueOnError)
	var o options
	fs.Int64Var(&o.collection, "collection", 0, "the collection ID (required)")
	fs.StringVar(&o.token, "token", "", "also sync-collection from this token (an older one from a backup)")
	fs.BoolVar(&o.raw, "raw", false, "print the multiget data of each href given")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if o.collection == 0 {
		return errors.New("usage: davprobe -collection ID [-token TOKEN] [-raw] [HREF...]")
	}
	o.hrefs = fs.Args()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "davprobe-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	snap := filepath.Join(tmp, "snapshot.db")
	if err := snapshot(ctx, paths.DB, snap); err != nil {
		return err
	}
	// Opening the snapshot runs this build's migrations, which may drop
	// the sync state maild has: read the stored token first.
	stored, err := syncToken(ctx, snap, o.collection)
	if err != nil {
		return err
	}
	db, err := store.Open(ctx, snap)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	col, err := db.GetCollection(ctx, o.collection)
	if err != nil {
		return fmt.Errorf("collection %d: %w", o.collection, err)
	}
	acct, err := db.GetAccount(ctx, col.AccountID)
	if err != nil {
		return err
	}
	service, kind := api.ServiceKindCalendar, davx.Calendars
	switch col.Kind {
	case api.CollectionKindAddressbook:
		service, kind = api.ServiceKindContacts, davx.AddressBooks
	case api.CollectionKindTasklist:
		service = api.ServiceKindTasks
	}
	services, err := db.Services(ctx, acct.ID)
	if err != nil {
		return err
	}
	home := ""
	for _, s := range services {
		if s.Service == service {
			home = s.Home
		}
	}
	if home == "" {
		return fmt.Errorf("account %d has no %s home set (Google task lists are not DAV)", acct.ID, service)
	}
	// The secrets file is the snapshot's: what Open would move from it is
	// nothing, so the user's keyring is only read.
	sec, err := secrets.Open(ctx, filepath.Join(tmp, "secrets.json"), log, secrets.HostOptions())
	if err != nil {
		return err
	}
	profile := providers.ForKind(acct.Kind)
	c, err := davx.New(home, davx.Options{
		Authorization: authorization(db, sec, acct, log),
		Trusted:       func(u *url.URL) bool { return profile.TrustsHost(u.Hostname()) },
		UserAgent:     "frostmail-davprobe",
	})
	if err != nil {
		return err
	}
	local, err := db.ObjectETags(ctx, col.ID)
	if err != nil {
		return err
	}
	col.SyncToken = stored
	p := &probe{c: c, kind: kind, col: col, local: local, opts: o, out: out}
	fmt.Fprintf(out, "collection %d %q (%s), account %d (%s): %d stored\n\n", col.ID, col.Name, col.Href, acct.ID, acct.Kind, len(local))
	return p.run(ctx)
}

type probe struct {
	c     *davx.Client
	kind  davx.Kind
	col   store.Collection
	local map[string]string
	opts  options
	out   io.Writer
}

func (p *probe) run(ctx context.Context) error {
	listed, err := p.c.List(ctx, p.col.Href)
	if err != nil {
		return fmt.Errorf("propfind: %w", err)
	}
	p.report("PROPFIND depth 1", davx.Delta{Changed: listed})
	fmt.Fprintln(p.out)
	for i, from := range []struct{ name, token string }{
		{"sync-collection from no token", ""},
		{"sync-collection from the stored token", p.col.SyncToken},
		{"sync-collection from -token", p.opts.token},
	} {
		if i > 0 && from.token == "" {
			continue
		}
		delta, err := syncAll(ctx, p.c, p.col.Href, from.token)
		if err != nil {
			fmt.Fprintf(p.out, "%s: %v\n\n", from.name, err)
			continue
		}
		p.report(from.name, delta)
		if err := p.served(ctx, delta.Deleted); err != nil {
			return err
		}
		fmt.Fprintln(p.out)
	}
	if len(p.opts.hrefs) > 0 {
		return p.fetch(ctx)
	}
	return nil
}

// report counts an answer against the store and shows the hrefs given.
func (p *probe) report(name string, d davx.Delta) {
	named := map[string]int{}
	etags := map[string]string{}
	for _, ch := range d.Changed {
		named[ch.Href]++
		etags[ch.Href] = ch.ETag
	}
	unstored, unlisted := 0, 0
	for h := range named {
		if _, ok := p.local[h]; !ok {
			unstored++
		}
	}
	for h := range p.local {
		if named[h] == 0 {
			unlisted++
		}
	}
	fmt.Fprintf(p.out, "%s: %d listed (%d hrefs), %d called deleted; %d listed and not stored, %d stored and not listed\n",
		name, len(d.Changed), len(named), len(d.Deleted), unstored, unlisted)
	for h, n := range named {
		if n > 1 {
			fmt.Fprintf(p.out, "  listed %d times: %s\n", n, h)
		}
	}
	for _, h := range p.opts.hrefs {
		_, stored := p.local[h]
		fmt.Fprintf(p.out, "  %s: listed %v %s, called deleted %v, stored %v\n",
			h, named[h] > 0, etags[h], slices.Contains(d.Deleted, h), stored)
	}
}

// served asks for the hrefs a sync calls deleted and lists those the
// server still sends.
func (p *probe) served(ctx context.Context, deleted []string) error {
	n := 0
	for start := 0; start < len(deleted); start += batch {
		objs, _, err := p.c.Multiget(ctx, p.kind, p.col.Href, deleted[start:min(start+batch, len(deleted))])
		if err != nil {
			return fmt.Errorf("multiget: %w", err)
		}
		for _, o := range objs {
			_, stored := p.local[o.Href]
			fmt.Fprintf(p.out, "  called deleted, served: %s (etag %s, stored %v)\n", o.Href, o.ETag, stored)
		}
		n += len(objs)
	}
	fmt.Fprintf(p.out, "  a multiget of those called deleted: %d served, %d not\n", n, len(deleted)-n)
	return nil
}

// fetch shows how a multiget and a GET answer the hrefs given.
func (p *probe) fetch(ctx context.Context) error {
	objs, missing, err := p.c.Multiget(ctx, p.kind, p.col.Href, p.opts.hrefs)
	if err != nil {
		return fmt.Errorf("multiget: %w", err)
	}
	fmt.Fprintf(p.out, "multiget: %d served, missing %q\n", len(objs), missing)
	for _, o := range objs {
		fmt.Fprintf(p.out, "  %s: etag %s, %d bytes\n", o.Href, o.ETag, len(o.Data))
		if p.opts.raw {
			fmt.Fprintf(p.out, "%s\n", o.Data)
		}
	}
	for _, h := range p.opts.hrefs {
		o, err := p.c.Get(ctx, h)
		if err != nil {
			fmt.Fprintf(p.out, "GET %s: %v\n", h, err)
			continue
		}
		fmt.Fprintf(p.out, "GET %s: etag %s, %d bytes\n", h, o.ETag, len(o.Data))
	}
	return nil
}

// syncAll follows a sync-collection's pages (507) to the end.
func syncAll(ctx context.Context, c *davx.Client, coll, token string) (davx.Delta, error) {
	var out davx.Delta
	for {
		d, err := c.Sync(ctx, coll, token)
		if err != nil {
			return out, err
		}
		out.Changed = append(out.Changed, d.Changed...)
		out.Deleted = append(out.Deleted, d.Deleted...)
		out.Token = d.Token
		if !d.More || d.Token == token {
			return out, nil
		}
		token = d.Token
	}
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

// syncToken reads a collection's sync token from a database as it is.
func syncToken(ctx context.Context, path string, collection int64) (string, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return "", err
	}
	defer func() { _ = db.Close() }()
	var token string
	if err := db.QueryRowContext(ctx, `SELECT sync_token FROM collections WHERE id = ?`, collection).Scan(&token); err != nil {
		return "", fmt.Errorf("collection %d: %w", collection, err)
	}
	return token, nil
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
