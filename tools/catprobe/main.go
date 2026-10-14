// Command catprobe measures Gmail's categories for the categories spike
// (docs/plans/0008-m5-mail-app-parity.md, Phase 7). Over a read-only
// session it asks Gmail which of an account's stored messages each
// category holds (X-GM-RAW on All Mail), times those searches over the
// sync window and over the newest mail only, and looks for categories
// among X-GM-LABELS; then it compares a classifier of stored headers
// (classify.go) with Gmail's answer. A non-Gmail account gets the
// classifier's counts only.
//
// It reads a snapshot of maild's database, removed when it ends, and
// prints counts and timings, never a message's subject or address.
//
//	go run ./tools/catprobe -account 4
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "catprobe:", err)
		os.Exit(1)
	}
}

type options struct {
	account int64
	runs    int
	newest  int
	sample  int
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("catprobe", flag.ContinueOnError)
	var o options
	fs.Int64Var(&o.account, "account", 0, "the account to measure (required)")
	fs.IntVar(&o.runs, "runs", 3, "timed runs of each search over the window")
	fs.IntVar(&o.newest, "newest", 200, "the newest messages a pass's search covers")
	fs.IntVar(&o.sample, "sample", 20, "messages per category whose X-GM-LABELS are read")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if o.account == 0 {
		return errors.New("-account is required")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "catprobe-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	snap := filepath.Join(tmp, "snapshot.db")
	if err := snapshot(ctx, paths.DB, snap); err != nil {
		return err
	}
	db, err := store.Open(ctx, snap)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	q, err := sql.Open("sqlite", "file:"+snap+"?mode=ro")
	if err != nil {
		return err
	}
	defer func() { _ = q.Close() }()
	acct, err := db.GetAccount(ctx, o.account)
	if err != nil {
		return err
	}
	msgs, err := load(ctx, q, acct)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Account %d (%s), sync window %d days: %d stored messages\n\n", acct.ID, acct.Kind, acct.SyncDays, len(msgs))
	if acct.Kind != api.AccountKindGmail {
		printLocal(out, msgs)
		return nil
	}
	// The secrets file is the snapshot's: what Open would move from it is
	// nothing, so the user's keyring is only read.
	sec, err := secrets.Open(ctx, filepath.Join(tmp, "secrets.json"), log)
	if err != nil {
		return err
	}
	s, err := dial(ctx, db, sec, acct, log)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	return probe(ctx, out, s, q, acct, msgs, o)
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

func dial(ctx context.Context, db *store.DB, sec secrets.Store, acct store.Account, log *slog.Logger) (*imapx.Session, error) {
	var secret string
	var err error
	isOAuth := acct.Auth == api.AuthKindOAuth2
	if isOAuth {
		tokens := &oauth.Manager{DB: db, Secrets: sec, Log: log}
		secret, err = tokens.AccessToken(ctx, acct.ID)
	} else {
		secret, err = sec.Get(ctx, secrets.AccountPassword(acct.ID))
	}
	if err != nil {
		return nil, fmt.Errorf("credential: %w", err)
	}
	return imapx.Open(ctx, imapx.DialOptions{
		Host: acct.IMAP.Host, Port: acct.IMAP.Port, TLS: acct.IMAP.TLS, Username: acct.IMAP.Username,
		Password: secret, OAuth: isOAuth, ReadOnly: true, Log: log,
	})
}

// message is a stored message as the probe sees it.
type message struct {
	uid   uint32 // in All Mail; 0 when not there (Spam, Trash)
	inbox bool
	mine  bool // from one of the account's addresses
	mail  Mail
}

// load reads the account's messages, oldest first, and who the user
// knows: contacts, everyone the user wrote to, and the conversations the
// user wrote in.
func load(ctx context.Context, q *sql.DB, acct store.Account) ([]message, error) {
	mine := map[string]bool{strings.ToLower(acct.Email): true}
	if err := each(ctx, q, `SELECT lower(email) FROM identities WHERE account_id = ?`, []any{acct.ID}, func(r *sql.Rows) error {
		var e string
		err := r.Scan(&e)
		mine[e] = true
		return err
	}); err != nil {
		return nil, err
	}
	known := map[string]bool{}
	if err := each(ctx, q, `SELECT email FROM contact_emails`, nil, func(r *sql.Rows) error {
		var e string
		err := r.Scan(&e)
		known[e] = true
		return err
	}); err != nil {
		return nil, err
	}
	wroteIn := map[int64]bool{}
	sentWhere := `FROM messages m WHERE m.account_id = ? AND m.deleted = 0 AND EXISTS (SELECT 1 FROM message_mailbox mm
		JOIN mailboxes b ON b.id = mm.mailbox_id WHERE mm.message_id = m.id AND b.role = 'sent')`
	if err := each(ctx, q, `SELECT lower(json_extract(j.value, '$.addr')), COALESCE(m.thread_id, -m.id) `+
		strings.Replace(sentWhere, "FROM messages m", "FROM messages m, json_each(m.to_json) j", 1)+
		` UNION ALL SELECT lower(json_extract(j.value, '$.addr')), COALESCE(m.thread_id, -m.id) `+
		strings.Replace(sentWhere, "FROM messages m", "FROM messages m, json_each(m.cc_json) j", 1),
		[]any{acct.ID, acct.ID}, func(r *sql.Rows) error {
			var addr sql.NullString
			var thread int64
			if err := r.Scan(&addr, &thread); err != nil {
				return err
			}
			if addr.String != "" {
				known[addr.String] = true
			}
			wroteIn[thread] = true
			return nil
		}); err != nil {
		return nil, err
	}
	var out []message
	err := each(ctx, q, `SELECT COALESCE((SELECT mm.uid FROM message_mailbox mm JOIN mailboxes b ON b.id = mm.mailbox_id
			WHERE mm.message_id = m.id AND b.role = 'all' AND mm.uid IS NOT NULL), 0),
		EXISTS (SELECT 1 FROM message_mailbox mm JOIN mailboxes b ON b.id = mm.mailbox_id
			WHERE mm.message_id = m.id AND b.role = 'inbox'),
		lower(m.from_addr), m.subject, m.list_id <> '', m.list_unsubscribe <> '', COALESCE(m.thread_id, -m.id)
		FROM messages m WHERE m.account_id = ? AND m.deleted = 0 ORDER BY m.internal_date, m.id`, []any{acct.ID},
		func(r *sql.Rows) error {
			var m message
			var thread int64
			if err := r.Scan(&m.uid, &m.inbox, &m.mail.From, &m.mail.Subject, &m.mail.ListID, &m.mail.Unsubscribe, &thread); err != nil {
				return err
			}
			m.mine = mine[m.mail.From]
			m.mail.Known = known[m.mail.From] || wroteIn[thread]
			out = append(out, m)
			return nil
		})
	return out, err
}

func each(ctx context.Context, q *sql.DB, query string, args []any, row func(*sql.Rows) error) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := row(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// printLocal prints the classifier's counts over received mail.
func printLocal(out io.Writer, msgs []message) {
	var counts [len(categoryNames)]int
	n := 0
	for _, m := range msgs {
		if m.mine {
			continue
		}
		counts[Classify(m.mail)]++
		n++
	}
	fmt.Fprintf(out, "Classifier over %d received messages\n", n)
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	for c, k := range counts {
		fmt.Fprintf(w, "%s\t%d\t%s\t\n", Category(c), k, pct(k, n))
	}
	_ = w.Flush()
}

func probe(ctx context.Context, out io.Writer, s *imapx.Session, q *sql.DB, acct store.Account,
	msgs []message, o options,
) error {
	var allMail string
	if err := q.QueryRowContext(ctx, `SELECT path FROM mailboxes WHERE account_id = ? AND role = 'all'`, acct.ID).
		Scan(&allMail); err != nil {
		return fmt.Errorf("all mail: %w", err)
	}
	var storedValidity int64
	if err := q.QueryRowContext(ctx, `SELECT COALESCE(uidvalidity, 0) FROM mailboxes WHERE account_id = ? AND role = 'all'`,
		acct.ID).Scan(&storedValidity); err != nil {
		return err
	}
	sel, err := s.Select(ctx, allMail)
	if err != nil {
		return err
	}
	if int64(sel.UIDValidity) != storedValidity {
		return fmt.Errorf("all mail's UIDVALIDITY is %d on the server and %d stored", sel.UIDValidity, storedValidity)
	}
	var since time.Time
	if acct.SyncDays > 0 {
		since = time.Now().AddDate(0, 0, -acct.SyncDays)
	}
	byUID := map[uint32]*message{}
	var uids []uint32
	for i := range msgs {
		if msgs[i].uid != 0 {
			byUID[msgs[i].uid] = &msgs[i]
			uids = append(uids, msgs[i].uid)
		}
	}
	slices.Sort(uids)
	fmt.Fprintf(out, "All Mail: %d messages on the server, %d stored with a UID\n\n", sel.Messages, len(uids))

	// The searches over the window, timed; the plain search is the baseline.
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "Search over the window\tresults\tstored\tmin\tmedian\tmax\t\n")
	base, times, err := timed(o.runs, func() ([]uint32, error) { return s.UIDsSince(ctx, since) })
	if err != nil {
		return err
	}
	row(w, "SINCE (baseline)", base, byUID, times)
	cats := map[uint32]map[string]bool{}
	var tabTotal time.Duration
	for _, c := range slices.Concat(gmailTabs, gmailExtra) {
		got, times, err := timed(o.runs, func() ([]uint32, error) { return s.SearchGmailRaw(ctx, "category:"+c, since, 0) })
		if err != nil {
			fmt.Fprintf(w, "category:%s\t%v\t\t\t\t\t\n", c, err)
			continue
		}
		row(w, "category:"+c, got, byUID, times)
		if slices.Contains(gmailTabs, c) {
			tabTotal += median(times)
		}
		for _, u := range got {
			if cats[u] == nil {
				cats[u] = map[string]bool{}
			}
			cats[u][c] = true
		}
	}
	_ = w.Flush()
	fmt.Fprintf(out, "Every tab over the window: %v (sum of medians)\n\n", tabTotal.Round(time.Millisecond))

	// What a pass would search: only the newest mail.
	if n := min(o.newest, len(uids)); n > 0 {
		floor := uids[len(uids)-n]
		w = tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
		fmt.Fprintf(w, "Search over the newest %d (UID %d:*)\tresults\ttime\t\n", n, floor)
		var total time.Duration
		for _, c := range gmailTabs {
			start := time.Now()
			got, err := s.SearchGmailRaw(ctx, "category:"+c, time.Time{}, floor)
			d := time.Since(start)
			if err != nil {
				fmt.Fprintf(w, "category:%s\t%v\t\t\n", c, err)
				continue
			}
			total += d
			fmt.Fprintf(w, "category:%s\t%d\t%v\t\n", c, len(got), d.Round(time.Millisecond))
		}
		_ = w.Flush()
		fmt.Fprintf(out, "A pass's searches: %v\n\n", total.Round(time.Millisecond))
	}
	day := time.Now().AddDate(0, 0, -1)
	w = tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "Search since yesterday (SINCE %s)\tresults\tmin\tmedian\tmax\t\n", day.Format("2-Jan-2006"))
	var dayTotal time.Duration
	for _, c := range gmailTabs {
		got, times, err := timed(o.runs, func() ([]uint32, error) { return s.SearchGmailRaw(ctx, "category:"+c, day, 0) })
		if err != nil {
			fmt.Fprintf(w, "category:%s\t%v\t\t\t\t\n", c, err)
			continue
		}
		dayTotal += median(times)
		ss := slices.Clone(times)
		slices.Sort(ss)
		r := func(d time.Duration) time.Duration { return d.Round(time.Millisecond) }
		fmt.Fprintf(w, "category:%s\t%d\t%v\t%v\t%v\t\n", c, len(got), r(ss[0]), r(median(ss)), r(ss[len(ss)-1]))
	}
	_ = w.Flush()
	fmt.Fprintf(out, "A pass's searches since yesterday: %v (sum of medians)\n\n", dayTotal.Round(time.Millisecond))

	if err := printLabels(ctx, out, s, cats, o.sample); err != nil {
		return err
	}
	printCoverage(out, msgs, cats)
	printAgreement(out, msgs, cats)
	printSenderMemory(out, msgs, cats)
	return nil
}

// timed runs a search runs times and returns its last result and the
// durations.
func timed(runs int, search func() ([]uint32, error)) ([]uint32, []time.Duration, error) {
	var got []uint32
	var times []time.Duration
	for range max(runs, 1) {
		start := time.Now()
		var err error
		if got, err = search(); err != nil {
			return nil, nil, err
		}
		times = append(times, time.Since(start))
	}
	return got, times, nil
}

func median(ds []time.Duration) time.Duration {
	s := slices.Clone(ds)
	slices.Sort(s)
	return s[len(s)/2]
}

func row(w io.Writer, name string, got []uint32, stored map[uint32]*message, times []time.Duration) {
	n := 0
	for _, u := range got {
		if stored[u] != nil {
			n++
		}
	}
	s := slices.Clone(times)
	slices.Sort(s)
	r := func(d time.Duration) time.Duration { return d.Round(time.Millisecond) }
	fmt.Fprintf(w, "%s\t%d\t%d\t%v\t%v\t%v\t\n", name, len(got), n, r(s[0]), r(median(s)), r(s[len(s)-1]))
}

// categoryLike is a label name that would carry a category.
var categoryLike = regexp.MustCompile(`(?i)categor|smartlabel|^\\?(social|promotions|updates|forums|personal)$`)

// printLabels reads X-GM-LABELS of a sample of each category's messages
// and reports whether any label carries the category. User labels are
// counted, not named.
func printLabels(ctx context.Context, out io.Writer, s *imapx.Session, cats map[uint32]map[string]bool, sample int) error {
	var pick []uint32
	for _, c := range gmailTabs {
		var in []uint32
		for u, cs := range cats {
			if cs[c] {
				in = append(in, u)
			}
		}
		slices.Sort(in)
		pick = append(pick, in[max(0, len(in)-sample):]...)
	}
	if len(pick) == 0 {
		return nil
	}
	hs, err := s.FetchGmailHeaders(ctx, pick)
	if err != nil {
		return err
	}
	system := map[string]int{}
	users := map[string]bool{}
	var like []string
	for _, h := range hs {
		for _, l := range h.Labels {
			switch {
			case categoryLike.MatchString(l):
				like = append(like, l)
			case strings.HasPrefix(l, `\`):
				system[l]++
			default:
				users[l] = true
			}
		}
	}
	names := make([]string, 0, len(system))
	for l, n := range system {
		names = append(names, fmt.Sprintf("%s (%d)", l, n))
	}
	slices.Sort(names)
	slices.Sort(like)
	fmt.Fprintf(out, "X-GM-LABELS of %d messages across the tabs: system labels %s; %d user labels; category-like: %v\n\n",
		len(hs), strings.Join(names, ", "), len(users), slices.Compact(like))
	return nil
}

// printCoverage reports how many stored messages Gmail gave a tab, how
// many tabs, and where the tabs fall, in the inbox and out of it.
func printCoverage(out io.Writer, msgs []message, cats map[uint32]map[string]bool) {
	var stored, none, one, several int
	tabs := map[string][2]int{} // [inbox, elsewhere]
	pairs := map[string]int{}
	for _, m := range msgs {
		if m.uid == 0 {
			continue
		}
		stored++
		var in []string
		for _, c := range gmailTabs {
			if cats[m.uid][c] {
				in = append(in, c)
				t := tabs[c]
				if m.inbox {
					t[0]++
				} else {
					t[1]++
				}
				tabs[c] = t
			}
		}
		switch len(in) {
		case 0:
			none++
		case 1:
			one++
		default:
			several++
			pairs[strings.Join(in, "+")]++
		}
	}
	fmt.Fprintf(out, "Stored All Mail messages: %d; with no tab %d (%s), one %d, several %d %v\n",
		stored, none, pct(none, stored), one, several, pairs)
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "Tab\tin the inbox\telsewhere\t\n")
	for _, c := range gmailTabs {
		fmt.Fprintf(w, "%s\t%d\t%d\t\n", c, tabs[c][0], tabs[c][1])
	}
	_ = w.Flush()
	fmt.Fprintln(out)
}

// printAgreement compares the classifier with Gmail's categories, mapped
// to Mail.app's, over received mail Gmail gave a tab.
func printAgreement(out io.Writer, msgs []message, cats map[uint32]map[string]bool) {
	const n = len(categoryNames)
	var matrix [n][n]int
	var untabbed [n]int
	total, agree := 0, 0
	for _, m := range msgs {
		if m.uid == 0 || m.mine {
			continue
		}
		want, ok := fromGmail(cats[m.uid])
		if !ok {
			untabbed[Classify(m.mail)]++
			continue
		}
		got := Classify(m.mail)
		matrix[want][got]++
		total++
		if got == want {
			agree++
		}
	}
	fmt.Fprintf(out, "Classifier against Gmail over %d received messages: %d agree (%s)\n", total, agree, pct(agree, total))
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprint(w, "Gmail \\ classifier\t")
	for c := range n {
		fmt.Fprintf(w, "%s\t", Category(c))
	}
	fmt.Fprint(w, "recall\t\n")
	for g := range n {
		fmt.Fprintf(w, "%s\t", Category(g))
		sum := 0
		for c := range n {
			fmt.Fprintf(w, "%d\t", matrix[g][c])
			sum += matrix[g][c]
		}
		fmt.Fprintf(w, "%s\t\n", pct(matrix[g][g], sum))
	}
	fmt.Fprint(w, "precision\t")
	for c := range n {
		col := 0
		for g := range n {
			col += matrix[g][c]
		}
		fmt.Fprintf(w, "%s\t", pct(matrix[c][c], col))
	}
	fmt.Fprint(w, "\t\n")
	_ = w.Flush()
	fmt.Fprintf(out, "Received mail with no tab, as the classifier sorts it: %v %v %v %v (%s)\n\n",
		untabbed[Primary], untabbed[Transactions], untabbed[Updates], untabbed[Promotions], strings.Join(categoryNames[:], ", "))
}

// printSenderMemory trains a memory of each sender's Gmail category on the
// oldest 80% of received mail with a tab and tests it on the newest 20%:
// the sender's address if it was seen, else its domain, else the
// classifier.
func printSenderMemory(out io.Writer, msgs []message, cats map[uint32]map[string]bool) {
	const n = len(categoryNames)
	type labeled struct {
		mail Mail
		want Category
	}
	var all []labeled
	for _, m := range msgs {
		if m.uid == 0 || m.mine {
			continue
		}
		if want, ok := fromGmail(cats[m.uid]); ok {
			all = append(all, labeled{m.mail, want})
		}
	}
	cut := len(all) * 4 / 5
	byAddr := map[string]*[n]int{}
	byDomain := map[string]*[n]int{}
	add := func(t map[string]*[n]int, k string, c Category) {
		if t[k] == nil {
			t[k] = &[n]int{}
		}
		t[k][c]++
	}
	for _, l := range all[:cut] {
		add(byAddr, l.mail.From, l.want)
		add(byDomain, domain(l.mail.From), l.want)
	}
	best := func(c *[n]int) Category {
		b := 0
		for i := range n {
			if c[i] > c[b] {
				b = i
			}
		}
		return Category(b)
	}
	type score struct{ seen, right int }
	var byA, byD, byC score
	for _, l := range all[cut:] {
		var got Category
		var s *score
		switch {
		case byAddr[l.mail.From] != nil:
			got, s = best(byAddr[l.mail.From]), &byA
		case byDomain[domain(l.mail.From)] != nil:
			got, s = best(byDomain[domain(l.mail.From)]), &byD
		default:
			got, s = Classify(l.mail), &byC
		}
		s.seen++
		if got == l.want {
			s.right++
		}
	}
	test := len(all) - cut
	fmt.Fprintf(out, "Sender memory, trained on the oldest %d, tested on the newest %d: %s agree\n", cut, test,
		pct(byA.right+byD.right+byC.right, test))
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "Decided by\tmessages\tshare\tagree\t\n")
	for _, r := range []struct {
		name string
		s    score
	}{{"the address", byA}, {"the domain", byD}, {"the classifier", byC}} {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t\n", r.name, r.s.seen, pct(r.s.seen, test), pct(r.s.right, r.s.seen))
	}
	_ = w.Flush()
}

func domain(addr string) string {
	_, d, _ := strings.Cut(addr, "@")
	return d
}

func pct(k, n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(k)/float64(n))
}
