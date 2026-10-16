// Command msprobe finds out how a Microsoft account (Outlook.com, Hotmail)
// behaves for Frostmail before maild speaks to it (ADR-0039; plan 0010,
// Phase 1): which scopes sign in to IMAP, SMTP and Graph from one consent,
// what Outlook's IMAP offers, and what Graph serves for calendars and
// contacts.
//
// signin runs the authorization-code flow with PKCE on a loopback port
// and keeps the refresh token in a 0600 state file outside the repository.
// The other commands redeem it for the audience they need, storing each
// rotated token before using the new one. imap and graph only read;
// smtp signs in without sending.
//
//	go run ./tools/msprobe -state /tmp/ms.json signin
//	go run ./tools/msprobe -state /tmp/ms.json -trace /tmp/imap.trace imap
//	go run ./tools/msprobe -state /tmp/ms.json smtp
//	go run ./tools/msprobe -state /tmp/ms.json -out /tmp/graph graph
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/smtpx"
)

// ClientID is Frostmail's public client in Microsoft Entra (plan 0010).
const ClientID = "8007f9b9-a6b6-4029-8295-3e6af287317b"

const (
	authURL  = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
	tokenURL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	graphURL = "https://graph.microsoft.com/v1.0"
)

// The scopes by audience. A token is for one audience; the refresh token
// serves every audience the user consented to.
var (
	outlookScopes = []string{"https://outlook.office.com/IMAP.AccessAsUser.All", "https://outlook.office.com/SMTP.Send"}
	graphScopes   = []string{"https://graph.microsoft.com/User.Read", "https://graph.microsoft.com/Calendars.ReadWrite",
		"https://graph.microsoft.com/Contacts.ReadWrite"}
	identityScopes = []string{"offline_access", "openid", "email", "profile"}
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "msprobe:", err)
		os.Exit(1)
	}
}

type probe struct {
	statePath string
	trace     string
	out       string
	w         io.Writer
	http      *http.Client
}

// state is what signin keeps: the account and its latest refresh token.
type state struct {
	Email    string    `json:"email"`
	Refresh  string    `json:"refresh"`
	Scope    string    `json:"scope"`
	Redeemed time.Time `json:"redeemed"`
}

func run(ctx context.Context, args []string, w io.Writer) error {
	fs := flag.NewFlagSet("msprobe", flag.ContinueOnError)
	statePath := fs.String("state", "", "the 0600 file that keeps the refresh token (required, outside the repository)")
	trace := fs.String("trace", "", "write imap's protocol trace here (credentials redacted)")
	out := fs.String("out", "", "write graph's raw JSON replies into this directory")
	authScopes := fs.String("scopes", "", "the authorization's scopes, space separated (default: identity, Outlook and Graph)")
	redirectPath := fs.String("redirect-path", "", "the path after http://localhost:PORT in the redirect URI")
	link := fs.String("link", "", "follow: a Graph delta or next link to read")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *statePath == "" || fs.NArg() != 1 {
		return errors.New("usage: msprobe -state FILE signin|imap|smtp|graph|writes|send|follow|flags|unflag")
	}
	p := &probe{statePath: *statePath, trace: *trace, out: *out, w: w, http: &http.Client{Timeout: time.Minute}}
	switch fs.Arg(0) {
	case "signin":
		scopes := slices.Concat(identityScopes, outlookScopes, graphScopes)
		if *authScopes != "" {
			scopes = strings.Fields(*authScopes)
		}
		return p.signin(ctx, scopes, *redirectPath)
	case "imap":
		return p.imap(ctx)
	case "smtp":
		return p.smtp(ctx)
	case "graph":
		return p.graph(ctx)
	case "writes":
		return p.writes(ctx)
	case "send":
		return p.send(ctx)
	case "follow":
		return p.follow(ctx, *link)
	case "flags":
		return p.flags(ctx, false)
	case "unflag":
		return p.flags(ctx, true)
	}
	return fmt.Errorf("unknown command %q", fs.Arg(0))
}

