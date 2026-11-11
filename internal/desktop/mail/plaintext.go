package mail

import (
	"html"
	"regexp"
	"strings"
)

// A text body's blocks (docs/specs/ui.md, Reader: Body): runs of lines at
// one quote level, and the signature.
type textBlock struct {
	signature bool
	quote     int
	lines     []string
}

// quoteFold is how many lines a long quote keeps before See More, and
// quoteLimit the most lines a quote shows whole.
const (
	quoteFold  = 2
	quoteLimit = 4
)

// maxQuoteDepth is the most levels a quote draws: the count of ">" comes
// from the message, and each level is a nested element of the page.
const maxQuoteDepth = 8

func splitQuote(line string) (level int, content string) {
	i := 0
	for i < len(line) {
		if line[i] == '>' {
			level++
			i++
			continue
		}
		if level > 0 && line[i] == ' ' && i+1 < len(line) && line[i+1] == '>' {
			i++
			continue
		}
		break
	}
	if level > 0 && i < len(line) && line[i] == ' ' {
		i++
	}
	return level, line[i:]
}

func parseText(text string) []textBlock {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	start, end := 0, len(raw)
	for start < end && strings.TrimSpace(raw[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(raw[end-1]) == "" {
		end--
	}
	sig := -1
	for i := start; i < end; i++ {
		if raw[i] == "-- " {
			sig = i
			break
		}
	}
	body := raw[start:end]
	if sig >= 0 {
		body = raw[start:sig]
	}
	var blocks []textBlock
	for _, line := range body {
		level, content := splitQuote(line)
		level = min(level, maxQuoteDepth)
		if n := len(blocks); n > 0 && blocks[n-1].quote == level {
			blocks[n-1].lines = append(blocks[n-1].lines, content)
			continue
		}
		blocks = append(blocks, textBlock{quote: level, lines: []string{content}})
	}
	if sig >= 0 {
		blocks = append(blocks, textBlock{signature: true, lines: raw[sig+1 : end]})
	}
	return blocks
}

var linkPattern = regexp.MustCompile(`https?://[^\s<>"]+|www\.[^\s<>"]+|[\w.+-]+@[\w-]+(?:\.[\w-]+)+`)

func linkHref(match string) string {
	switch {
	case strings.HasPrefix(match, "www."):
		return "https://" + match
	case strings.HasPrefix(match, "http://"), strings.HasPrefix(match, "https://"):
		return match
	}
	return "mailto:" + match
}

// linked writes a line as HTML, its URLs and addresses as links.
func linked(b *strings.Builder, line string) {
	last := 0
	for _, loc := range linkPattern.FindAllStringIndex(line, -1) {
		end := loc[1]
		for end > loc[0] && strings.ContainsRune(".,;:!?)", rune(line[end-1])) {
			end--
		}
		if end == loc[0] {
			continue
		}
		text := line[loc[0]:end]
		b.WriteString(html.EscapeString(line[last:loc[0]]))
		b.WriteString(`<a href="`)
		b.WriteString(html.EscapeString(linkHref(text)))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(text))
		b.WriteString(`</a>`)
		last = end
	}
	b.WriteString(html.EscapeString(line[last:]))
}

func writeLines(b *strings.Builder, lines []string) {
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		linked(b, line)
	}
}

// quoteColors are --quote-N of docs/specs/ui.md, in the page's own
// literals, for levels 1 to 3 and deeper.
var quoteColors = []string{"#007aff", "#34c759", "#af52de"}

// TextStyle is the page style a text body's HTML needs.
const TextStyle = `.text{font-size:14px;line-height:21px;white-space:pre-wrap;margin:0}` +
	`.quote{margin:0;padding-left:10px;border-left:2px solid}` +
	`.sig{color:#6e6e73}` +
	`details>summary{color:#007aff;cursor:pointer;list-style:none;font-size:12px}` +
	`details>summary::-webkit-details-marker{display:none}` +
	`details[open]>summary{display:none}`

// TextHTML writes a text body as the reader's page shows it: quotes as
// indented bars instead of ">" prefixes, a long quote folded behind See
// More (a <details>, so the page needs no script), links, and a dimmed
// signature. Every piece of the text is escaped.
func TextHTML(text string) string {
	var b strings.Builder
	b.WriteString(`<div class="text">`)
	for _, blk := range parseText(text) {
		switch {
		case blk.signature:
			b.WriteString(`<div class="sig">`)
			writeLines(&b, blk.lines)
			b.WriteString(`</div>`)
		case blk.quote == 0:
			b.WriteString(`<div>`)
			writeLines(&b, blk.lines)
			b.WriteString(`</div>`)
		default:
			writeQuote(&b, blk)
		}
	}
	b.WriteString(`</div>`)
	return b.String()
}

