// Command davrec turns a DAV and Tasks trace from maild (MAILD_DAV_TRACE)
// into one that can be checked in (docs/design/testing.md, DAV and Tasks
// recordings): every personal value becomes a stable fake of the same
// shape, and photos a one-pixel image. It refuses to write a trace in
// which a replaced value remains. Task T-0092 builds it.
//
//	go run ./tools/davrec -account ann@gmail.com -name "Ann Lee" -note "Google, first sync" -o out.trace TRACE
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "davrec:", err)
		os.Exit(1)
	}
}

func run(args []string, _ io.Writer) error {
	if len(args) == 0 {
		return nil
	}
	return errors.New("task T-0092 builds it")
}
