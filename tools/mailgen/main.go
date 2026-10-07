// Command mailgen writes a reproducible Maildir of N synthetic messages for
// load tests (make mailtest-seed); the generator is internal/mailgen.
//
//	go run ./tools/mailgen -n 50000 -seed 1 -out DIR
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/frostyard/frostmail/internal/mailgen"
)

// Options say what to generate.
type Options = mailgen.Options

// Generate writes the Maildir (mailgen.Generate).
func Generate(o Options) error { return mailgen.Generate(o) }

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
