//go:build js && wasm

package main

import (
	"context"
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

func (w *workspace) downloadContent(path *string, err error) {
	if err != nil {
		w.problem.Set(err.Error())
		return
	}
	if path == nil {
		w.problem.Set("Lodge returned no download path.")
		return
	}
	js.Global().Get("frostmailLodge").Call("download", *path)
}
func (w *workspace) savePart(id int64, p api.Part) {
	perform(w, func(ctx context.Context, c *api.Client) (*string, error) {
		f, err := c.Message().Part(ctx, &api.MessagePartParams{ID: id, Path: p.Path})
		if err != nil {
			return nil, err
		}
		return f.ContentPath, nil
	}, w.downloadContent)
}
func (w *workspace) saveMessage(m api.MessageSummary) {
	perform(w, func(ctx context.Context, c *api.Client) (*string, error) {
		yes := true
		s, err := c.Message().Source(ctx, &api.MessageSourceParams{ID: m.ID, Content: &yes})
		if err != nil {
			return nil, err
		}
		return s.ContentPath, nil
	}, w.downloadContent)
}

const desktopServer = false

func (w *workspace) serverPanel(*core.Scope) core.Node { return nil }

func (w *workspace) platformStart() {
	host := js.Global().Get("frostmailLodge")
	if host.IsUndefined() {
		return
	}
	ready := core.NewSignal(w.cx, false)
	var last location
	var sequence int
	navigate := func(path string) {
		sequence++
		seq := sequence
		ready.Set(false)
		// Supersede any outstanding lookup even when the next route must wait.
		w.routeGeneration++
		w.whenConnected(func() {
			w.applyRoute(path, func() {
				if seq == sequence {
					last = w.location()
					host.Call("report", last.path, last.title, true)
					ready.Set(true)
				}
			})
		})
	}
	receive := js.FuncOf(func(_ js.Value, args []js.Value) any {
		path := args[0].String()
		w.cx.Post(func() { navigate(path) })
		return nil
	})
	path := host.Call("onRoute", receive).String()
	w.cx.Cleanup(func() { host.Call("onRoute"); receive.Release() })
	navigate(path)
	w.cx.Effect(func() {
		if !ready.Get() {
			return
		}
		next := w.location()
		if next.path != last.path {
			replace := next.place == last.place && !(next.depth && !last.depth)
			host.Call("report", next.path, next.title, replace)
		}
		last = next
	})
}
func (w *workspace) traySetting() core.Node { return nil }
