package uikit

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// fakePage stands in for a web page: it records what the frame asks of it,
// and hands back the handlers the frame set.
type fakePage struct {
	mu        sync.Mutex
	calls     []string
	rules     []*mygo.ContentRules
	documents []string
	evals     int
	heights   []float64 // the measures to give, the last one repeating
	evalErr   error
	held      string        // a document whose load waits for release
	release   chan struct{} // closed to let it go on

	willNavigate func(*mygo.NavigateEvent)
	windowOpen   func(mygo.WindowOpenRequest) *mygo.WindowOptions
	didFinish    func()
}

func (p *fakePage) OnWillNavigate(fn func(*mygo.NavigateEvent)) func() {
	p.willNavigate = fn
	return func() {}
}

func (p *fakePage) SetWindowOpenHandler(fn func(mygo.WindowOpenRequest) *mygo.WindowOptions) {
	p.windowOpen = fn
}

func (p *fakePage) OnDidFinishLoad(fn func()) func() {
	p.didFinish = fn
	return func() {}
}

func (p *fakePage) SetContentRules(rules ...*mygo.ContentRules) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "rules")
	p.rules = rules
}

func (p *fakePage) LoadHTML(html, _ string) {
	if p.release != nil && html == p.held {
		<-p.release
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "load")
	p.documents = append(p.documents, html)
}

func (p *fakePage) EvalContext(_ context.Context, _ string) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.evals++
	if p.evalErr != nil {
		return nil, p.evalErr
	}
	h := p.heights[0]
	if len(p.heights) > 1 {
		p.heights = p.heights[1:]
	}
	return h, nil
}

func (p *fakePage) FindInPage(string, mygo.FindOptions) (mygo.FindResult, error) {
	return mygo.FindResult{}, nil
}
func (p *fakePage) StopFindInPage() {}
func (p *fakePage) Print()          {}

func (p *fakePage) evaluated() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.evals
}

func (p *fakePage) called() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.calls)
}

// pageFixture is a SafeHTMLView over a fake page, in a synctest bubble.
type pageFixture struct {
	page      *fakePage
	view      *SafeHTMLView
	mu        sync.Mutex
	opened    []string
	notified  int
	destroyed bool
}

func compiled(string, string) (*mygo.ContentRules, error) { return &mygo.ContentRules{}, nil }

func newPageFixture(compile func(id, rules string) (*mygo.ContentRules, error), heights ...float64) *pageFixture {
	f := &pageFixture{page: &fakePage{heights: heights}}
	f.view = newSafeHTMLView("test-rules", compile, func() (webPage, error) {
		return webPage{page: f.page, destroy: func() { f.destroyed = true }}, nil
	}, func(url string) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.opened = append(f.opened, url)
	}, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.notified++
	})
	return f
}

func (f *pageFixture) frame(t *testing.T) Frame {
	t.Helper()
	frame, err := f.view.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(frame.Close)
	return frame
}

// idle lets everything finish, then lets the bubble's clock run an hour
// with nothing else to do: it does not wait for real time.
func idle() {
	synctest.Wait()
	time.Sleep(time.Hour)
	synctest.Wait()
}

func TestSafeFrameRefusesNavigation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(compiled, 100)
		f.frame(t)
		for _, url := range []string{"https://example.test/", "file:///etc/passwd", "about:blank"} {
			for _, user := range []bool{true, false} {
				e := &mygo.NavigateEvent{URL: url, UserInitiated: user}
				f.page.willNavigate(e)
				if !e.DefaultPrevented() {
					t.Errorf("navigation to %s (user %v) went on", url, user)
				}
			}
		}
	})
}

func TestSafeFrameSendsWindowOpenToOpen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(compiled, 100)
		f.frame(t)
		const url = "https://example.test/link"
		if got := f.page.windowOpen(mygo.WindowOpenRequest{URL: url, UserInitiated: true}); got != nil {
			t.Errorf("the handler opened a window: %+v", got)
		}
		synctest.Wait()
		if !slices.Equal(f.opened, []string{url}) {
			t.Errorf("opened %q", f.opened)
		}
	})
}

