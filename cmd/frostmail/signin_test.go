package main

import "testing"

// Only an https page opens; the URL comes from maild.
func TestSignInOpensHTTPSOnly(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "frostmail-toast:open?message=1", "http://sign.in.test/", "javascript:alert(1)"} {
		var launched []string
		o := signInOpener(func(u string) error { launched = append(launched, u); return nil })
		if err := o.OpenURL(raw); err == nil {
			t.Errorf("%s opened", raw)
		}
		if len(launched) != 0 {
			t.Errorf("%s reached the system: %v", raw, launched)
		}
	}
	var launched []string
	o := signInOpener(func(u string) error { launched = append(launched, u); return nil })
	if err := o.OpenURL("https://accounts.test/authorize?x=1"); err != nil || len(launched) != 1 {
		t.Errorf("https: %v, launched %v", err, launched)
	}
}
