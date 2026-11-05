// Command mailctl is the command-line client for maild: it speaks the same
// RPC API as the app (docs/specs/rpc-api.md).
package main

import (
	"os"

	"github.com/frostyard/clix"
	"github.com/frostyard/std/problems"

	"github.com/frostyard/frostmail/internal/problemrec"
)

// Set via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "local"
)

func main() {
	// A panic of the main goroutine becomes a problem record (ADR-0049).
	rec := problemrec.New(version, problems.ComponentCLI)
	defer rec.RecoverPanic()
	app := clix.App{Version: version, Commit: commit, Date: date, BuiltBy: builtBy}
	if err := app.Run(newRootCmd()); err != nil {
		os.Exit(1)
	}
}
