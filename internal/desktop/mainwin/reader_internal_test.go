package mainwin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
	"github.com/frostyard/frostui/frostuitest"
)

// heldFrame is a page whose Find waits on the channel of its text.
type heldFrame struct {
	uikit.Frame
	hold      map[string]chan struct{}
	selection string
	doc       string

	mu    sync.Mutex
	finds int
}

func (f *heldFrame) Find(text string, _, _ bool) (int, int) {
	if ch := f.hold[text]; ch != nil {
		<-ch
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finds++
	return 1, 1
}
func (f *heldFrame) StopFind()         {}
func (f *heldFrame) Selection() string { return f.selection }
func (f *heldFrame) Load(doc string)   { f.doc = doc }

func convOf(frames map[int64]uikit.Frame, ids ...int64) *conv {
	cv := &conv{msgs: map[int64]*openMsg{}}
	for _, id := range ids {
		cv.rows = append(cv.rows, api.MessageSummary{ID: id})
		cv.msgs[id] = &openMsg{frame: frames[id]}
	}
	return cv
}

// M51: a Find still running when the bar closes must not land on the next
// one, whose pages can be fewer: stepFind indexes pages by its counts.
func TestFindFromBeforeACloseIsDropped(t *testing.T) {
	posted := make(chan func(), 2)
	w := &Window{o: Options{Post: func(fn func()) { posted <- fn }}}
	older, newer := make(chan struct{}), make(chan struct{})
	hold := map[string]chan struct{}{"old": older, "new": newer}
	cv := convOf(map[int64]uikit.Frame{1: &heldFrame{hold: hold}, 2: &heldFrame{hold: hold}}, 1, 2)
	w.conv = cv

	w.find.open = true
	w.runFind("old")
	w.closeFind()
	delete(cv.msgs, 2)
	w.find.open = true
	w.runFind("new")

	close(newer)
	(<-posted)()
	close(older)
	(<-posted)()

	if len(w.find.pages) != 1 || len(w.find.counts) != 1 {
		t.Errorf("%d counts for %d pages: the closed find's landed", len(w.find.counts), len(w.find.pages))
	}
}

// M51: counts of other pages, as stand until a find's arrive, move nothing.
func TestStepFindRefusesCountsOfOtherPages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		page := &heldFrame{}
		w := &Window{}
		w.find = findState{open: true, query: "x", counts: []int{2, 1}, pages: []pageOf{{id: 1, frame: page}}}
		w.stepFind(1)
		synctest.Wait()
		if page.finds != 0 {
			t.Errorf("%d finds on a page the counts are not of", page.finds)
		}
	})
}

// M48: a message collapsed while its remote content loads gets no page,
// which nothing would close.
func TestRemoteRenderMakesNoPageForACollapsedMessage(t *testing.T) {
	made := 0
	w := &Window{o: Options{NewFrame: func() (uikit.Frame, error) {
		made++
		return &heldFrame{}, nil
	}}}
	cv := &conv{msgs: map[int64]*openMsg{}}
	w.conv = cv
	m := &openMsg{summary: api.MessageSummary{ID: 1}, loadingRemote: true}

	w.remoteRendered(cv, m, &api.Rendering{Text: "late"}, nil)
	if made != 0 || m.frame != nil {
		t.Errorf("%d pages made for a collapsed message", made)
	}
	if m.loadingRemote {
		t.Error("still loading")
	}

	cv.msgs[1] = m
	w.remoteRendered(cv, m, &api.Rendering{Text: "shown"}, nil)
	if made != 1 || m.frame == nil {
		t.Errorf("%d pages made for a message that shows", made)
	}
}

func printedDocument(t *testing.T, check *api.SenderCheck, r *api.Rendering) string {
	t.Helper()
	frame := &heldFrame{}
	w := &Window{o: Options{Now: time.Now, NewFrame: func() (uikit.Frame, error) { return frame, nil }}}
	w.showRendering(&openMsg{message: &api.Message{
		Summary:     api.MessageSummary{From: api.Address{Address: "ann@x<b>.test"}, Subject: "Hello"},
		SenderCheck: check,
	}}, r)
	return frame.doc
}