func TestSafeFrameLoadsNothingBeforeTheRules(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		rules := &mygo.ContentRules{}
		var gotID, gotRules string
		f := newPageFixture(func(id, list string) (*mygo.ContentRules, error) {
			gotID, gotRules = id, list
			<-release
			return rules, nil
		}, 100)
		frame := f.frame(t)
		frame.Load("<p>first</p>")
		synctest.Wait()
		if got := f.page.called(); len(got) != 0 {
			t.Fatalf("before the rules compiled: %q", got)
		}
		close(release)
		synctest.Wait()
		if gotID != "test-rules" || gotRules != PageRules {
			t.Errorf("compiled %q as %q", gotRules, gotID)
		}
		if got := f.page.called(); !slices.Equal(got, []string{"rules", "load"}) {
			t.Fatalf("calls = %q, want the rules, then the load", got)
		}
		if len(f.page.rules) != 1 || f.page.rules[0] != rules || f.page.documents[0] != "<p>first</p>" {
			t.Errorf("rules %v, documents %q", f.page.rules, f.page.documents)
		}
	})
}

func TestSafeFrameStaysBlankWhenTheRulesFail(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(func(string, string) (*mygo.ContentRules, error) { return nil, errors.New("no compiler") }, 100)
		frame := f.frame(t)
		frame.Load("<p>first</p>")
		synctest.Wait()
		if got := f.page.called(); len(got) != 0 {
			t.Errorf("with no rules: %q", got)
		}
	})
}

func TestSafeFrameClosedBeforeTheRulesLoadsNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		f := newPageFixture(func(string, string) (*mygo.ContentRules, error) {
			<-release
			return &mygo.ContentRules{}, nil
		}, 100)
		frame, err := f.view.New()
		if err != nil {
			t.Fatal(err)
		}
		frame.Load("<p>first</p>")
		synctest.Wait()
		frame.Close()
		synctest.Wait()
		close(release)
		synctest.Wait()
		if got := f.page.called(); len(got) != 0 {
			t.Errorf("after Close: %q", got)
		}
		if !f.destroyed {
			t.Error("Close left the page")
		}
	})
}

// A loaded page is measured until the measures agree, and then no more.
func TestSafeFrameStopsMeasuringOnceSettled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(compiled, 100, 250)
		frame := f.frame(t)
		if h := frame.Height(); h != 0 {
			t.Errorf("height before the load = %v", h)
		}
		f.page.didFinish()
		idle()
		if got := f.page.evaluated(); got != 1+measureAgree {
			t.Errorf("%d measures, want %d", got, 1+measureAgree)
		}
		if h := frame.Height(); h != 250 {
			t.Errorf("height = %v", h)
		}
		if f.notified != 2 {
			t.Errorf("notified %d times for 2 heights", f.notified)
		}
	})
}

// A page whose measures never agree, or fail, stops all the same.
func TestSafeFramePollEnds(t *testing.T) {
	endless := make([]float64, 2*measureLimit)
	for i := range endless {
		endless[i] = float64(i + 1)
	}
	for name, setup := range map[string]func(p *fakePage){
		"growing": func(p *fakePage) { p.heights = endless },
		"failing": func(p *fakePage) { p.evalErr = errors.New("gone") },
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newPageFixture(compiled, 100)
				setup(f.page)
				f.frame(t)
				f.page.didFinish()
				idle()
				if got := f.page.evaluated(); got != measureLimit {
					t.Errorf("%d measures, want %d", got, measureLimit)
				}
			})
		})
	}
}

func TestSafeFrameCloseEndsThePoll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(compiled, 100, 200, 300, 400, 500)
		frame, err := f.view.New()
		if err != nil {
			t.Fatal(err)
		}
		f.page.didFinish()
		synctest.Wait()
		frame.Close()
		idle()
		if got := f.page.evaluated(); got != 1 {
			t.Errorf("%d measures after Close", got)
		}
	})
}

