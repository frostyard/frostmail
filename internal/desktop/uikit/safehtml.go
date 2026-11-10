package uikit

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// PageCSP is the policy of every page of HTML the app did not write: only
// data: images, fonts and media, inline style, no script, no form, and no
// base to move links with (docs/plans/0015-mygo-desktop.md, Phase 0's
// records).
const PageCSP = "default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src data:; media-src data:; base-uri 'none'; form-action 'none'"

// PageRules are the content rules of such a page: everything is blocked
// but data: and about:, whatever the CSP says.
const PageRules = `[
 {"trigger": {"url-filter": ".*"}, "action": {"type": "block"}},
 {"trigger": {"url-filter": "^data:"}, "action": {"type": "ignore-previous-rules"}},
 {"trigger": {"url-filter": "^about:"}, "action": {"type": "ignore-previous-rules"}}
]`

const pageStyle = `html{color-scheme:light}` +
	`body{margin:0;padding:20px;background:#fff;color:#000;font:14px/1.45 "Inter Variable",system-ui,sans-serif;overflow-wrap:anywhere}` +
	`img{max-width:100%;height:auto}` +
	`@media screen{.print-only{display:none}}` +
	`.print-only{font-size:12px;margin:0 0 16px;padding:0 0 8px;border-bottom:1px solid #ccc}` +
	`.print-only div{margin:2px 0}`

// Document wraps sanitized HTML in the page. printHeader is HTML, already
// escaped, that shows only on paper: what the app draws natively above the
// page on screen.
func Document(body, printHeader, extraStyle string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="`)
	b.WriteString(PageCSP)
	b.WriteString(`"><meta charset="utf-8"><meta name="color-scheme" content="light"><base target="_blank"><style>`)
	b.WriteString(pageStyle)
	b.WriteString(extraStyle)
	b.WriteString(`</style></head><body>`)
	if printHeader != "" {
		b.WriteString(`<div class="print-only">`)
		b.WriteString(printHeader)
		b.WriteString(`</div>`)
	}
	b.WriteString(body)
	b.WriteString(`</body></html>`)
	return b.String()
}

// Frame is one page of HTML the app did not write, as a window shows it.
// Its methods belong to the UI thread, except those that say they wait.
type Frame interface {
	// Load shows a document made by Document.
	Load(document string)
	// View draws the page; give it a height.
	View(c *ui.Context) ui.Element
	// Height is the page's content height, 0 until it has loaded.
	Height() float32
	// Selection returns the text selected in the page. It waits for the
	// page: call it from a worker.
	Selection() string
	// Find highlights the matches of text and goes to one: the first
	// without next, the next or the previous with it. It reports which
	// is current, from 1, and how many there are. It waits for the page:
	// call it from a worker.
	Find(text string, next, back bool) (active, matches int)
	// StopFind removes the highlights.
	StopFind()
	// Print opens the print dialog on the page.
	Print()
	// Close destroys the page.
	Close()
}

// SafeHTMLView makes the Frames of one window: untrusted pages in one
// ephemeral session, under PageRules, whose links go to Open and which
// never navigate or open a window.
type SafeHTMLView struct {
	win     *mygo.Window
	open    func(url string)
	notify  func()
	session *mygo.Session

	ready chan struct{}
	rules *mygo.ContentRules
	err   error
}

// NewSafeHTMLView starts compiling the rules for pages of win; open gets
// each link clicked, on a worker, and notify runs when a page's height
// changes. Call it after the app is ready.
func NewSafeHTMLView(win *mygo.Window, open func(url string), notify func()) *SafeHTMLView {
	v := &SafeHTMLView{win: win, open: open, notify: notify, session: mygo.NewEphemeralSession(), ready: make(chan struct{})}
	go func() {
		defer close(v.ready)
		v.rules, v.err = mygo.CompileContentRules("frostmail-reader", PageRules)
		if v.err != nil {
			slog.Error("compiling the reader's content rules", "err", v.err)
		}
	}()
	return v
}

// New makes a page. It shows nothing until Load.
func (v *SafeHTMLView) New() (Frame, error) {
	view, err := v.win.NewWebView(mygo.WebViewOptions{ScrollToContainer: true,
		Page: mygo.PageOptions{Untrusted: true, Session: v.session}})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &safeFrame{owner: v, view: view, ctx: ctx, cancel: cancel}
	p := view.Page()
	p.OnWillNavigate(func(e *mygo.NavigateEvent) { e.PreventDefault() })
	p.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		go v.open(req.URL)
		return nil
	})
	p.OnDidFinishLoad(func() { go f.measureLoop() })
	return f, nil
}

// safeFrame is a Frame over a web view.
type safeFrame struct {
	owner  *SafeHTMLView
	view   *mygo.WebView
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	height  float32
	polling bool
}

// Load waits for the rules, so that no page shows without them: a page
// whose rules failed to compile stays blank.
func (f *safeFrame) Load(document string) {
	go func() {
		select {
		case <-f.ctx.Done():
			return
		case <-f.owner.ready:
		}
		if f.owner.err != nil {
			return
		}
		p := f.view.Page()
		p.SetContentRules(f.owner.rules)
		p.LoadHTML(document, "")
	}()
}

func (f *safeFrame) View(c *ui.Context) ui.Element { return ui.WebView(c, f.view) }

func (f *safeFrame) Height() float32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.height
}

// measureInterval is how often a page shown is measured: a click on See
// More, or a new width, changes its height and no script in it can say so.
const measureInterval = 400 * time.Millisecond

const measureScript = `Math.ceil(Math.max(document.body.scrollHeight,document.body.getBoundingClientRect().height))`

func (f *safeFrame) measureLoop() {
	f.mu.Lock()
	if f.polling {
		f.mu.Unlock()
		return
	}
	f.polling = true
	f.mu.Unlock()
	for {
		f.measure()
		select {
		case <-f.ctx.Done():
			return
		case <-time.After(measureInterval):
		}
	}
}

func (f *safeFrame) measure() {
	h, err := mygo.EvalAs[float64](f.view.Page(), measureScript)
	if err != nil {
		return
	}
	f.mu.Lock()
	changed := f.height != float32(h)
	f.height = float32(h)
	f.mu.Unlock()
	if changed {
		f.owner.notify()
	}
}

func (f *safeFrame) Selection() string {
	s, err := mygo.EvalAs[string](f.view.Page(), `String(window.getSelection())`)
	if err != nil {
		return ""
	}
	return s
}

func (f *safeFrame) Find(text string, next, back bool) (active, matches int) {
	res, err := f.view.Page().FindInPage(text, mygo.FindOptions{FindNext: next, Backward: back})
	if err != nil {
		return 0, 0
	}
	return res.Active, res.Matches
}

func (f *safeFrame) StopFind() { f.view.Page().StopFindInPage() }

func (f *safeFrame) Print() { f.view.Page().Print() }

func (f *safeFrame) Close() {
	f.cancel()
	f.view.Destroy()
}