// M47: the sender check's warning prints with the message, escaped, and
// only for a failure (docs/specs/ui.md).
func TestPrintHeaderCarriesTheSenderWarning(t *testing.T) {
	fail, pass := api.SenderCheckFail, api.SenderCheckPass
	for _, tc := range []struct {
		name  string
		check *api.SenderCheck
		warns bool
	}{{"fail", &fail, true}, {"pass", &pass, false}, {"absent", nil, false}} {
		t.Run(tc.name, func(t *testing.T) {
			doc := printedDocument(t, tc.check, &api.Rendering{Text: "body"})
			_, header, ok := strings.Cut(doc, `<div class="print-only">`)
			if !ok {
				t.Fatalf("no print header in %s", doc)
			}
			header, _, _ = strings.Cut(header, `</div></div>`)
			if got := strings.Contains(header, "Sender not verified."); got != tc.warns {
				t.Errorf("warning %v, want %v: %s", got, tc.warns, header)
			}
			if strings.Contains(header, "<b>.test") {
				t.Errorf("the sender's domain is not escaped: %s", header)
			}
			if tc.warns && !strings.Contains(header, "x&lt;b&gt;.test") {
				t.Errorf("no escaped domain: %s", header)
			}
		})
	}
}

// M46: the guard reaches the header ahead of anything the message's CSS
// says: its layer is declared before the message, and the header is the
// body's first child, which is what the guard selects.
func TestPrintGuardComesBeforeTheMessage(t *testing.T) {
	const hostile = `<style>@layer first{}.print-only{display:none!important}body::before{content:"From: Bank"}</style><div class="print-only">From: Bank</div>`
	for name, r := range map[string]*api.Rendering{"html": {HTML: hostile}, "text": {Text: "body"}} {
		t.Run(name, func(t *testing.T) {
			doc := printedDocument(t, nil, r)
			guard := strings.Index(doc, "@layer print-guard{@media print{")
			body := strings.Index(doc, "<body>")
			if guard < 0 || guard > body {
				t.Fatalf("guard at %d, body at %d: %s", guard, body, doc)
			}
			if strings.Contains(doc[:guard], "@layer") {
				t.Errorf("a layer comes before the guard: %s", doc)
			}
			if !strings.Contains(doc, `<body><div class="print-only"><div><b>From:`) {
				t.Errorf("the header is not the body's first child: %s", doc)
			}
		})
	}
}

// M52: a page keeps its selection when the focus leaves it, so the page
// focused comes first, then the reader's order.
func TestSelectionForReplyPrefersThePageFocused(t *testing.T) {
	pages := []pageOf{{id: 3, frame: &heldFrame{selection: "stale"}}, {id: 2, frame: &heldFrame{}}, {id: 1, frame: &heldFrame{selection: "latest"}}}
	for _, tc := range []struct {
		name    string
		focused int64
		id      int64
		text    string
	}{
		{"the page focused", 1, 1, "latest"},
		{"a focused page with no selection", 2, 3, "stale"},
		{"no page focused", 0, 3, "stale"},
		{"a page gone", 9, 3, "stale"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if id, text := selectionForReply(pages, tc.focused); id != tc.id || text != tc.text {
				t.Errorf("got %d %q, want %d %q", id, text, tc.id, tc.text)
			}
		})
	}
}

