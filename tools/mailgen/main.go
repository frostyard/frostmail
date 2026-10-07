// Command mailgen writes a reproducible Maildir of N synthetic messages for
// load tests (make mailtest-seed). Task T-0021 implements it.
//
//	go run ./tools/mailgen -n 50000 -seed 1 -out DIR
package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Options say what to generate.
type Options struct {
	N    int    // number of messages
	Seed uint64 // the same seed always produces the same bytes
	Out  string // Maildir root; cur/, new/ and tmp/ are created
}

var (
	firstNames = []string{"James", "Maria", "David", "Aisha", "Peter", "Lena", "Omar", "Nina", "Victor", "Iris"}
	lastNames  = []string{"Brown", "Davis", "Lopez", "Walker", "Young", "King", "Ward", "Price"}
	companies  = []string{"acme", "northwind", "vertex"}
	words      = []string{
		"backup", "cabinet", "delivery", "engine", "filter", "garden", "harbor",
		"island", "jigsaw", "kernel", "ladder", "market", "network", "orbit",
		"packet", "quartz", "router", "signal", "thread", "update", "volume",
		"window", "yield", "zone", "anchor", "bridge", "canvas", "drift",
		"ember", "fossil", "grain", "hollow", "index", "junction", "keystone",
		"ledger", "meadow", "nimbus", "opcode", "prairie", "quiver", "ribbon",
		"shard", "timber", "umbrella", "vector", "wallet", "zenith", "atlas",
		"beacon", "cinder", "delta", "echo", "flare", "glint", "halo", "input",
	}
)

type sender struct {
	name string
	addr string
}

// record keeps what later replies need about a message: its Message-ID, its
// References and its subject without any "Re: " prefix.
type record struct {
	id      string
	refs    string // space-joined References, empty for a new thread
	baseSub string
	flags   string
}

