//go:build automation

package main

// Built with -tags automation, the app lets mygo-automation list, press,
// scroll and capture its windows, for end-to-end checks (plan 0015). It
// listens only when MYGO_AUTOMATION names a socket.

import _ "github.com/egoist/mygo/plugins/automation"
