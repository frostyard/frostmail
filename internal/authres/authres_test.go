package authres

import "testing"

func TestSenderCheck(t *testing.T) {
	tests := []struct {
		name, header string
		want         Verdict
	}{
		{"dmarc pass", "mx.google.com; dkim=pass header.i=@a.example; spf=pass; dmarc=pass (p=REJECT) header.from=a.example", Pass},
		{"dmarc fail", "mx.example; dkim=fail; spf=pass; dmarc=fail header.from=bank.example", Fail},
		{"many dkim, one passes", "mx.example; dkim=fail (body hash did not verify); dkim=pass header.d=list.example", Pass},
		{"dkim fail alone", "mx.example; dkim=fail", Fail},
		{"dmarc none beats dkim fail", "mx.example; dkim=fail; dmarc=none", None},
		{"spf fail alone", "mx.example; spf=fail", None},
		{"unknown result", "mx.example; dkim=passed", None},
		{"empty", "", None},
		{"authserv-id only", "mx.example", None},
		{"case", "mx.example; DMARC=FAIL", Fail},
		{"version", "mx.example; dkim/1=fail", Fail},
		{"comment holds dkim=pass", "mx.example; dkim=fail (dkim=pass); spf=pass", Fail},
		{"semicolon in a comment", "mx.example; dkim=pass (old; dmarc=fail) header.d=a.example", Pass},
		{"nested comment", "mx.example; dkim=fail (a (dkim=pass) b)", Fail},
		{"quoted paren", `mx.example; dkim=pass header.i="a(b"; dmarc=fail`, Fail},
		{"quoted semicolon", `mx.example; dkim=fail reason="x; dkim=pass"`, Fail},
		{"dmarc fail with dkim pass", "mx.example; dkim=pass; dmarc=fail", Fail},
		{"dmarc pass with dkim fail", "mx.example; dkim=fail; dmarc=pass", Pass},
		{"dmarc none with dkim pass", "mx.example; dkim=pass; dmarc=none", None},
		{"second dmarc skipped", "mx.example; dmarc=pass; dmarc=fail", Pass},
		{"second dmarc skipped, none first", "mx.example; dmarc=none; dmarc=pass", None},
		{"softfail", "mx.example; dmarc=softfail", None},
		{"policy", "mx.example; dkim=policy", None},
		{"folded", "mx.example;\r\n\tdkim=pass\r\n header.d=a.example", Pass},
		{"garbage piece skipped", "mx.example; ???; =x; dkim=pass", Pass},
		{"spf, arc, iprev, auth never decide", "mx.example; spf=pass; arc=pass; iprev=pass; auth=pass", None},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SenderCheck(tt.header); got != tt.want {
				t.Errorf("SenderCheck(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}
