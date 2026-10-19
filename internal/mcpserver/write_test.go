package mcpserver

import "testing"

func TestReplyHTML(t *testing.T) {
	quote := `<p><br></p><p>On Monday, Ann wrote:</p><blockquote type="cite">Hi</blockquote>`
	for _, c := range []struct{ text, body, want string }{
		{"Thanks.", quote, `<p>Thanks.</p>` + quote},
		{"a <b> & c\r\nnext line\n\n\n second", quote,
			`<p>a &lt;b&gt; &amp; c<br>next line</p><p>second</p>` + quote},
		{"Thanks.", `<blockquote>x</blockquote>`, `<p>Thanks.</p><blockquote>x</blockquote>`},
	} {
		if got := replyHTML(c.text, c.body); got != c.want {
			t.Errorf("replyHTML(%q) = %s, want %s", c.text, got, c.want)
		}
	}
}
