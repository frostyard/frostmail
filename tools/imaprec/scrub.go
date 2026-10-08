package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// The scrubber replaces what an account's mail says and keeps what the
// protocol needs. A word (a run of ASCII letters and digits, or of bytes
// above 0x7f) becomes a fake of the same length and character classes,
// derived from a secret key, so the same word is the same fake everywhere:
// a Message-ID in an ENVELOPE still matches the References header that
// cites it, a mailbox in LIST still matches the SELECT the client sends,
// and a Gmail label still matches its mailbox. Words of one character,
// and the protocol, MIME, header and date words in vocabulary, stay.
// Envelope dates, INTERNALDATE, MIME types and the parameters that steer
// decoding, flags, UIDs and numbers outside strings stay as recorded.

type scrubber struct {
	key     []byte
	fakes   map[string]string // word → its fake
	secrets map[string]bool   // the words replaced
}

func newScrubber(key []byte) *scrubber {
	return &scrubber{key: key, fakes: map[string]string{}, secrets: map[string]bool{}}
}

func wordByte(c byte) bool {
	return c >= 0x80 || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func hexByte(c byte) bool { return c >= '0' && c <= '9' || c >= 'A' && c <= 'F' }

// text replaces the words of s. A quoted-printable escape (=XX, in bodies
// and in encoded words) becomes =3D, which still decodes, so neither the
// encoded text nor an invalid escape survives.
func (s *scrubber) text(t string) string {
	var b strings.Builder
	for i := 0; i < len(t); {
		if t[i] == '=' && i+2 < len(t) && hexByte(t[i+1]) && hexByte(t[i+2]) {
			b.WriteString("=3D")
			i += 3
			continue
		}
		if !wordByte(t[i]) {
			b.WriteByte(t[i])
			i++
			continue
		}
		j := i
		for j < len(t) && wordByte(t[j]) {
			j++
		}
		b.WriteString(s.word(t[i:j]))
		i = j
	}
	return b.String()
}

// word returns w's fake, or w when it stays.
func (s *scrubber) word(w string) string {
	if len(w) <= 1 || vocabulary[strings.ToLower(w)] {
		return w
	}
	if f, ok := s.fakes[w]; ok {
		return f
	}
	mac := hmac.New(sha256.New, s.key)
	var sum []byte
	out := make([]byte, len(w))
	for i := range len(w) {
		if i%sha256.Size == 0 {
			mac.Reset()
			fmt.Fprintf(mac, "%d:%s", i, w)
			sum = mac.Sum(nil)
		}
		r := sum[i%sha256.Size]
		switch c := w[i]; {
		case c >= '0' && c <= '9':
			out[i] = '0' + r%10
		case c >= 'A' && c <= 'Z':
			out[i] = 'A' + r%26
		default: // lower case, and bytes of non-ASCII text
			out[i] = 'a' + r%26
		}
	}
	f := string(out)
	s.fakes[w] = f
	s.secrets[w] = true
	return f
}

// mailbox replaces a mailbox name's words. INBOX and Gmail's system labels
// (\Inbox, \Starred) stay; modified UTF-7 runs (&…-) are scrubbed as the
// letters they are written with.
func (s *scrubber) mailbox(name string) string {
	if strings.EqualFold(name, "INBOX") || strings.HasPrefix(name, `\`) {
		return name
	}
	var b strings.Builder
	for rest := name; rest != ""; {
		amp := strings.IndexByte(rest, '&')
		dash := -1
		if amp >= 0 {
			dash = strings.IndexByte(rest[amp:], '-')
		}
		if amp < 0 || dash <= 1 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:amp] + rest[amp+1:amp+dash])
		rest = rest[amp+dash+1:]
	}
	return s.text(b.String())
}

// leaks lists the replaced words of six or more characters, with a
// letter, that still appear in out. Shorter words are left out: a fake of
// four letters matches some real word too often to mean anything.
func (s *scrubber) leaks(out string) []string {
	var found []string
	seen := map[string]bool{}
	for i := 0; i < len(out); {
		if !wordByte(out[i]) {
			i++
			continue
		}
		j := i
		for j < len(out) && wordByte(out[j]) {
			j++
		}
		w := out[i:j]
		i = j
		if len(w) >= 6 && s.secrets[w] && !seen[w] && strings.ContainsFunc(w, func(r rune) bool { return r > '9' }) {
			seen[w] = true
			found = append(found, w)
		}
	}
	slices.Sort(found)
	return found
}

func (s *scrubber) str(n *node) {
	if n.isString() {
		n.text = s.text(n.text)
	}
}

// any replaces the words of every string in n.
func (s *scrubber) any(n *node) {
	s.str(n)
	for _, item := range n.items {
		s.any(item)
	}
}

func (s *scrubber) mailboxNode(n *node) {
	if n.isString() || n.kind == atom && !strings.EqualFold(n.text, "NIL") {
		n.text = s.mailbox(n.text)
	}
}

// keywords replaces the words of flag keywords; system flags (\Seen) stay.
func (s *scrubber) keywords(n *node) {
	for _, f := range n.items {
		if f.kind == atom && !strings.HasPrefix(f.text, `\`) {
			f.text = s.text(f.text)
		}
	}
}

// command scrubs a line the client sent: mailbox names, Gmail labels,
// keywords and search strings. The rest of a command is the client's own
// protocol and stays.
func (s *scrubber) command(line string) (string, error) {
	tag, rest, ok := strings.Cut(line, " ")
	if !ok || rest == "[redacted]" {
		return line, nil // DONE, a redacted SASL line
	}
	nodes, err := parseSeq(rest)
	if err != nil || len(nodes) == 0 {
		return "", fmt.Errorf("cannot parse the command %.60q: %v", line, err)
	}
	name, args := nodes[0].upper(), nodes[1:]
	if name == "UID" && len(args) > 0 {
		name, args = name+" "+args[0].upper(), args[1:]
	}
	var plain []*node // arguments other than lists
	for _, a := range args {
		if a.kind != list {
			plain = append(plain, a)
		}
	}
	switch name {
	case "SELECT", "EXAMINE", "STATUS", "CREATE", "DELETE", "SUBSCRIBE", "UNSUBSCRIBE", "APPEND", "GETQUOTAROOT":
		if len(plain) > 0 {
			s.mailboxNode(plain[0])
		}
	case "RENAME", "LIST", "LSUB", "XLIST":
		for _, a := range plain[:min(2, len(plain))] {
			s.mailboxNode(a)
		}
	case "COPY", "MOVE", "UID COPY", "UID MOVE":
		if len(plain) > 0 {
			s.mailboxNode(plain[len(plain)-1])
		}
	case "STORE", "UID STORE":
		if len(args) >= 3 {
			s.storeItems(args[1].upper(), args[2])
		}
	case "SEARCH", "UID SEARCH":
		for _, a := range args {
			s.any(a)
		}
	}
	return tag + " " + format(nodes), nil
}

// storeItems scrubs the values of a STORE: labels or keywords.
func (s *scrubber) storeItems(item string, value *node) {
	values := []*node{value}
	if value.kind == list {
		values = value.items
	}
	for _, v := range values {
		switch {
		case strings.Contains(item, "X-GM-LABELS"):
			s.mailboxNode(v)
		case strings.Contains(item, "FLAGS") && v.kind == atom && !strings.HasPrefix(v.text, `\`):
			v.text = s.text(v.text)
		}
	}
}

// response scrubs one server response.
func (s *scrubber) response(text string) (string, error) {
	tag, rest, spaced := strings.Cut(text, " ")
	if tag == "+" {
		if !spaced {
			return text, nil
		}
		return "+ " + s.text(rest), nil
	}
	first, after, _ := strings.Cut(rest, " ")
	if _, err := strconv.ParseUint(first, 10, 32); err == nil && tag == "*" {
		word, data, _ := strings.Cut(after, " ")
		if !strings.EqualFold(word, "FETCH") {
			return text, nil // EXISTS, EXPUNGE, RECENT
		}
		nodes, err := parseSeq(data)
		if err != nil || len(nodes) == 0 || nodes[0].kind != list {
			return "", fmt.Errorf("cannot parse the FETCH response %.60q: %v", text, err)
		}
		if err := s.fetch(nodes[0]); err != nil {
			return "", err
		}
		return tag + " " + first + " " + word + " " + format(nodes), nil
	}
	prefix := tag + " " + first + " "
	switch strings.ToUpper(first) {
	case "OK", "NO", "BAD", "BYE", "PREAUTH":
		return tag + " " + first + s.respText(after, rest != first), nil
	case "CAPABILITY", "ENABLED", "SEARCH", "ESEARCH", "VANISHED", "NAMESPACE":
		return text, nil
	case "FLAGS":
		return s.data(prefix, after, func(nodes []*node) {
			if len(nodes) > 0 {
				s.keywords(nodes[0])
			}
		})
	case "LIST", "LSUB", "XLIST":
		return s.data(prefix, after, func(nodes []*node) {
			for i, n := range nodes {
				switch {
				case i == 2:
					s.mailboxNode(n)
				case i > 2:
					s.any(n)
				}
			}
		})
	case "STATUS":
		return s.data(prefix, after, func(nodes []*node) {
			if len(nodes) > 0 {
				s.mailboxNode(nodes[0])
			}
		})
	}
	if rest == "" {
		return text, nil
	}
	// Anything else (ID, QUOTA, METADATA): every string.
	return s.data(prefix, after, func(nodes []*node) {
		for _, n := range nodes {
			s.any(n)
		}
	})
}

// data parses a response's data, lets f scrub it and prints it back.
func (s *scrubber) data(prefix, data string, f func([]*node)) (string, error) {
	nodes, err := parseSeq(data)
	if err != nil {
		return "", fmt.Errorf("cannot parse %.60q: %w", prefix+data, err)
	}
	f(nodes)
	return prefix + format(nodes), nil
}

// respText scrubs a status response's text after its keyword: a response
// code in brackets stays (its keywords aside), the human text does not.
func (s *scrubber) respText(t string, spaced bool) string {
	if !spaced {
		return ""
	}
	if strings.HasPrefix(t, "[") {
		end := strings.IndexByte(t, ']')
		if end > 0 {
			code := t[:end+1]
			if strings.HasPrefix(strings.ToUpper(code), "[PERMANENTFLAGS ") {
				if nodes, err := parseSeq(code[len("[PERMANENTFLAGS ") : len(code)-1]); err == nil && len(nodes) > 0 {
					s.keywords(nodes[0])
					code = code[:len("[PERMANENTFLAGS ")] + format(nodes) + "]"
				}
			}
			return " " + code + s.text(t[end+1:])
		}
	}
	return " " + s.text(t)
}

// fetch scrubs a FETCH response's items.
func (s *scrubber) fetch(n *node) error {
	if len(n.items)%2 != 0 {
		return fmt.Errorf("a FETCH response has an item without a value")
	}
	for i := 0; i < len(n.items); i += 2 {
		name, v := n.items[i].upper(), n.items[i+1]
		base, section := name, false
		if j := strings.IndexAny(name, "[<"); j >= 0 {
			base, section = name[:j], true
		}
		switch {
		case base == "FLAGS":
			s.keywords(v)
		case base == "X-GM-LABELS":
			for _, label := range v.items {
				s.mailboxNode(label)
			}
		case base == "ENVELOPE":
			s.envelope(v)
		case (base == "BODYSTRUCTURE" || base == "BODY") && !section:
			s.body(v)
		case keptItems[base]:
		default: // BODY[…], BINARY[…], RFC822…, and anything unknown
			s.any(v)
		}
	}
	return nil
}

// keptItems carry no text of the mail's own.
var keptItems = map[string]bool{
	"UID": true, "RFC822.SIZE": true, "MODSEQ": true, "INTERNALDATE": true, "SAVEDATE": true,
	"X-GM-MSGID": true, "X-GM-THRID": true, "EMAILID": true, "THREADID": true, "BINARY.SIZE": true,
}

// envelope scrubs an ENVELOPE; its date stays.
func (s *scrubber) envelope(n *node) {
	if n.kind != list || len(n.items) < 10 {
		s.any(n)
		return
	}
	s.str(n.items[1]) // subject
	for _, addrs := range n.items[2:8] {
		for _, a := range addrs.items {
			for j, field := range a.items {
				if j != 1 { // name, mailbox, host; not the source route
					s.str(field)
				}
			}
		}
	}
	s.str(n.items[8]) // in-reply-to
	s.str(n.items[9]) // message-id
}

// body scrubs a BODYSTRUCTURE: types, encodings, sizes and the parameters
// that steer decoding stay; names, IDs, descriptions, boundaries and
// locations do not.
func (s *scrubber) body(n *node) {
	if n.kind != list || len(n.items) == 0 {
		return
	}
	items := n.items
	if items[0].kind == list { // multipart: parts, subtype, extensions
		i := 0
		for i < len(items) && items[i].kind == list {
			s.body(items[i])
			i++
		}
		if i < len(items) {
			s.extensions(items[i+1:], true)
		}
		return
	}
	if len(items) < 7 {
		s.any(n)
		return
	}
	s.params(items[2])
	s.str(items[3]) // id
	s.str(items[4]) // description
	rest := items[7:]
	switch typ, sub := items[0].upper(), items[1].upper(); {
	case typ == "TEXT" && len(rest) > 0:
		rest = rest[1:] // lines
	case typ == "MESSAGE" && (sub == "RFC822" || sub == "GLOBAL") && len(rest) >= 3:
		s.envelope(rest[0])
		s.body(rest[1])
		rest = rest[3:]
	}
	s.extensions(rest, false)
}

// extensions scrubs a body's extension data: md5 (one part) or parameters
// (multipart), disposition, language (stays), location, and the rest.
func (s *scrubber) extensions(ext []*node, multipart bool) {
	for i, n := range ext {
		switch i {
		case 0:
			if multipart {
				s.params(n)
			} else {
				s.str(n)
			}
		case 1:
			if n.kind == list && len(n.items) >= 2 {
				s.params(n.items[1])
			}
		case 2:
		default:
			s.any(n)
		}
	}
}

// params scrubs a parameter list's values, except those that steer
// decoding.
func (s *scrubber) params(n *node) {
	for i := 0; i+1 < len(n.items); i += 2 {
		if !keptParams[n.items[i].upper()] {
			s.str(n.items[i+1])
		}
	}
}

var keptParams = map[string]bool{
	"CHARSET": true, "FORMAT": true, "DELSP": true, "REPORT-TYPE": true, "PROTOCOL": true,
	"MICALG": true, "TYPE": true, "METHOD": true, "SMIME-TYPE": true,
}

// vocabulary holds the words that stay wherever they appear: protocol,
// MIME, header, date and provider words, which say nothing about the
// account. Lower case.
var vocabulary = func() map[string]bool {
	words := `
ok no bad bye preauth completed complete success successful selected select examine search fetch list
status store copy move append expunge idle idling noop logout capability authenticated authentication
logged in ready requests from for the and or of to is are be with valid uids uid uidnext uidvalidity
highestmodseq modseq predicted next flags permitted permanent read write only took ms done
continuation literal data server mailbox mailboxes throttled try again later command commands
inbox gmail google gimap imap mail all spam trash drafts draft sent starred important junk archive
archives notes deleted messages message items recovered outbox chats icloud apple me mac
nonexistent haschildren hasnochildren noinferiors noselect marked unmarked subscribed remote
text plain html enriched multipart alternative mixed related report signed encrypted digest parallel
rfc822 global delivery disposition notification partial external body image png jpeg jpg gif webp
bmp tiff svg xml audio video application pdf octet stream zip json msword calendar ics vcard pgp
signature pkcs7 mime smime charset utf us ascii iso latin windows cp koi8 gb2312 gbk big5 euc kr jp
shift jis 8859 1252 2022 quoted printable base64 7bit 8bit binary name filename boundary format
flowed fixed delsp yes attachment inline type protocol micalg sha1 sha256 sha512 method request
references reply list id unsubscribe post authentication results content transfer encoding version
date subject cc bcc sender received return path mailer user agent priority importance thread index
topic delivered dkim arc seal spf dmarc pass fail softfail neutral none temperror permerror policy
header smtp mailfrom helo reject quarantine mx dara sig1 bh rsa ed25519 relaxed simple bounces
noreply mailto http https www com org net edu gov io uk co de
mon tue wed thu fri sat sun jan feb mar apr may jun jul aug sep oct nov dec gmt utc ut est edt cst
cdt mst mdt pst pdt
vendor support url host os arguments environment address
re fwd fw aw wg sv vs antw tr rif enc
seen answered flagged recent forwarded mdnsent notjunk nonjunk phishing notphishing redirected
mailflagbit0 mailflagbit1 mailflagbit2 icloudcleanup label1 label2 label3 label4 label5
submitpending submitted
frostmail frostyard
`
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}()