func (p *probe) printf(format string, args ...any) { fmt.Fprintf(p.w, format, args...) }

// signin listens on a loopback port, prints the authorization URL, and
// redeems the code for an Outlook token, keeping the refresh token.
func (p *probe) signin(ctx context.Context, scopes []string, redirectPath string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	listeners := []net.Listener{ln}
	// Browsers may try localhost's IPv6 address first.
	if ln6, err := net.Listen("tcp", "[::1]:"+strconv.Itoa(port)); err == nil {
		listeners = append(listeners, ln6)
	}
	redirect := "http://localhost:" + strconv.Itoa(port) + redirectPath
	verifier, err := oauth.NewVerifier(rand.Reader)
	if err != nil {
		return err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	st := hex.EncodeToString(nonce)
	q := url.Values{
		"client_id":             {ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirect},
		"response_mode":         {"query"},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {st},
		"code_challenge":        {oauth.Challenge(verifier)},
		"code_challenge_method": {"S256"},
		"prompt":                {"select_account"},
	}
	p.printf("Open this URL in your browser and sign in to the Microsoft account:\n\n%s?%s\n\nWaiting up to 5 minutes for the redirect to %s\n",
		authURL, q.Encode(), redirect)

	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query()
		if v.Get("state") != st {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var res result
		if e := v.Get("error"); e != "" {
			res.err = fmt.Errorf("authorization refused: %s: %s", e, v.Get("error_description"))
			_, _ = io.WriteString(w, "Frostmail's probe was not authorized: "+e+". You can close this tab.")
		} else {
			res.code = v.Get("code")
			_, _ = io.WriteString(w, "Frostmail's probe is signed in. You can close this tab.")
		}
		select {
		case done <- res:
		default:
		}
	})}
	for _, l := range listeners {
		go func() { _ = srv.Serve(l) }()
	}
	defer func() { _ = srv.Close() }()
	var res result
	select {
	case res = <-done:
	case <-time.After(5 * time.Minute):
		return errors.New("no redirect within 5 minutes")
	case <-ctx.Done():
		return ctx.Err()
	}
	if res.err != nil {
		return res.err
	}
	// A token is for one audience: redeem the code for Outlook's.
	tok, body, err := p.token(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {res.code},
		"redirect_uri":  {redirect},
		"code_verifier": {verifier},
		"scope":         {strings.Join(slices.Concat(outlookScopes, []string{"offline_access"}), " ")},
	})
	if err != nil {
		return err
	}
	s := state{Email: idTokenEmail(body), Refresh: tok.Refresh, Scope: tok.Scope, Redeemed: time.Now().UTC()}
	if s.Refresh == "" {
		return errors.New("the token response has no refresh token (offline_access missing?)")
	}
	if err := p.save(s); err != nil {
		return err
	}
	p.printf("signed in as %q\n  granted: %s\n  access token lifetime: %s\n  refresh token: %d bytes\n",
		s.Email, tok.Scope, time.Until(tok.Expiry).Round(time.Second), len(tok.Refresh))
	return nil
}

