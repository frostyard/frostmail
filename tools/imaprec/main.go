// Command imaprec turns a session trace from maild (MAILD_IMAP_TRACE) into
// a replay script that can be checked in (docs/design/testing.md,
// transcript replay). It cuts the session after one command, can keep only
// the newest messages each mailbox fetched, and replaces what the
// account's mail says (names, addresses, subjects, bodies, mailbox names)
// with stable fakes while keeping the protocol's shape. It refuses to
// write a script in which a word it replaced still appears.
//
//	go run ./tools/imaprec -through T14 -keep 5 -note "Gmail, first sync" -o script.txt TRACE
//
// Scripts that continue one another (a second session of the same account)
// must share their fakes: scrub them with the same -keyfile, made once with
// head -c 32 /dev/urandom and never checked in.
package main

import (
	"crypto/rand"
	"crypto/sha256"
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
	keyfile := fs.String("keyfile", "", "derive the key from this private `file`, to share fakes between scripts (default: a new key)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: imaprec [-through TAG] [-keep N] [-keyfile FILE] [-note TEXT] [-o FILE] TRACE")
	}
	trace, err := replay.Load(fs.Arg(0))
	if err != nil {
		return err
	}
	// A secret key: fakes cannot be matched to words by anyone who guesses
	// them. Without a key file, scripts from separate runs share nothing.
	key := make([]byte, 32)
	if *keyfile != "" {
		secret, err := os.ReadFile(*keyfile)
		if err != nil {
			return err
		}
		if len(secret) < 16 {
			return fmt.Errorf("%s holds %d bytes; a key needs at least 16", *keyfile, len(secret))
		}
		sum := sha256.Sum256(secret)
		key = sum[:]
	} else if _, err := rand.Read(key); err != nil {
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