// Generate writes the Maildir: cur/, new/ and tmp/ under o.Out, and N
// messages in cur/, byte-for-byte reproducible from o.Seed.
func Generate(o Options) error {
	for _, sub := range []string{"cur", "new", "tmp"} {
		if err := os.MkdirAll(filepath.Join(o.Out, sub), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", sub, err)
		}
	}
	rnd := rand.New(rand.NewPCG(o.Seed, o.Seed^0x9e3779b97f4a7c15))
	senders := buildSenders()
	base := time.Date(2025, 10, 7, 0, 0, 0, 0, time.UTC)
	step := 365 * 24 * time.Hour / time.Duration(o.N)
	prev := make([]record, 0, o.N)
	for i := 1; i <= o.N; i++ {
		rec, msg := buildMessage(rnd, o.Seed, i, base.Add(time.Duration(i)*step), senders, prev)
		prev = append(prev, rec)
		name := fmt.Sprintf("%07d.mailgen:2,%s", i, rec.flags)
		if err := os.WriteFile(filepath.Join(o.Out, "cur", name), msg, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

func buildSenders() []sender {
	senders := make([]sender, 0, len(firstNames)*len(lastNames)*len(companies))
	for _, f := range firstNames {
		for _, l := range lastNames {
			for _, c := range companies {
				senders = append(senders, sender{
					name: f + " " + l,
					addr: strings.ToLower(f) + "." + strings.ToLower(l) + "@" + c + ".test",
				})
			}
		}
	}
	return senders
}

func buildMessage(rnd *rand.Rand, seed uint64, i int, date time.Time, senders []sender, prev []record) (record, []byte) {
	rec := record{id: fmt.Sprintf("<gen-%d-%d@mailgen.test>", seed, i)}
	if i > 1 && rnd.Float64() < 0.25 {
		window := min(i-1, 200)
		p := prev[i-1-window+rnd.IntN(window)]
		rec.baseSub = p.baseSub
		rec.refs = p.id
		if p.refs != "" {
			rec.refs = p.refs + " " + p.id
		}
	} else {
		rec.baseSub = randomSubject(rnd)
	}
	if rnd.Float64() < 0.05 {
		rec.flags += "F"
	}
	if rec.refs != "" && rnd.Float64() < 0.3 {
		rec.flags += "R"
	}
	if rnd.Float64() < 0.6 {
		rec.flags += "S"
	}

	var buf bytes.Buffer
	w := func(line string) {
		buf.WriteString(line)
		buf.WriteString("\r\n")
	}
	fromIdx := rnd.IntN(len(senders))
	from := senders[fromIdx]
	w("Date: " + date.Format(time.RFC1123Z))
	w("From: " + from.name + " <" + from.addr + ">")
	w("To: Test <test@mailtest.test>")
	if rnd.Float64() < 0.2 {
		cc := senders[(fromIdx+1+rnd.IntN(len(senders)-1))%len(senders)]
		w("Cc: " + cc.name + " <" + cc.addr + ">")
	}
	subject := rec.baseSub
	if rec.refs != "" {
		subject = "Re: " + rec.baseSub
	}
	w("Subject: " + subject)
	w("Message-ID: " + rec.id)
	if rec.refs != "" {
		w("In-Reply-To: " + rec.refs[strings.LastIndexByte(rec.refs, ' ')+1:])
		w("References: " + rec.refs)
	}
	w("MIME-Version: 1.0")
	writeBody(&buf, rnd, i)
	return rec, buf.Bytes()
}

func writeBody(buf *bytes.Buffer, rnd *rand.Rand, i int) {
	w := func(line string) {
		buf.WriteString(line)
		buf.WriteString("\r\n")
	}
	text := bodyText(rnd)
	boundary := fmt.Sprintf("b%d", i)
	switch kind := rnd.Float64(); {
	case kind < 0.7:
		w("Content-Type: text/plain; charset=utf-8")
		buf.WriteString("\r\n")
		buf.WriteString(text)
		buf.WriteString("\r\n")
	case kind < 0.9:
		w(`Content-Type: multipart/alternative; boundary="` + boundary + `"`)
		buf.WriteString("\r\n")
		buf.WriteString("--" + boundary + "\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
		buf.WriteString(text)
		buf.WriteString("\r\n--" + boundary + "\r\nContent-Type: text/html; charset=utf-8\r\n\r\n")
		buf.WriteString(htmlBody(text))
		buf.WriteString("\r\n--" + boundary + "--\r\n")
	default:
		w(`Content-Type: multipart/mixed; boundary="` + boundary + `"`)
		buf.WriteString("\r\n")
		buf.WriteString("--" + boundary + "\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
		buf.WriteString(text)
		buf.WriteString("\r\n--" + boundary + "\r\n")
		name := fmt.Sprintf("report-%d.csv", i)
		w(`Content-Type: text/csv; name="` + name + `"`)
		w("Content-Transfer-Encoding: base64")
		w(`Content-Disposition: attachment; filename="` + name + `"`)
		buf.WriteString("\r\n")
		buf.WriteString(attachmentCSV(rnd))
		buf.WriteString("--" + boundary + "--\r\n")
	}
}

func bodyText(rnd *rand.Rand) string {
	paras := make([]string, 0, 4)
	for range 1 + rnd.IntN(4) {
		paras = append(paras, wrap72(randomWords(rnd, 20+rnd.IntN(41))))
	}
	return strings.Join(paras, "\r\n\r\n")
}

func htmlBody(text string) string {
	var b strings.Builder
	b.WriteString("<html>\r\n<body>\r\n")
	for p := range strings.SplitSeq(text, "\r\n\r\n") {
		b.WriteString("<p>" + p + "</p>\r\n")
	}
	b.WriteString("</body>\r\n</html>")
	return b.String()
}

func attachmentCSV(rnd *rand.Rand) string {
	var csv strings.Builder
	csv.WriteString("id,name,value\n")
	for row := range 8 {
		fmt.Fprintf(&csv, "%d,%s,%d\n", row+1, words[rnd.IntN(len(words))], rnd.IntN(1000))
	}
	const lineLen = 76
	b64 := base64.StdEncoding.EncodeToString([]byte(csv.String()))
	var out strings.Builder
	for len(b64) > lineLen {
		out.WriteString(b64[:lineLen] + "\r\n")
		b64 = b64[lineLen:]
	}
	out.WriteString(b64 + "\r\n")
	return out.String()
}

func randomWords(rnd *rand.Rand, n int) []string {
	out := make([]string, n)
	for k := range out {
		out[k] = words[rnd.IntN(len(words))]
	}
	return out
}

func randomSubject(rnd *rand.Rand) string {
	s := strings.Join(randomWords(rnd, 3+rnd.IntN(5)), " ")
	return strings.ToUpper(s[:1]) + s[1:]
}

func wrap72(ws []string) string {
	var lines []string
	line := ""
	for _, w := range ws {
		switch {
		case line == "":
			line = w
		case len(line)+1+len(w) <= 72:
			line += " " + w
		default:
			lines = append(lines, line)
			line = w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\r\n")
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mailgen:", err)
		os.Exit(1)
	}
}

// run parses flags into Options and calls Generate.
func run(args []string) error {
	fs := flag.NewFlagSet("mailgen", flag.ContinueOnError)
	n := fs.Int("n", 0, "number of messages (at least 1)")
	seed := fs.Uint64("seed", 1, "random seed")
	out := fs.String("out", "", "Maildir root to create")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *n < 1 {
		return errors.New("-n must be at least 1")
	}
	if *out == "" {
		return errors.New("-out is required")
	}
	return Generate(Options{N: *n, Seed: *seed, Out: *out})
}