// token posts a token request, returning the token and the raw reply.
func (p *probe) token(ctx context.Context, form url.Values) (oauth.Token, []byte, error) {
	form.Set("client_id", ClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return oauth.Token{}, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.http.Do(req)
	if err != nil {
		return oauth.Token{}, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return oauth.Token{}, nil, err
	}
	tok, err := oauth.ParseToken(body, resp.StatusCode, time.Now())
	return tok, body, err
}

// access redeems the stored refresh token for an audience's scopes and
// stores the rotated refresh token before returning the access token.
func (p *probe) access(ctx context.Context, scopes []string) (string, state, error) {
	s, err := p.load()
	if err != nil {
		return "", s, err
	}
	tok, _, err := p.token(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {s.Refresh},
		"scope":         {strings.Join(slices.Concat(scopes, []string{"offline_access"}), " ")},
	})
	if err != nil {
		return "", s, fmt.Errorf("redeem for %s: %w", scopes[0], err)
	}
	rotated := tok.Refresh != "" && tok.Refresh != s.Refresh
	if tok.Refresh != "" {
		s.Refresh, s.Redeemed = tok.Refresh, time.Now().UTC()
		if err := p.save(s); err != nil {
			return "", s, err
		}
	}
	p.printf("token for %s: granted %q, lifetime %s, refresh token rotated: %v (%d bytes)\n",
		audience(scopes[0]), tok.Scope, time.Until(tok.Expiry).Round(time.Second), rotated, len(tok.Refresh))
	return tok.Access, s, nil
}

func audience(scope string) string {
	if u, err := url.Parse(scope); err == nil && u.Host != "" {
		return u.Host
	}
	return scope
}

func (p *probe) load() (state, error) {
	var s state
	b, err := os.ReadFile(p.statePath)
	if err != nil {
		return s, fmt.Errorf("no state; run signin first: %w", err)
	}
	return s, json.Unmarshal(b, &s)
}

func (p *probe) save(s state) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := p.statePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.statePath)
}