// An event during a poll starts it over: the measures before it say
// nothing of the height after it.
func TestSafeFrameEventRestartsThePoll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(compiled, 100)
		f.frame(t)
		f.page.didFinish()
		synctest.Wait()
		time.Sleep(measureInterval + measureInterval/2) // between measures 2 and 3
		synctest.Wait()
		if got := f.page.evaluated(); got != 2 {
			t.Fatalf("%d measures so far", got)
		}
		f.page.didFinish()
		idle()
		if got := f.page.evaluated(); got != 2+measureAgree {
			t.Errorf("%d measures, want %d", got, 2+measureAgree)
		}
	})
}

// drawn is a frame in a window, for what its View sees; margin is room the
// window keeps beside the page, as a pane would.
func drawn(t *testing.T, f *pageFixture, margin *float32) (Frame, *ui.Tester) {
	t.Helper()
	frame := f.frame(t)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() {
			ui.Box(c).Width(*margin)
			ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
				frame.View(c).Height(200).Label("Page")
			})
		})
	}, 600, 400)
	return frame, tt
}

// A page drawn, idle, is not measured, however many frames go by.
func TestSafeFrameIdleFramesDoNotMeasure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(compiled, 100)
		margin := float32(0)
		_, tt := drawn(t, f, &margin)
		tt.Frame()
		tt.Frame()
		idle()
		if got := f.page.evaluated(); got != 0 {
			t.Fatalf("%d measures before the load", got)
		}
		f.page.didFinish()
		idle()
		settled := f.page.evaluated()
		for range 5 {
			tt.Frame()
			idle()
		}
		if got := f.page.evaluated(); got != settled {
			t.Errorf("%d measures after %d settled, with nothing changed", got, settled)
		}
	})
}

func TestSafeFrameMeasuresAfterANewWidth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(compiled, 100)
		margin := float32(0)
		frame, tt := drawn(t, f, &margin)
		tt.Frame()
		f.page.didFinish()
		idle()
		for _, step := range []struct {
			name   string
			height float64
			resize func()
		}{
			{"the window", 180, func() { tt.SetSize(420, 400) }},
			{"a pane", 260, func() { margin = 120; tt.Frame(); tt.Frame() }},
		} {
			settled := f.page.evaluated()
			f.page.mu.Lock()
			f.page.heights = []float64{step.height}
			f.page.mu.Unlock()
			step.resize()
			idle()
			if got := f.page.evaluated(); got <= settled {
				t.Fatalf("no measure after %s changed width", step.name)
			}
			if h := frame.Height(); h != float32(step.height) {
				t.Errorf("height after %s changed width = %v", step.name, h)
			}
		}
	})
}

func TestSafeFrameMeasuresAfterAClick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(compiled, 100)
		margin := float32(0)
		frame, tt := drawn(t, f, &margin)
		tt.Frame()
		f.page.didFinish()
		idle()
		settled := f.page.evaluated()
		f.page.mu.Lock()
		f.page.heights = []float64{320}
		f.page.mu.Unlock()
		page, ok := tt.Access("Page")
		if !ok {
			t.Fatal("no page in the window")
		}
		tt.ClickAt(page.Bounds.X+page.Bounds.W/2, page.Bounds.Y+page.Bounds.H/2)
		idle()
		// The click also gave the page the keyboard, which it keeps.
		if got := f.page.evaluated(); got != settled+measureAgree+watchLimit {
			t.Errorf("%d measures, want %d", got, settled+measureAgree+watchLimit)
		}
		if h := frame.Height(); h != 320 {
			t.Errorf("height after the click = %v", h)
		}
	})
}

