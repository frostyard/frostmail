//go:build automation

package main

// Built with -tags automation, the app lets mygo-automation list, press,
// scroll and capture its windows, for end-to-end checks (plan 0015). It
// listens only when MYGO_AUTOMATION names a socket.

import (
	"fmt"
	"os"

	_ "github.com/egoist/mygo/plugins/automation"
)

func init() {
	file := os.Getenv("FROSTMAIL_TEST_LINK_LOG")
	if file == "" {
		return
	}
	// A check clicks links without opening the desktop's browser.
	testLaunch = func(url string) error {
		f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = fmt.Fprintln(f, url)
		return err
	}
}
