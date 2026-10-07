// Command uifixture builds a maild data directory for the app's tests: one
// account whose INBOX holds N mailgen messages with their bodies stored, and
// a Hostile mailbox holding the hostile-HTML corpus (docs/plans/0004, Phase 4).
// maild started on it needs no mail server: bodies are local and the account
// has no password, so sync never connects.
//
//	go run ./tools/uifixture -out DIR -n 100000 -hostile internal/render/testdata/hostile
//
// Task T-0036 implements Build and run; the stubs do nothing.
package main

import (
	"context"
	"fmt"
	"os"
)

// Options say what to build.
type Options struct {
	Out     string // the data directory to create (FROSTMAIL_DATA_DIR)
	N       int    // mailgen messages in INBOX
	Seed    uint64 // mailgen seed
	Hostile string // a directory of .html files for the Hostile mailbox; "" for none
}

// Build creates the data directory.
func Build(_ context.Context, _ Options) error {
	return nil
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "uifixture:", err)
		os.Exit(1)
	}
}

// run parses flags into Options and calls Build.
func run(_ context.Context, _ []string) error {
	return nil
}
