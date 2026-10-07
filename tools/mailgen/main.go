// Command mailgen writes a reproducible Maildir of N synthetic messages for
// load tests (make mailtest-seed). Task T-0021 implements it.
//
//	go run ./tools/mailgen -n 50000 -seed 1 -out DIR
package main

import (
	"fmt"
	"os"
)

// Options say what to generate.
type Options struct {
	N    int    // number of messages
	Seed uint64 // the same seed always produces the same bytes
	Out  string // Maildir root; cur/, new/ and tmp/ are created
}

// Generate writes the Maildir. Task T-0021 implements it; the stub writes
// nothing.
func Generate(o Options) error {
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mailgen:", err)
		os.Exit(1)
	}
}

// run parses flags into Options and calls Generate. Task T-0021 implements
// it; the stub ignores args.
func run(args []string) error {
	return Generate(Options{})
}
