package render

import "strings"

// pageStyle is the reader page's style: white paper in both color schemes
// (docs/specs/ui.md, Reader: Body), as the app's frame template has it
// (app/src/features/reader/MessageFrame.tsx).
const pageStyle = `html{color-scheme:light}` +
	`body{margin:0;padding:20px;background:#fff;color:#000;font:14px/1.45 "Inter Variable",system-ui,sans-serif;overflow-wrap:anywhere}` +
	`img{max-width:100%;height:auto}`

// Page wraps a rendering's sanitized HTML in the reader's whole page, for
// lodge's content route (docs/specs/lodge-app.md). Every link targets a new
// tab: the sanitizer drops base and target from messages (ADR-0036).
func Page(html string) []byte {
	var b strings.Builder
	b.Grow(len(html) + 512)
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><meta name="color-scheme" content="light"><base target="_blank"><style>`)
	b.WriteString(pageStyle)
	b.WriteString(`</style></head><body>`)
	b.WriteString(html)
	b.WriteString(`</body></html>`)
	return []byte(b.String())
}
