// Command davrec scrubs recorded DAV and Google Tasks sessions for replay.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json/jsontext"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/frostyard/frostmail/internal/httprec"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "davrec:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("davrec", flag.ContinueOnError)
	account := fs.String("account", "", "account email address (required)")
	name := fs.String("name", "", "account owner's name")
	keyfile := fs.String("keyfile", "", "private key file for stable fakes")
	note := fs.String("note", "", "trace comment")
	out := fs.String("o", "", "output file (default: standard output)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *account == "" || fs.NArg() != 1 {
		return errors.New("usage: davrec -account EMAIL [-name NAME] [-keyfile FILE] [-note TEXT] [-o FILE] TRACE")
	}
	key, err := readKey(*keyfile)
	if err != nil {
		return err
	}
	xs, err := httprec.Load(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("load trace: %w", err)
	}
	s := newScrubber(key)
	s.collectValue(*account)
	s.collectValue(*name)
	if err := s.prepare(xs); err != nil {
		return err
	}
	if err := s.replace(xs); err != nil {
		return err
	}
	// A replay test gives its account the address as scrubbed.
	comment := strings.TrimSpace(*note + "\naccount " + s.text(*account))
	var b bytes.Buffer
	if err := httprec.Write(&b, comment, xs); err != nil {
		return fmt.Errorf("encode trace: %w", err)
	}
	data, err := canonicalTrace(b.Bytes())
	if err != nil {
		return err
	}
	if *out != "" {
		return os.WriteFile(*out, data, 0o644)
	}
	_, err = stdout.Write(data)
	return err
}

func readKey(path string) ([]byte, error) {
	key := make([]byte, 32)
	if path == "" {
		_, err := rand.Read(key)
		return key, err
	}
	secret, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key: %w", err)
	}
	if len(secret) < 16 {
		return nil, errors.New("a key file needs at least 16 bytes")
	}
	sum := sha256.Sum256(secret)
	return sum[:], nil
}

// httprec.Write uses JSON maps for headers; canonicalize their ordering
// after encoding so the same trace and key produce identical output.
func canonicalTrace(data []byte) ([]byte, error) {
	var out bytes.Buffer
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		if line[0] != '#' {
			value := jsontext.Value(line)
			if err := value.Canonicalize(); err != nil {
				return nil, fmt.Errorf("canonicalize trace: %w", err)
			}
			line = value
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}
