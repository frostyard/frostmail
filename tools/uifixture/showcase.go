package main

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"maps"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// The showcase (docs/images, make screenshots) is hand-written mail for the
// README's screenshots: everything in it is made up. Its directory holds
// one directory per account, named by the account's address, with a
// "name" file for the display name and a directory per mailbox (nested
// for a hierarchy, Projects/Aurora) of NN[-FLAGS].eml files: NN is the
// UID, FLAGS the Maildir letters F (flagged, a digit after it for the
// color), R (answered) and S (seen). Every Date moves by whole days so
// that the newest message arrived today and the screenshots look current.
// The accounts' servers are mail.<domain>, which do not exist: maild runs
// the showcase with FROSTMAIL_SYNC=off.

// showcaseMessage is one .eml file of the showcase.
type showcaseMessage struct {
	mailbox string
	uid     uint32
	flags   string
	raw     []byte
	date    time.Time
}

var dateHeader = regexp.MustCompile(`(?mi)^Date:[^\r\n]*`)

// buildShowcase builds the showcase accounts in the data directory out's
// db, as of now.
func buildShowcase(ctx context.Context, db *store.DB, blobs *blob.Store, out, dir string, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read showcase: %w", err)
	}
	type account struct {
		email, name string
		msgs        []showcaseMessage
	}
	var accounts []account
	var newest time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		a := account{email: e.Name()}
		root := filepath.Join(dir, e.Name())
		name, err := os.ReadFile(filepath.Join(root, "name"))
		if err != nil {
			return fmt.Errorf("showcase account %s: %w", a.email, err)
		}
		a.name = strings.TrimSpace(string(name))
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".eml" {
				return err
			}
			m, err := readShowcaseMessage(root, path)
			if err != nil {
				return err
			}
			if m.date.After(newest) {
				newest = m.date
			}
			a.msgs = append(a.msgs, m)
			return nil
		})
		if err != nil {
			return err
		}
		accounts = append(accounts, a)
	}
	if len(accounts) == 0 {
		return fmt.Errorf("showcase %s has no accounts", dir)
	}
	days := daysBetween(newest, now)
	for _, a := range accounts {
		boxes := map[string][]showcaseMessage{}
		for _, m := range a.msgs {
			m.raw = dateHeader.ReplaceAll(m.raw, []byte("Date: "+m.date.AddDate(0, 0, days).Format(time.RFC1123Z)))
			boxes[m.mailbox] = append(boxes[m.mailbox], m)
		}
		if inv, ok := showcaseInvitation(a.email, now, boxes["INBOX"]); ok {
			boxes["INBOX"] = append(boxes["INBOX"], inv)
		}
		host := "mail." + a.email[strings.LastIndexByte(a.email, '@')+1:]
		id, mbIDs, err := insertAccount(ctx, db, a.email, a.name, host, 993, 587, showcaseMailboxes(boxes))
		if err != nil {
			return err
		}
		// A password, so the account reads as signed in; maild runs the
		// showcase with sync off and never uses it.
		if err := secrets.NewFile(filepath.Join(out, "secrets.json")).Set(ctx, secrets.AccountPassword(id), "showcase"); err != nil {
			return err
		}
		if err := addShowcaseContacts(ctx, db, id, a.email, now); err != nil {
			return err
		}
		if err := addShowcaseCalendars(ctx, db, id, a.email, now); err != nil {
			return err
		}
		if err := addShowcaseTasks(ctx, db, id, a.email, now); err != nil {
			return err
		}
		b := &builder{db: db, blobs: blobs, accountID: id}
		for _, path := range slices.Sorted(maps.Keys(boxes)) {
			msgs := boxes[path]
			slices.SortFunc(msgs, func(x, y showcaseMessage) int { return int(x.uid) - int(y.uid) })
			for _, m := range msgs {
				if err := b.add(ctx, mbIDs[path], m.raw, m.uid, m.flags); err != nil {
					return err
				}
			}
			if err := b.flush(ctx, mbIDs[path]); err != nil {
				return err
			}
		}
	}
	return nil
}

// readShowcaseMessage reads one NN[-FLAGS].eml file under an account's root.
func readShowcaseMessage(root, path string) (showcaseMessage, error) {
	var m showcaseMessage
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return m, err
	}
	m.mailbox = filepath.ToSlash(rel)
	base := strings.TrimSuffix(filepath.Base(path), ".eml")
	num, flags, _ := strings.Cut(base, "-")
	uid, err := strconv.ParseUint(num, 10, 32)
	if err != nil || uid == 0 {
		return m, fmt.Errorf("showcase %s: the name must start with a UID", path)
	}
	m.uid, m.flags = uint32(uid), flags
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	// Files are written with LF; mail is CRLF.
	m.raw = bytes.ReplaceAll(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))
	msg, err := mail.ReadMessage(bytes.NewReader(m.raw))
	if err != nil {
		return m, fmt.Errorf("showcase %s: %w", path, err)
	}
	if m.date, err = msg.Header.Date(); err != nil {
		return m, fmt.Errorf("showcase %s: %w", path, err)
	}
	return m, nil
}

// daysBetween is the whole days from newest's date to now's, in local time.
func daysBetween(newest, now time.Time) int {
	day := func(t time.Time) time.Time {
		y, m, d := t.In(time.Local).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	return int(day(now).Sub(day(newest)).Hours() / 24)
}

// showcaseMailboxes is the fixture's standard mailboxes plus the showcase's
// own, with their parents as folders that hold no mail.
func showcaseMailboxes(boxes map[string][]showcaseMessage) []store.ServerMailbox {
	list := fixtureMailboxes()
	list = slices.DeleteFunc(list, func(mb store.ServerMailbox) bool { return mb.Path == "Hostile" })
	have := map[string]bool{}
	for _, mb := range list {
		have[mb.Path] = true
	}
	for _, path := range slices.Sorted(maps.Keys(boxes)) {
		parts := strings.Split(path, "/")
		for i := range parts {
			p := strings.Join(parts[:i+1], "/")
			if have[p] {
				continue
			}
			have[p] = true
			list = append(list, store.ServerMailbox{Path: p, Delimiter: "/", Role: api.MailboxRoleNone,
				Selectable: i == len(parts)-1, Subscribed: true})
		}
	}
	return list
}
