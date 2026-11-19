//go:build e2e

package e2e

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// probeMark is shown once by the probe page, and once more by each script
// that runs; the find bar counts it.
const probeMark = "FMPROBEMARK"

// probeJS leaves one more mark and asks the canary if a script runs. A
// script in a frame of the page's origin marks the top page, where the find
// bar counts it; in a frame of another origin it marks its own page, and only
// its request shows it ran.
func probeJS(origin, name string) string {
	js := "var d=document;try{d=top.document}catch(e){}var p=d.createElement('p');p.textContent='FMPROBE'+'MARK';(d.body||d.documentElement).appendChild(p);new Image().src='" + origin + "/script/" + name + "';"
	if strings.ContainsAny(js, "\"<>&") {
		panic("probe script is not safe in a double-quoted HTML attribute")
	}
	return js
}

// probeVectors are the ways a message can try to run script: each runs
// probeJS, or loads it from the canary.
func probeVectors(origin string) []string {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	js := func(name string) string { return probeJS(origin, name) }
	return []string{
		`<script>` + js("script-inline") + `</script>`,
		`<script type="module">` + js("script-module") + `</script>`,
		`<script src="data:text/javascript;base64,` + b64(js("script-data-src")) + `"></script>`,
		`<script src="` + origin + `/probe.js"></script>`,
		`<img src="data:image/png;base64,AAAA" onerror="` + js("img-onerror-data") + `">`,
		`<img src="` + origin + `/img.png" onerror="` + js("img-onerror-remote") + `">`,
		`<img src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7" onload="` + js("img-onload-data") + `">`,
		`<body onload="` + js("body-onload") + `">`,
		`<svg onload="` + js("svg-onload") + `"></svg>`,
		`<svg><script>` + js("svg-script") + `</script></svg>`,
		`<svg><animate attributeName="x" dur="1s" onbegin="` + js("svg-animate-onbegin") + `"/></svg>`,
		`<details open ontoggle="` + js("details-ontoggle") + `"><summary>s</summary></details>`,
		`<style>@keyframes fmprobe{from{opacity:0}to{opacity:1}}</style><div style="animation:fmprobe 10ms" onanimationstart="` + js("css-animation") + `">a</div>`,
		`<style onload="` + js("style-onload") + `"></style>`,
		`<video src="data:video/mp4;base64,AAAA" onerror="` + js("video-onerror") + `"></video>`,
		`<iframe src="javascript:` + url.PathEscape(js("iframe-javascript")) + `"></iframe>`,
		`<iframe srcdoc="&lt;script&gt;` + js("iframe-srcdoc") + `&lt;/script&gt;"></iframe>`,
		`<iframe src="data:text/html;base64,` + b64(`<script>`+js("iframe-data")+`</script>`) + `"></iframe>`,
		`<iframe src="` + origin + `/frame.html"></iframe>`,
		`<object data="data:text/html;base64,` + b64(`<script>`+js("object-data")+`</script>`) + `"></object>`,
		`<embed src="data:image/svg+xml;base64,` + b64(`<svg xmlns="http://www.w3.org/2000/svg" onload="`+js("embed-svg")+`"/>`) + `">`,
		`<meta http-equiv="refresh" content="0;url=javascript:` + js("meta-refresh-javascript") + `">`,
	}
}

func probePage(origin string) string {
	return `<p>` + probeMark + `</p>` + strings.Join(probeVectors(origin), "\n")
}

// rawRender hands the reader the probe page as it was written, past maild's
// sanitizer.
type rawRender struct {
	api.MessageService
	raw    string
	served atomic.Int64
}

func (r *rawRender) Render(ctx context.Context, p *api.MessageRenderParams) (*api.Rendering, error) {
	rendered, err := r.MessageService.Render(ctx, p)
	if err == nil && strings.Contains(rendered.HTML, probeMark) {
		out := *rendered
		out.HTML = r.raw
		r.served.Add(1)
		return &out, nil
	}
	return rendered, err
}

// TestReaderPageRunsNoScript is plan 0015 Phase 4's direct probe for script
// in the built app's reader. The first subtest bypasses only maild's
// sanitizer, so the page's CSP (uikit.PageCSP) and content rules
// (uikit.PageRules) are what stand.
func TestReaderPageRunsNoScript(t *testing.T) {
	for _, subtest := range []string{"page defenses only", "with maild's sanitizer"} {
		t.Run(subtest, func(t *testing.T) {
			var selfchecks atomic.Int64
			var mu sync.Mutex
			var hits int
			var paths []string
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			origin := "http://" + ln.Addr().String()
			srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/selfcheck" {
					selfchecks.Add(1)
					return
				}
				mu.Lock()
				hits++
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				if r.URL.Path == "/probe.js" {
					w.Header().Set("Content-Type", "text/javascript")
					_, _ = fmt.Fprint(w, probeJS(origin, "remote-js-body"))
					return
				}
				w.Header().Set("Content-Type", "text/html")
				_, _ = fmt.Fprint(w, `<script>`+probeJS(origin, "remote-html-body")+`</script>`)
			})}
			go func() { _ = srv.Serve(ln) }()
			t.Cleanup(func() { _ = srv.Close() })
			res, err := http.Get(origin + "/selfcheck")
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			mu.Lock()
			initialHits := hits
			mu.Unlock()
			if n := selfchecks.Load(); n != 1 || initialHits != 0 {
				t.Fatalf("canary selfcheck: %d selfchecks, %d other requests", n, initialHits)
			}

			vectors := probeVectors(origin)
			page := probePage(origin)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "01-script-probe.html"), []byte(page), 0o600); err != nil {
				t.Fatal(err)
			}
			o := options{fixture: []string{"-n", "0", "-hostile", dir}}
			raw := &rawRender{raw: page}
			if subtest == "page defenses only" {
				o.server = &rpctest.Options{Services: func(services *api.Services) {
					raw.MessageService, services.Message = services.Message, raw
				}}
			}
			s := start(t, o)
			openHostile(s)
			// The page's handlers and requests run as it loads, with no end
			// signal. TestHostileMailMakesNoRequests waits the same way.
			time.Sleep(3 * time.Second)
			s.key(1, "CmdOrCtrl+F")
			s.input(1, map[string]any{"type": "text", "text": probeMark})
			counts := regexp.MustCompile(`^(\d+) of (\d+)$`)
			tree := s.waitFor(1, "the probe mark's find count", func(tree []node) bool {
				return find(tree, "status", counts.String()) != nil
			})
			match := counts.FindStringSubmatch(find(tree, "status", counts.String()).Label)
			total, err := strconv.Atoi(match[2])
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Second)
			mu.Lock()
			requests := hits
			gotPaths := append([]string(nil), paths...)
			mu.Unlock()
			scripts := total - 1
			t.Logf("%s on %s: %d vectors, %d script executions, %d canary requests", subtest, runtime.GOOS, len(vectors), scripts, requests)
			if scripts != 0 {
				t.Errorf("the reader ran %d scripts", scripts)
			}
			if requests != 0 {
				t.Errorf("the reader made %d canary requests: %v", requests, gotPaths)
			}
			if subtest == "page defenses only" && raw.served.Load() < 1 {
				t.Error("the reader did not receive the unsanitized probe page")
			}
		})
	}
}
