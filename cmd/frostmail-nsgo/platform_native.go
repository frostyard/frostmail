//go:build !js

package main

import (
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"net/url"
	"os"
)

func platformTarget() (conn.Target, error) { return conn.TargetFrom(os.Getenv) }
func bodyView(w *workspace, r *api.Rendering) core.Node {
	if r.HTML == "" {
		return plainBody(r.Text)
	}
	doc := uikit.ScrollingDocument(r.HTML)
	return core.WebView{Label: "Message body", Source: core.WebHTML(doc, ""), Style: ui.FillRest, NoJavaScript: true, Incognito: true, OnOpenWindow: func(req core.WebWindowRequest) {
		u, err := url.Parse(req.URL)
		if err == nil && req.UserInitiated && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			w.cx.OpenURL(req.URL, nil)
		}
	}}
}
