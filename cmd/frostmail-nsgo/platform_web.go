//go:build js && wasm

package main

import (
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"syscall/js"
)

func platformTarget() (conn.Target, error) {
	l, err := conn.ParseLodge(js.Global().Get("location").Get("origin").String())
	return conn.Target{Lodge: &l}, err
}
func bodyView(_ *workspace, r *api.Rendering) core.Node {
	if r.ContentPath == nil {
		return plainBody(r.Text)
	}
	host := js.Global().Get("frostmailLodge")
	if host.IsUndefined() {
		return plainBody(r.Text)
	}
	// Lodge supplies a short-lived content URL on its isolated reader origin.
	// The response enforces the mail app's sandbox, CSP and referrer policy.
	return core.WebView{Label: "Message body", Source: core.WebSrc(host.Call("contentURL", *r.ContentPath).String()), NoJavaScript: true, Style: ui.FillRest}
}
