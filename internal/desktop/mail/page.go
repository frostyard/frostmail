package mail

import "strings"

// PageCSP is the policy of every page of HTML the app did not write: only
// data: images, fonts and media, inline style, no script, no form, and no
// base to move links with (docs/plans/0015-mygo-desktop.md, Phase 0's
// records).
const PageCSP = "default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src data:; media-src data:; base-uri 'none'; form-action 'none'"

const pageStyle = `html{color-scheme:light}` +
	`body{margin:0;padding:20px;background:#fff;color:#000;font:14px/1.45 "Inter Variable",system-ui,sans-serif;overflow-wrap:anywhere}` +
	`img{max-width:100%;height:auto}` +
	`@media screen{.print-only{display:none}body>.print-only:first-child+div{display:contents}}` +
	`.print-only{font-size:12px;margin:0 0 16px;padding:0 0 8px;border-bottom:1px solid #ccc}` +
	`.print-only div{margin:2px 0}`

// pageHeightStyle gives the page the height of its content: a message
// that sizes its root to the viewport (html,body{height:100%}) and clips or
// scrolls a box of that height would otherwise show only the viewport's
// part of it. With the root's height auto, a percentage height below it
// lays out as auto. The rule is in a layer declared before any of the
// message's, so no important rule of the message can undo it.
const pageHeightStyle = `@layer page-height{html,body{height:auto!important}}`

// ScrollingDocument wraps sanitized HTML in the reader's page, which owns
// its viewport and scrolls: the policy first, the page's style, style (the
// text style and the zoom of the text size), then the height rule, and the
// message in the body.
func ScrollingDocument(body, style string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="`)
	b.WriteString(PageCSP)
	b.WriteString(`"><meta charset="utf-8"><meta name="color-scheme" content="light"><base target="_blank"><style>`)
	b.WriteString(pageStyle)
	b.WriteString(style)
	b.WriteString(pageHeightStyle)
	b.WriteString(`</style></head><body>`)
	b.WriteString(body)
	b.WriteString("</body></html>")
	return b.String()
}
