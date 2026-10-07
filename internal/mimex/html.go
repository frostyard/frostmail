package mimex

import "golang.org/x/net/html"

// HTMLToText renders an HTML message part as plain text for previews and the
// search index. Task T-0007 implements it; the stub only pins the x/net
// dependency it uses.
func HTMLToText(s string) string {
	_ = html.NewTokenizer
	return ""
}
