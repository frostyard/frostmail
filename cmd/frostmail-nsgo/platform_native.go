//go:build !js

package main

import (
	"context"
	"fmt"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

func (w *workspace) savePart(id int64, p api.Part) {
	target := w.session.Target()
	perform(w, func(ctx context.Context, c *api.Client) (*core.SaveOptions, error) {
		f, err := c.Message().Part(ctx, &api.MessagePartParams{ID: id, Path: p.Path})
		if err != nil {
			return nil, err
		}
		paths, err := config.Resolve(os.Getenv)
		if err != nil {
			return nil, err
		}
		data, err := conn.ReadPart(ctx, target.Lodge, paths.CacheDir, f, conn.MaxDownload)
		if err != nil {
			return nil, err
		}
		return &core.SaveOptions{Name: conn.SafeName(f.Filename), MIME: f.ContentType, Data: data}, nil
	}, func(o *core.SaveOptions, err error) {
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.cx.SaveFile(*o, func(_ bool, _ string, err error) {
			if err != nil {
				w.problem.Set(err.Error())
			}
		})
	})
}
func (w *workspace) saveMessage(m api.MessageSummary) {
	lodge := w.session.Target().Lodge
	perform(w, func(ctx context.Context, c *api.Client) (string, error) {
		dir, err := os.MkdirTemp("", "fm-export-")
		if err != nil {
			return "", err
		}
		path := filepath.Join(dir, "message.eml")
		if lodge != nil {
			remote := true
			var s *api.MessageSource
			s, err = c.Message().Source(ctx, &api.MessageSourceParams{ID: m.ID, Content: &remote})
			if err == nil {
				if s.ContentPath == nil {
					err = fmt.Errorf("Lodge returned no download path")
				} else {
					err = lodge.SaveTo(ctx, *s.ContentPath, path)
				}
			}
		} else {
			err = c.Message().Save(ctx, &api.MessageSaveParams{ID: m.ID, Path: path})
		}
		if err != nil {
			_ = os.RemoveAll(dir)
			return "", err
		}
		return path, nil
	}, func(path string, err error) {
		if err != nil {
			if path != "" {
				_ = os.RemoveAll(filepath.Dir(path))
			}
			w.problem.Set(err.Error())
			return
		}
		cleanup := func() { _ = os.RemoveAll(filepath.Dir(path)) }
		w.cx.SaveFile(core.SaveOptions{Name: mail.SaveName(m.Subject), Path: path, MIME: "message/rfc822"}, func(_ bool, _ string, err error) {
			cleanup()
			if err != nil {
				w.problem.Set(err.Error())
			}
		})
	})
}

// Only the fixed print template uses the host's print command. Every
// mail-derived value is escaped text. The HTML mail reader keeps scripts off.
func (w *workspace) printMessage(p *messagePage) {
	esc := html.EscapeString
	src := `<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><style>body{margin:2em;font:15px system-ui;color:#161616;background:white}h1{font-size:1.4em}pre{font:inherit;white-space:pre-wrap;overflow-wrap:anywhere}header{border-bottom:1px solid #aaa}</style><header><h1>` + esc(p.message.Summary.Subject) + `</h1><pre>` + esc(strings.Join(printHeaders(p.message), "\n")) + `</pre></header><pre>` + esc(p.rendering.Text) + `</pre>`
	w.show(func(cx *core.Scope) core.Node {
		ref := &core.Ref{}
		ready := core.NewSignal(cx, false)
		return w.dialog("Print message", []core.Node{core.WebView{Ref: ref, Label: "Printable message", Source: core.WebHTML(src, ""), Incognito: true, Style: core.Style{Height: core.Pt(380)}, OnLoad: func(core.WebNavigation) { ready.Set(true) }}}, button("Close", w.closePanel), ui.Button{Text: "Print or save PDF", Disabled: func() bool { return !ready.Get() }, OnPress: func() {
			ref.Evaluate("window.print()", func(_ string, err error) {
				if err != nil {
					w.problem.Set(err.Error())
				}
			})
		}})
	})
}