func writeQuote(b *strings.Builder, blk textBlock) {
	for level := 1; level <= blk.quote; level++ {
		b.WriteString(`<div class="quote" style="border-color:`)
		b.WriteString(quoteColors[min(level, len(quoteColors))-1])
		b.WriteString(`">`)
	}
	if len(blk.lines) > quoteLimit {
		b.WriteString(`<div>`)
		writeLines(b, blk.lines[:quoteFold])
		b.WriteString(`</div><details><summary>See More</summary><div>`)
		writeLines(b, blk.lines[quoteFold:])
		b.WriteString(`</div></details>`)
	} else {
		writeLines(b, blk.lines)
	}
	b.WriteString(strings.Repeat(`</div>`, blk.quote))
}

// printedHeader selects the header the app writes: the first child of the
// page's body, a place no element of the message can take.
const printedHeader = `body>.print-only:first-child`

// PrintGuard keeps a message's own CSS from hiding or forging the header on
// paper. The message's <style> shares the page and can name anything, with
// !important and any specificity. These rules come first in the page, in
// the first cascade layer, and the first layer's !important beats every
// later layer's and every unlayered one's; maild's sanitizer renames every
// layer a message declares, so none of its rules joins this one. So the
// header takes only these values, the page's frame cannot move it, no
// generated text goes with it, and no transition carries an older value
// onto paper. Nor can it paint the header through an ancestor: html and
// body take no filter, opacity, blending, mask, containment or skipped
// rendering, which a child's own values cannot undo. The message is all in the element after the header
// (uikit.Document), which the app writes, so no style attribute of the
// message reaches it: it paints below the header, and so does everything
// in it, whatever its own display, position or z-index.
const PrintGuard = `@layer print-guard{@media print{` +
	`html,body{display:block!important;position:static!important;float:none!important;transform:none!important;translate:none!important;rotate:none!important;scale:none!important;zoom:1!important;clip-path:none!important;mask:none!important;-webkit-mask:none!important;margin:0!important;border:0!important;width:auto!important;min-width:0!important;max-width:none!important;height:auto!important;min-height:0!important;max-height:none!important;overflow:visible!important;columns:auto!important;writing-mode:horizontal-tb!important;` +
	`filter:none!important;-webkit-filter:none!important;backdrop-filter:none!important;-webkit-backdrop-filter:none!important;` +
	`opacity:1!important;mix-blend-mode:normal!important;isolation:auto!important;-webkit-mask-box-image:none!important;` +
	`contain:none!important;content-visibility:visible!important;perspective:none!important;transition:none!important}` +
	`html{padding:0!important}` +
	`body{padding:0 20px 20px!important}` +
	`html::before,html::after,body::before,body::after{content:none!important}` +
	printedHeader + `,` + printedHeader + ` *{all:initial!important;font:12px/1.45 system-ui,sans-serif!important;color:#000!important;overflow-wrap:anywhere!important;direction:ltr!important;unicode-bidi:normal!important;transition:none!important}` +
	printedHeader + `{display:block!important;position:relative!important;z-index:2147483647!important;background:#fff!important;-webkit-print-color-adjust:exact!important;print-color-adjust:exact!important;margin:0 0 16px!important;padding:0 0 8px!important;border-bottom:1px solid #ccc!important}` +
	printedHeader + ` div{display:block!important;margin:2px 0!important}` +
	printedHeader + ` b{font-weight:700!important}` +
	printedHeader + `::before,` + printedHeader + `::after,` + printedHeader + ` *::before,` + printedHeader + ` *::after{content:none!important}` +
	printedHeader + `::first-line,` + printedHeader + `::first-letter,` + printedHeader + ` *::first-line,` + printedHeader + ` *::first-letter{all:inherit!important}` +
	printedHeader + `+*{display:block!important;position:relative!important;z-index:0!important;transition:none!important}` +
	`}}`

// PrintHeader writes the lines that head a message on paper, escaped; a
// warning, when there is one, follows them as the banner follows the header
// on screen.
func PrintHeader(from, to, cc, date, subject, warning string) string {
	var b strings.Builder
	line := func(label, value string) {
		if value == "" {
			return
		}
		b.WriteString(`<div><b>`)
		b.WriteString(label)
		b.WriteString(`:</b> `)
		b.WriteString(html.EscapeString(value))
		b.WriteString(`</div>`)
	}
	line("From", from)
	line("To", to)
	line("Cc", cc)
	line("Date", date)
	line("Subject", subject)
	if warning != "" {
		b.WriteString(`<div>`)
		b.WriteString(html.EscapeString(warning))
		b.WriteString(`</div>`)
	}
	return b.String()
}