// idTokenEmail reads the email (or preferred_username) claim of the
// token reply's ID token, unverified: it only names the account.
func idTokenEmail(body []byte) string {
	var reply struct {
		IDToken string `json:"id_token"`
	}
	if json.Unmarshal(body, &reply) != nil {
		return ""
	}
	parts := strings.Split(reply.IDToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := decodeSegment(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	if claims.Email != "" {
		return claims.Email
	}
	return claims.PreferredUsername
}

// imapHosts are Outlook.com's IMAP hosts as Microsoft and clients name them.
var imapHosts = []string{"outlook.office365.com", "imap-mail.outlook.com"}

// imap signs in to each host and reports what the server offers, read-only.
func (p *probe) imap(ctx context.Context) error {
	tok, s, err := p.access(ctx, outlookScopes[:1])
	if err != nil {
		return err
	}
	for _, host := range imapHosts {
		opts := imapx.DialOptions{Host: host, Port: 993, TLS: api.TLSModeTLS, Username: s.Email, Password: tok, OAuth: true, ReadOnly: true}
		if p.trace != "" {
			f, err := os.OpenFile(p.trace+"."+host, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			opts.Trace = f
		}
		p.printf("\n== IMAP %s\n", host)
		if err := p.imapHost(ctx, opts); err != nil {
			p.printf("  error: %v\n", err)
		}
	}
	return nil
}

func (p *probe) imapHost(ctx context.Context, opts imapx.DialOptions) error {
	start := time.Now()
	c, err := imapx.Dial(ctx, opts)
	if err != nil {
		return err
	}
	defer func() { _ = c.Logout().Wait() }()
	p.printf("  signed in in %s\n", time.Since(start).Round(time.Millisecond))
	caps, err := c.Capability().Wait()
	if err != nil {
		return err
	}
	var names []string
	for cp := range caps {
		names = append(names, string(cp))
	}
	slices.Sort(names)
	p.printf("  capabilities: %s\n", strings.Join(names, " "))
	if caps.Has(imap.CapCondStore) {
		if _, err := c.Enable(imap.CapCondStore).Wait(); err != nil {
			p.printf("  ENABLE CONDSTORE: %v\n", err)
		}
	}
	listOpts := &imap.ListOptions{}
	if caps.Has(imap.CapSpecialUse) {
		listOpts.ReturnSpecialUse = true
	}
	boxes, err := c.List("", "*", listOpts).Collect()
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	statusOpts := &imap.StatusOptions{NumMessages: true, UIDNext: true, UIDValidity: true, NumUnseen: true,
		HighestModSeq: caps.Has(imap.CapCondStore)}
	for _, b := range boxes {
		var attrs []string
		for _, a := range b.Attrs {
			attrs = append(attrs, string(a))
		}
		line := fmt.Sprintf("  %-28q delim %q attrs [%s]", b.Mailbox, string(b.Delim), strings.Join(attrs, " "))
		if slices.Contains(b.Attrs, imap.MailboxAttrNoSelect) || slices.Contains(b.Attrs, imap.MailboxAttrNonExistent) {
			p.printf("%s\n", line)
			continue
		}
		st, err := c.Status(b.Mailbox, statusOpts).Wait()
		if err != nil {
			p.printf("%s status: %v\n", line, err)
			continue
		}
		msgs := uint32(0)
		if st.NumMessages != nil {
			msgs = *st.NumMessages
		}
		p.printf("%s messages %d uidnext %d uidvalidity %d modseq %d\n", line, msgs, st.UIDNext, st.UIDValidity, st.HighestModSeq)
	}
	sel, err := c.Select("INBOX", &imap.SelectOptions{ReadOnly: true, CondStore: caps.Has(imap.CapCondStore)}).Wait()
	if err != nil {
		return fmt.Errorf("examine INBOX: %w", err)
	}
	var perm []string
	for _, f := range sel.PermanentFlags {
		perm = append(perm, string(f))
	}
	p.printf("  INBOX: %d messages, uidnext %d, highestmodseq %d, permanent flags [%s]\n",
		sel.NumMessages, sel.UIDNext, sel.HighestModSeq, strings.Join(perm, " "))
	if sel.NumMessages > 0 {
		from := uint32(1)
		if sel.NumMessages > 5 {
			from = sel.NumMessages - 4
		}
		var seqs imap.SeqSet
		seqs.AddRange(from, sel.NumMessages)
		msgs, err := c.Fetch(seqs, &imap.FetchOptions{
			UID: true, Flags: true, InternalDate: true, RFC822Size: true, ModSeq: caps.Has(imap.CapCondStore),
			BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
		}).Collect()
		if err != nil {
			return fmt.Errorf("fetch: %w", err)
		}
		for _, m := range msgs {
			var flags []string
			for _, f := range m.Flags {
				flags = append(flags, string(f))
			}
			p.printf("  uid %d size %d date %s modseq %d flags [%s]\n", m.UID, m.RFC822Size,
				m.InternalDate.UTC().Format(time.RFC3339), m.ModSeq, strings.Join(flags, " "))
		}
	}
	// How many connections the account may hold at once.
	var extra []io.Closer
	defer func() {
		for _, x := range extra {
			_ = x.Close()
		}
	}()
	for n := 2; n <= 6; n++ {
		c2, err := imapx.Dial(ctx, imapx.DialOptions{Host: opts.Host, Port: opts.Port, TLS: opts.TLS,
			Username: opts.Username, Password: opts.Password, OAuth: true, ReadOnly: true})
		if err != nil {
			p.printf("  connection %d refused: %v\n", n, err)
			break
		}
		extra = append(extra, c2)
	}
	p.printf("  held %d connections at once\n", len(extra)+1)
	return nil
}

// flags selects INBOX read-write and tries the flags Frostmail writes on
// the probe's own message (subject "FM probe"): \Flagged with a color
// ($MailFlagBit0), $Forwarded and a keyword, reading each back, then
// restores the message's flags.
//
// unflag only removes \Flagged and $Forwarded from that message.
func (p *probe) flags(ctx context.Context, unflag bool) error {
	tok, s, err := p.access(ctx, outlookScopes[:1])
	if err != nil {
		return err
	}
	c, err := imapx.Dial(ctx, imapx.DialOptions{Host: imapHosts[0], Port: 993, TLS: api.TLSModeTLS, Username: s.Email, Password: tok, OAuth: true})
	if err != nil {
		return err
	}
	defer func() { _ = c.Logout().Wait() }()
	sel, err := c.Select("INBOX", nil).Wait()
	if err != nil {
		return err
	}
	var perm []string
	for _, f := range sel.PermanentFlags {
		perm = append(perm, string(f))
	}
	p.printf("SELECT INBOX: flags [%v] permanent flags [%s]\n", sel.Flags, strings.Join(perm, " "))
	found, err := c.UIDSearch(&imap.SearchCriteria{Header: []imap.SearchCriteriaHeaderField{{Key: "Subject", Value: "FM probe"}}}, nil).Wait()
	if err != nil {
		return err
	}
	uids := found.AllUIDs()
	if len(uids) == 0 {
		return errors.New("no FM probe message in INBOX")
	}
	uid := uids[len(uids)-1]
	set := imap.UIDSetNum(uid)
	read := func(step string) []imap.Flag {
		msgs, err := c.Fetch(set, &imap.FetchOptions{UID: true, Flags: true}).Collect()
		if err != nil || len(msgs) == 0 {
			p.printf("  %s: fetch: %v\n", step, err)
			return nil
		}
		p.printf("  %s: %v\n", step, msgs[0].Flags)
		return msgs[0].Flags
	}
	before := read("before")
	if unflag {
		err := c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsDel, Silent: true, Flags: []imap.Flag{imap.FlagFlagged, "$Forwarded"}}, nil).Close()
		if err != nil {
			return err
		}
		read("unflagged")
		return nil
	}
	for _, try := range [][]imap.Flag{{imap.FlagFlagged, "$MailFlagBit0"}, {"$Forwarded"}, {"FMProbeKeyword"}} {
		err := c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: try}, nil).Close()
		p.printf("  STORE +FLAGS %v: %v\n", try, err)
		read("after")
	}
	// Restore what the message had; \Recent belongs to the server.
	keep := slices.DeleteFunc(slices.Clone(before), func(f imap.Flag) bool { return strings.EqualFold(string(f), `\Recent`) })
	if err := c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsSet, Silent: true, Flags: keep}, nil).Close(); err != nil {
		return fmt.Errorf("restore flags: %w", err)
	}
	read("restored")
	return nil
}

