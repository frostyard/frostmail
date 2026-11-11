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
	open    func(url string)
	notify  func()
	newPage func() (webPage, error)

	ready chan struct{}
	rules *mygo.ContentRules
	err   error
}

// page is what a frame asks of its page, as *mygo.Page has it; the tests
// put a fake in its place.
type page interface {
	OnWillNavigate(fn func(e *mygo.NavigateEvent)) (off func())
	SetWindowOpenHandler(fn func(req mygo.WindowOpenRequest) *mygo.WindowOptions)
	OnDidFinishLoad(fn func()) (off func())
	SetContentRules(rules ...*mygo.ContentRules)
	LoadHTML(html, baseURL string)
	EvalContext(ctx context.Context, code string) (any, error)
	FindInPage(text string, opts mygo.FindOptions) (mygo.FindResult, error)
	StopFindInPage()
	Print()
}

// webPage is a page, the native view showing it, and what destroys both.
type webPage struct {
	page    page
	view    ui.NativeWebView
	destroy func()
}

// NewSafeHTMLView starts compiling the rules for pages of win, as the list
// rulesID; open gets each link clicked, on a worker, and notify runs when a
// page's height changes. Call it after the app is ready.
func NewSafeHTMLView(win *mygo.Window, rulesID string, open func(url string), notify func()) *SafeHTMLView {
	session := mygo.NewEphemeralSession()
	newPage := func() (webPage, error) {
		view, err := win.NewWebView(mygo.WebViewOptions{ScrollToContainer: true,
			Page: mygo.PageOptions{Untrusted: true, Session: session}})
		if err != nil {
			return webPage{}, err
		}
		return webPage{page: view.Page(), view: view, destroy: view.Destroy}, nil
	}
	return newSafeHTMLView(rulesID, mygo.CompileContentRules, newPage, open, notify)
}

func newSafeHTMLView(rulesID string, compile func(id, rules string) (*mygo.ContentRules, error), newPage func() (webPage, error), open func(url string), notify func()) *SafeHTMLView {
	v := &SafeHTMLView{open: open, notify: notify, newPage: newPage, ready: make(chan struct{})}
	go func() {
		defer close(v.ready)
		v.rules, v.err = compile(rulesID, PageRules)
		if v.err != nil {
			slog.Error("compiling the pages' content rules", "id", rulesID, "err", v.err)
		}
	}()
	return v
}

// New makes a page. It shows nothing until Load.
func (v *SafeHTMLView) New() (Frame, error) {
	web, err := v.newPage()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &safeFrame{owner: v, web: web, ctx: ctx, cancel: cancel}
	p := web.page
	p.OnWillNavigate(func(e *mygo.NavigateEvent) { e.PreventDefault() })
	p.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		go v.open(req.URL)
		return nil
	})
	p.OnDidFinishLoad(f.loaded)
	return f, nil
}

// safeFrame is a Frame over a web view.
type safeFrame struct {
	owner  *SafeHTMLView
	web    webPage
	ctx    context.Context
	cancel context.CancelFunc

	width [2]float32 // the window's and the element's; the UI thread's

	mu       sync.Mutex
	height   float32
	shown    bool // a document has finished loading
	polling  bool
	agreed   int // measures in a row, this poll, that gave height
	measures int // measures since the last event
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
		f.web.page.SetContentRules(f.owner.rules)
		f.web.page.LoadHTML(document, "")
	}()
}

// View draws the page. A new width or a click (on See More, say) can change
// the page's height, and no script in it can say so: each measures it again.
func (f *safeFrame) View(c *ui.Context) ui.Element {
	e := ui.WebView(c, f.web.view)
	// The window's width is this frame's, the element's the last frame's.
	window, _ := c.Size()
	width := [2]float32{window, e.Bounds().W}
	resized := width[1] > 0 && width != f.width
	if resized {
		f.width = width
	}
	if resized || e.PressedInside() {
		f.remeasure()
	}
	return e
}

func (f *safeFrame) Height() float32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.height
}

// A poll measures the page every measureInterval until measureAgree
// measures in a row agree, or measureLimit measures since the last event
// have not: then nothing measures the page until the next event.
const (
	measureInterval = 400 * time.Millisecond
	measureAgree    = 3
	measureLimit    = 25
)

const measureScript = `Math.ceil(Math.max(document.body.scrollHeight,document.body.getBoundingClientRect().height))`

// loaded runs when a document has finished loading.
func (f *safeFrame) loaded() {
	f.mu.Lock()
	f.shown = true
	f.mu.Unlock()
	f.remeasure()
}

// remeasure starts a poll of the page's height, or starts the running one
// over; a page not loaded yet waits for loaded.
func (f *safeFrame) remeasure() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.shown || f.ctx.Err() != nil {
		return
	}
	f.agreed, f.measures = 0, 0
	if !f.polling {
		f.polling = true
		go f.poll()
	}
}

func (f *safeFrame) poll() {
	for {
		changed, done := f.record(f.measure())
		if changed {
			f.owner.notify()
		}
		if done {
			return
		}
		select {
		case <-f.ctx.Done():
			return
		case <-time.After(measureInterval):
		}
	}
}

// measure asks the page for its height; false when it did not say.
func (f *safeFrame) measure() (float32, bool) {
	v, err := f.web.page.EvalContext(f.ctx, measureScript)
	if err != nil {
		return 0, false
	}
	h, ok := v.(float64)
	return float32(h), ok
}

// record takes a measure, and reports whether the height changed and
// whether the poll is over.
func (f *safeFrame) record(height float32, ok bool) (changed, done bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.measures++
	switch {
	case !ok:
		f.agreed = 0
	case f.agreed > 0 && height == f.height:
		f.agreed++
	default:
		f.agreed = 1
	}
	if ok {
		changed = height != f.height
		f.height = height
	}
	done = f.agreed >= measureAgree || f.measures >= measureLimit
	if done {
		f.polling = false
	}
	return changed, done
}

func (f *safeFrame) Selection() string {
	v, err := f.web.page.EvalContext(f.ctx, `String(window.getSelection())`)
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func (f *safeFrame) Find(text string, next, back bool) (active, matches int) {
	res, err := f.web.page.FindInPage(text, mygo.FindOptions{FindNext: next, Backward: back})
	if err != nil {
		return 0, 0
	}
	return res.Active, res.Matches
}

func (f *safeFrame) StopFind() { f.web.page.StopFindInPage() }

func (f *safeFrame) Print() { f.web.page.Print() }

func (f *safeFrame) Close() {
	f.cancel()
	f.web.destroy()
}
