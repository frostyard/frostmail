//go:build !js

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/opener"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func platformTarget() (conn.Target, error) { return conn.TargetFrom(os.Getenv) }
func bodyView(w *workspace, r *api.Rendering) core.Node {
	body, style := r.HTML, ""
	if body == "" {
		body, style = mail.TextHTML(r.Text), mail.TextStyle
	}
	return core.Dynamic(func(*core.Scope) core.Node {
		doc := mail.ScrollingDocument(body, style+mail.ZoomStyle(w.textScale.Get()))
		return messageWebView(doc, func(link string) { w.cx.OpenURL(link, nil) })
	})
}

// messageWebView is the web view of a message's page: no script, no stored
// state, the page's own policy (mail.PageCSP), and a link that opens a
// window goes to the system browser only on the user's press and after
// the opener's check (TestMessagePageRunsNoScript).
func messageWebView(doc string, open func(link string)) core.WebView {
	return core.WebView{Label: "Message body", Source: core.WebHTML(doc, ""), Style: ui.FillRest, NoJavaScript: true, Incognito: true, OnOpenWindow: func(req core.WebWindowRequest) {
		if !req.UserInitiated {
			return
		}
		link, err := (opener.Opener{}).CheckURL(req.URL)
		if err == nil {
			open(link)
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
	const printScript = "document.getElementById('print').addEventListener('click',()=>window.print());"
	sum := sha256.Sum256([]byte(printScript))
	permit := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	src := `<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src ` + permit + `; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><style>body{margin:2em;font:15px system-ui;color:#161616;background:white}h1{font-size:1.4em}pre{font:inherit;white-space:pre-wrap;overflow-wrap:anywhere}header{border-bottom:1px solid #aaa}button{padding:6px 12px}@media print{button{display:none}}</style><button id="print">Print or save PDF</button><header><h1>` + esc(p.message.Summary.Subject) + `</h1><pre>` + esc(strings.Join(printHeaders(p.message), "\n")) + `</pre></header><pre>` + esc(p.rendering.Text) + `</pre><script>` + printScript + `</script>`
	w.show(func(*core.Scope) core.Node {
		return w.dialog("Print message", []core.Node{core.WebView{Label: "Printable message", Source: core.WebHTML(src, ""), Incognito: true, Style: core.Style{Height: core.Pt(380)}}})
	})
}

func (w *workspace) partThumbnail(cx *core.Scope, id int64, p api.Part) core.Node {
	data := core.NewSignalFunc(cx, []byte(nil), func(_, _ []byte) bool { return false })
	lodge := w.session.Target().Lodge
	perform(w, func(ctx context.Context, c *api.Client) ([]byte, error) {
		f, err := c.Message().Part(ctx, &api.MessagePartParams{ID: id, Path: p.Path})
		if err != nil {
			return nil, err
		}
		paths, err := config.Resolve(os.Getenv)
		if err != nil {
			return nil, err
		}
		raw, err := conn.ReadPart(ctx, lodge, paths.CacheDir, f, mail.MaxPreviewBytes)
		if err != nil {
			return nil, err
		}
		pic, err := mail.Thumbnail(raw, 320, 185)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		err = png.Encode(&buf, pic)
		return buf.Bytes(), err
	}, func(b []byte, err error) {
		if err == nil {
			cx.Post(func() { data.Set(b) })
		}
	})
	return core.Dynamic(func(*core.Scope) core.Node {
		if b := data.Get(); len(b) > 0 {
			return core.Image{Label: p.Filename, Source: core.ImageSource{Data: b}, Style: core.Style{Width: core.Pt(220), Height: core.Pt(130)}}
		}
		return nil
	})
}
func (w *workspace) openAttachment(id int64, p api.Part) {
	lodge := w.session.Target().Lodge
	perform(w, func(ctx context.Context, c *api.Client) (openedPart, error) {
		f, err := c.Message().Part(ctx, &api.MessagePartParams{ID: id, Path: p.Path})
		if err != nil {
			return openedPart{}, err
		}
		paths, err := config.Resolve(os.Getenv)
		if err != nil {
			return openedPart{}, err
		}
		if lodge == nil {
			root := filepath.Join(paths.CacheDir, "parts")
			path, err := mail.AttachmentOpener(root).CheckFile(filepath.Join(root, filepath.FromSlash(f.Path)))
			return openedPart{path: path}, err
		}
		if f.ContentPath == nil {
			return openedPart{}, fmt.Errorf("Lodge returned no attachment path")
		}
		dir, err := os.MkdirTemp("", "frostmail-attachment-")
		if err != nil {
			return openedPart{}, err
		}
		path, err := lodge.Download(ctx, dir, *f.ContentPath, f.Filename)
		if err == nil {
			path, err = mail.AttachmentOpener(dir).CheckFile(path)
		}
		if err != nil {
			_ = os.RemoveAll(dir)
			return openedPart{}, err
		}
		return openedPart{path: path, dir: dir}, nil
	}, func(part openedPart, err error) {
		if part.dir != "" {
			w.cx.Cleanup(func() { _ = os.RemoveAll(part.dir) })
		}
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.cx.OpenFile(core.SharedFile{Path: part.path}, func(opened bool, err error) {
			switch {
			case err != nil:
				w.problem.Set(err.Error())
			case !opened:
				w.problem.Set("No app on this computer opens " + filepath.Base(part.path) + ".")
			}
		})
	})
}

// openedPart is an attachment that passed the opener's checks: its path,
// and the temporary folder that holds a download from lodge, which the
// workspace removes when it ends.
type openedPart struct{ path, dir string }

func calendarZone() *time.Location { return calendar.LocalZone() }
