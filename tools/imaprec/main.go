// Command imaprec turns a session trace from maild (MAILD_IMAP_TRACE) into
// a replay script that can be checked in (docs/design/testing.md,
// transcript replay). It cuts the session after one command, can keep only
// the newest messages each mailbox fetched, and replaces what the
// account's mail says (names, addresses, subjects, bodies, mailbox names)
// with stable fakes while keeping the protocol's shape. It refuses to
// write a script in which a word it replaced still appears.
//
//	go run ./tools/imaprec -through T14 -keep 5 -note "Gmail, first sync" -o script.txt TRACE
package main

import (
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/frostyard/frostmail/internal/imapx/replay"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "imaprec:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("imaprec", flag.ContinueOnError)
	through := fs.String("through", "", "keep the session up to the command `tag`ged so (default: every complete exchange)")
	keep := fs.Int("keep", 0, "keep only the newest `n` messages each mailbox fetched (0 keeps all)")
	note := fs.String("note", "", "a comment for the top of the script")
	out := fs.String("o", "", "write the script to `file` (default: standard output)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: imaprec [-through TAG] [-keep N] [-note TEXT] [-o FILE] TRACE")
	}
	trace, err := replay.Load(fs.Arg(0))
	if err != nil {
		return err
	}
	// A new key each run: fakes cannot be matched to words by anyone who
	// guesses them, and scripts from separate runs share nothing.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	script, err := convert(trace, options{through: *through, keep: *keep, key: key, note: *note})
	if err != nil {
		return err
	}
	if *out == "" {
		_, err = stdout.Write(script)
		return err
	}
	return os.WriteFile(*out, script, 0o644)
}