// M53: methods go on after a failure, but not after a cancel: the client
// sends a call even on a cancelled context.
func TestTryUnsubscribeStopsOnACancel(t *testing.T) {
	methods := []api.UnsubscribeMethod{api.UnsubscribeMethodOneclick, api.UnsubscribeMethodMail}
	failure := errors.New("the host said no")

	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var tried []api.UnsubscribeMethod
		_, err := tryUnsubscribe(ctx, methods, func(ctx context.Context, m api.UnsubscribeMethod) (*api.UnsubscribeResult, error) {
			tried = append(tried, m)
			cancel()
			return nil, ctx.Err()
		})
		if !errors.Is(err, context.Canceled) || len(tried) != 1 {
			t.Errorf("err %v after trying %v", err, tried)
		}
	})
	t.Run("already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := tryUnsubscribe(ctx, methods, func(context.Context, api.UnsubscribeMethod) (*api.UnsubscribeResult, error) {
			t.Error("a call on a cancelled context")
			return nil, failure
		}); !errors.Is(err, context.Canceled) {
			t.Errorf("err %v", err)
		}
	})
	t.Run("failure goes on", func(t *testing.T) {
		var tried []api.UnsubscribeMethod
		res, err := tryUnsubscribe(t.Context(), methods, func(_ context.Context, m api.UnsubscribeMethod) (*api.UnsubscribeResult, error) {
			tried = append(tried, m)
			if m == api.UnsubscribeMethodOneclick {
				return nil, failure
			}
			return &api.UnsubscribeResult{}, nil
		})
		if err != nil || res == nil || len(tried) != 2 {
			t.Errorf("res %v err %v after trying %v", res, err, tried)
		}
	})
	t.Run("all fail", func(t *testing.T) {
		if _, err := tryUnsubscribe(t.Context(), methods, func(context.Context, api.UnsubscribeMethod) (*api.UnsubscribeResult, error) {
			return nil, failure
		}); !errors.Is(err, failure) {
			t.Errorf("err %v", err)
		}
	})
}

// M50: two downloads never share a path, and old ones go.
func TestOperationDirIsPerOperation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "attachments")
	now := time.Now()
	first, err := operationDir(root, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := operationDir(root, now)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("both downloads in %s", first)
	}
	for _, dir := range []string{first, second} {
		if rel, err := filepath.Rel(root, dir); err != nil || !filepath.IsLocal(rel) {
			t.Errorf("%s is outside %s, where the opener looks", dir, root)
		}
	}

	other := filepath.Join(root, "keep")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-2 * operationKeep)
	for _, dir := range []string{first, other} {
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := operationDir(root, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the old download stayed: %v", err)
	}
	for _, dir := range []string{second, other} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("%s went: %v", dir, err)
		}
	}
}

type commandFrame struct {
	heldFrame
	prints int
}

func (f *commandFrame) Print() { f.prints++ }
func (f *commandFrame) Close() {}

func TestCollapsedReaderCommandsRunOnce(t *testing.T) {
	for _, command := range []string{"headers", "source", "print"} {
		t.Run(command, func(t *testing.T) {
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			row := api.MessageSummary{ID: 1, Subject: "Collapsed"}
			s := &fakeMaild{result: map[string]any{
				"message.get": api.Message{Summary: row}, "message.render": api.Rendering{Text: "Body"},
				"message.source": api.MessageSource{Text: "Raw body", Headers: "X-Test: header"},
			}, hold: map[string]chan struct{}{"message.get": release}}
			loop := frostuitest.NewLoop()
			frame := &commandFrame{}
			w := New(Options{Target: conn.Target{Socket: "none"}, Post: loop.Post, NewFrame: func() (uikit.Frame, error) { return frame, nil }})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			cv := &conv{id: 1, client: s.client(t), ctx: ctx, cancel: cancel, rows: []api.MessageSummary{row, {ID: 2}}, expanded: map[int64]bool{}, msgs: map[int64]*openMsg{}}
			w.conv = cv
			switch command {
			case "headers":
				w.allHeadersCommand()
			case "source":
				w.rawSourceCommand()
			case "print":
				w.printMessage()
				if frame.prints != 0 || !cv.msgs[1].printOnLoad {
					t.Fatal("print was not queued for the collapsed message")
				}
			}
			unblock()
			poll(t, "the command's result", func() bool {
				loop.Flush()
				switch command {
				case "headers":
					return cv.msgs[1].headers != nil && *cv.msgs[1].headers == "X-Test: header"
				case "source":
					return w.rawSource != nil && w.rawSource.source.Text == "Raw body"
				default:
					return frame.prints == 1
				}
			})
			if !cv.isOpen(1) {
				t.Error("the command did not expand the message")
			}
			if command != "print" && s.count("message.source") != 1 {
				t.Error("the command read the source more than once")
			}
		})
	}
}