// smtp signs in to Outlook.com's submission server, sending nothing.
func (p *probe) smtp(ctx context.Context) error {
	tok, s, err := p.access(ctx, outlookScopes[1:])
	if err != nil {
		return err
	}
	for _, host := range []string{"smtp-mail.outlook.com", "smtp.office365.com"} {
		opts := smtpx.Options{Host: host, Port: 587, TLS: api.TLSModeStartTLS, Username: s.Email, Password: tok, OAuth: true}
		size, err := smtpx.MaxSize(ctx, opts)
		if err != nil {
			p.printf("== SMTP %s: %v\n", host, err)
			continue
		}
		p.printf("== SMTP %s: signed in; SIZE %d\n", host, size)
	}
	return nil
}

// send mails one probe message from the account to itself and watches
// whether Outlook.com files a copy in Sent, as Gmail does and iCloud does
// not (the profile's SavesSent).
func (p *probe) send(ctx context.Context) error {
	tok, s, err := p.access(ctx, outlookScopes)
	if err != nil {
		return err
	}
	opts := imapx.DialOptions{Host: imapHosts[0], Port: 993, TLS: api.TLSModeTLS, Username: s.Email, Password: tok, OAuth: true, ReadOnly: true}
	count := func(box string) (uint32, error) {
		c, err := imapx.Dial(ctx, opts)
		if err != nil {
			return 0, err
		}
		defer func() { _ = c.Logout().Wait() }()
		st, err := c.Status(box, &imap.StatusOptions{NumMessages: true}).Wait()
		if err != nil || st.NumMessages == nil {
			return 0, fmt.Errorf("status %s: %w", box, err)
		}
		return *st.NumMessages, nil
	}
	sentBefore, err := count("Sent")
	if err != nil {
		return err
	}
	inboxBefore, err := count("INBOX")
	if err != nil {
		return err
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	msg := "From: <" + s.Email + ">\r\nTo: <" + s.Email + ">\r\nSubject: FM probe " + stamp +
		"\r\nDate: " + time.Now().Format(time.RFC1123Z) + "\r\nMessage-ID: <fm-probe-" + strconv.FormatInt(time.Now().UnixNano(), 36) +
		"@frostmail.invalid>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nFrostmail's Outlook.com probe (plan 0010). Safe to delete.\r\n"
	sopts := smtpx.Options{Host: "smtp-mail.outlook.com", Port: 587, TLS: api.TLSModeStartTLS, Username: s.Email, Password: tok, OAuth: true}
	if err := smtpx.Send(ctx, sopts, smtpx.Envelope{From: s.Email, To: []string{s.Email}}, int64(len(msg)), strings.NewReader(msg)); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	p.printf("sent %q to %s at %s; Sent had %d, INBOX %d\n", "FM probe "+stamp, s.Email, stamp, sentBefore, inboxBefore)
	for wait := 0; wait < 12; wait++ {
		time.Sleep(5 * time.Second)
		sent, err := count("Sent")
		if err != nil {
			return err
		}
		inbox, err := count("INBOX")
		if err != nil {
			return err
		}
		p.printf("  after %2ds: Sent %d, INBOX %d\n", 5*(wait+1), sent, inbox)
		if sent > sentBefore && inbox > inboxBefore {
			break
		}
	}
	return nil
}

// follow reads a saved delta link to its end and prints what each change
// carries, deletions included, read-only.
func (p *probe) follow(ctx context.Context, link string) error {
	if link == "" {
		return errors.New("follow needs -link")
	}
	tok, _, err := p.access(ctx, graphScopes)
	if err != nil {
		return err
	}
	g := graphClient{p: p, token: tok}
	headers := map[string]string{"Prefer": `outlook.timezone="UTC", IdType="ImmutableId"`}
	for i := 0; link != "" && i < 50; i++ {
		doc := g.get(ctx, fmt.Sprintf("follow-%d", i), link, headers)
		for _, v := range values(doc) {
			p.printf("  %v %v %v %v removed=%v keys=%d\n", v["@odata.type"], v["type"], v["subject"], dig(v, "start", "dateTime"), v["@removed"], len(v))
		}
		link = ""
		if next, ok := doc["@odata.nextLink"].(string); ok {
			link = next
		}
	}
	return nil
}

// graph reads the account's profile, calendars and contacts, read-only,
// and asks for delta where Graph may offer it.
func (p *probe) graph(ctx context.Context) error {
	tok, _, err := p.access(ctx, graphScopes)
	if err != nil {
		return err
	}
	if p.out != "" {
		if err := os.MkdirAll(p.out, 0o700); err != nil {
			return err
		}
	}
	g := graphClient{p: p, token: tok}
	g.get(ctx, "me", "/me", nil)
	cals := g.get(ctx, "calendars", "/me/calendars", nil)
	for i, c := range values(cals) {
		p.printf("  calendar %d: %v (owner %v, canEdit %v, color %v, hexColor %v, isDefault %v)\n", i,
			c["name"], dig(c, "owner", "address"), c["canEdit"], c["color"], c["hexColor"], c["isDefaultCalendar"])
	}
	now := time.Now().UTC()
	window := url.Values{"startDateTime": {now.AddDate(-1, 0, 0).Format(time.RFC3339)}, "endDateTime": {now.AddDate(1, 0, 0).Format(time.RFC3339)}}
	zone := map[string]string{"Prefer": `outlook.timezone="UTC", IdType="ImmutableId"`}
	g.pages(ctx, "calendarview-delta", "/me/calendarView/delta?"+window.Encode(), zone)
	g.pages(ctx, "events", "/me/events?$top=50", zone)
	masters := g.get(ctx, "series-masters", "/me/events?$filter="+url.QueryEscape("type eq 'seriesMaster'")+"&$top=10", zone)
	for i, m := range values(masters) {
		id, _ := m["id"].(string)
		p.printf("  series %d: recurrence %v\n", i, m["recurrence"])
		g.get(ctx, fmt.Sprintf("series-%d-instances", i), "/me/events/"+url.PathEscape(id)+"/instances?"+window.Encode(), zone)
	}
	folders := g.get(ctx, "contact-folders", "/me/contactFolders", nil)
	for i, f := range values(folders) {
		p.printf("  contact folder %d: %v\n", i, f["displayName"])
	}
	g.pages(ctx, "contacts", "/me/contacts?$top=50", map[string]string{"Prefer": `IdType="ImmutableId"`})
	g.pages(ctx, "contacts-delta", "/me/contacts/delta", map[string]string{"Prefer": `IdType="ImmutableId"`})
	for i, f := range values(folders) {
		id, _ := f["id"].(string)
		g.pages(ctx, fmt.Sprintf("folder-%d-contacts-delta", i), "/me/contactFolders/"+url.PathEscape(id)+"/contacts/delta", nil)
	}
	g.get(ctx, "todo-lists", "/me/todo/lists", nil)
	return nil
}

type graphClient struct {
	p     *probe
	token string
}

// get fetches one Graph URL (a path under v1.0, or a full next/delta link),
// prints its status, size and the shape of what came back, and saves it.
func (g graphClient) get(ctx context.Context, name, path string, headers map[string]string) map[string]any {
	u := path
	if strings.HasPrefix(path, "/") {
		u = graphURL + path
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		g.p.printf("== %s: %v\n", name, err)
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := g.p.http.Do(req)
	if err != nil {
		g.p.printf("== %s: %v\n", name, err)
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if g.p.out != "" {
		_ = os.WriteFile(filepath.Join(g.p.out, name+".json"), body, 0o600)
	}
	var doc map[string]any
	_ = json.Unmarshal(body, &doc)
	g.p.printf("== %s: HTTP %d, %d bytes, %s", name, resp.StatusCode, len(body), time.Since(start).Round(time.Millisecond))
	if resp.StatusCode != http.StatusOK {
		g.p.printf(": %s\n", strings.TrimSpace(string(body[:min(len(body), 400)])))
		return doc
	}
	vals := values(doc)
	g.p.printf(", %d values", len(vals))
	for _, k := range []string{"@odata.nextLink", "@odata.deltaLink"} {
		if _, ok := doc[k]; ok {
			g.p.printf(", %s", k)
		}
	}
	g.p.printf("\n")
	if len(vals) > 0 {
		g.p.printf("  keys of the first: %s\n", strings.Join(keys(vals[0]), " "))
	}
	return doc
}

// pages follows nextLinks to the end and reports how a delta round ends.
func (g graphClient) pages(ctx context.Context, name, path string, headers map[string]string) {
	total := 0
	for i := 0; path != "" && i < 50; i++ {
		doc := g.get(ctx, fmt.Sprintf("%s-%d", name, i), path, headers)
		if doc == nil {
			return
		}
		total += len(values(doc))
		path = ""
		if next, ok := doc["@odata.nextLink"].(string); ok {
			path = next
		} else if delta, ok := doc["@odata.deltaLink"].(string); ok {
			g.p.printf("  %s: %d values in all; the delta link is %d bytes\n", name, total, len(delta))
		}
	}
}

func values(doc map[string]any) []map[string]any {
	raw, _ := doc["value"].([]any)
	var out []map[string]any
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func dig(m map[string]any, path ...string) any {
	var cur any = m
	for _, k := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

// decodeSegment decodes a JWT segment (unpadded base64url).
func decodeSegment(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