// Of two loads waiting for the rules, only the newer shows.
func TestSafeFrameDropsASupersededLoad(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		f := newPageFixture(func(string, string) (*mygo.ContentRules, error) {
			<-release
			return &mygo.ContentRules{}, nil
		}, 100)
		frame := f.frame(t)
		frame.Load("<p>older</p>")
		frame.Load("<p>newer</p>")
		synctest.Wait()
		close(release)
		synctest.Wait()
		if !slices.Equal(f.page.documents, []string{"<p>newer</p>"}) {
			t.Errorf("documents = %q", f.page.documents)
		}
	})
}

// A load that comes while an older one is going to the page lands after it.
func TestSafeFrameLoadsOneAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(compiled, 100)
		f.page.held, f.page.release = "<p>older</p>", make(chan struct{})
		frame := f.frame(t)
		frame.Load("<p>older</p>")
		synctest.Wait()
		frame.Load("<p>newer</p>")
		synctest.Wait()
		close(f.page.release)
		synctest.Wait()
		if !slices.Equal(f.page.documents, []string{"<p>older</p>", "<p>newer</p>"}) {
			t.Errorf("documents = %q, want the newer last", f.page.documents)
		}
	})
}

// setHeight is the height the page measures from now on, as a key the page
// took for itself would change it: nothing tells the frame.
func (p *fakePage) setHeight(h float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.heights = []float64{h}
}

// See More toggled from the keyboard says nothing to the app: the page is
// measured while it has the keyboard, with no frame or event to ask for it.
func TestSafeFrameMeasuresAKeyboardChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(compiled, 100)
		margin := float32(0)
		frame, tt := drawn(t, f, &margin)
		tt.Frame()
		f.page.didFinish()
		idle()
		tt.Key(0, ui.KeyTab) // into the page
		synctest.Wait()
		time.Sleep(30 * time.Second) // reading, with the keyboard in the page
		synctest.Wait()
		f.page.setHeight(640)
		time.Sleep(2 * watchInterval)
		synctest.Wait()
		if h := frame.Height(); h != 640 {
			t.Errorf("height after the toggle = %v", h)
		}
		if f.notified != 2 {
			t.Errorf("notified %d times for 2 heights", f.notified)
		}
	})
}

// A page left with the keyboard is measured for a while, and then no more.
func TestSafeFrameWatchEnds(t *testing.T) {
	endless := make([]float64, 2*(measureLimit+watchLimit))
	for i := range endless {
		endless[i] = float64(i + 1)
	}
	for name, heights := range map[string][]float64{"settled": {100}, "growing": endless} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newPageFixture(compiled, heights...)
				margin := float32(0)
				_, tt := drawn(t, f, &margin)
				tt.Frame()
				tt.Key(0, ui.KeyTab)
				f.page.didFinish()
				idle()
				measured := f.page.evaluated()
				if measured > measureLimit+watchLimit {
					t.Errorf("%d measures, over the bound of %d", measured, measureLimit+watchLimit)
				}
				for range 3 {
					tt.Frame()
					idle()
				}
				if got := f.page.evaluated(); got != measured {
					t.Errorf("%d measures after the watch ended at %d", got, measured)
				}
			})
		})
	}
}

// The watch ends with the page's keyboard: the window losing it stops it.
func TestSafeFrameWatchEndsWithTheFocus(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newPageFixture(compiled, 100)
		margin := float32(0)
		_, tt := drawn(t, f, &margin)
		tt.Frame()
		tt.Key(0, ui.KeyTab)
		f.page.didFinish()
		synctest.Wait()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		tt.SetFocused(false)
		synctest.Wait()
		time.Sleep(watchInterval) // the measure already waiting
		synctest.Wait()
		stopped := f.page.evaluated()
		idle()
		if got := f.page.evaluated(); got != stopped {
			t.Errorf("%d measures after the focus left, at %d when it did", got, stopped)
		}
		if stopped >= watchLimit {
			t.Errorf("%d measures: the watch ran on", stopped)
		}
	})
}
