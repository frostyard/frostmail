package oauth

// CONTRACT TEST for task card T-0051 (docs/tasks). Do not edit.

import (
	"bytes"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPKCE(t *testing.T) {
	// RFC 7636 appendix B.
	if got := Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Errorf("Challenge = %q", got)
	}
	v, err := NewVerifier(bytes.NewReader(bytes.Repeat([]byte{0xab}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	if v != base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xab}, 32)) {
		t.Errorf("NewVerifier = %q, want 32 random bytes in unpadded base64url", v)
	}
	if len(v) < 43 || len(v) > 128 {
		t.Errorf("verifier length %d", len(v))
	}
	if _, err := NewVerifier(bytes.NewReader(make([]byte, 10))); err == nil {
		t.Error("a short random source did not fail")
	}
}

func TestAuthURL(t *testing.T) {
	raw := AuthURL(Google, "id.apps.googleusercontent.com", "http://127.0.0.1:4242/", "st8", "chal", "ann@gmail.com")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme+"://"+u.Host+u.Path != Google.AuthURL {
		t.Errorf("base = %s", raw)
	}
	q := u.Query()
	want := map[string]string{
		"response_type": "code", "client_id": "id.apps.googleusercontent.com", "redirect_uri": "http://127.0.0.1:4242/",
		"scope": "https://mail.google.com/", "state": "st8", "code_challenge": "chal", "code_challenge_method": "S256",
		"access_type": "offline", "prompt": "consent", "login_hint": "ann@gmail.com",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	if len(q) != len(want) {
		t.Errorf("query has %d parameters, want %d: %s", len(q), len(want), u.RawQuery)
	}
	two := AuthURL(Endpoint{AuthURL: "https://auth.test/authorize", Scopes: []string{"a", "b c"}}, "id", "r", "s", "c", "")
	u2, _ := url.Parse(two)
	if u2.Query().Get("scope") != "a b c" || u2.Query().Has("login_hint") {
		t.Errorf("second URL = %s", two)
	}
	if !strings.HasPrefix(two, "https://auth.test/authorize?") {
		t.Errorf("second URL = %s", two)
	}
}

func TestParseToken(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	tok, err := ParseToken([]byte(`{"access_token":"ya29.a","expires_in":3599,"refresh_token":"1//r","scope":"https://mail.google.com/","token_type":"Bearer"}`), 200, now)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Access != "ya29.a" || tok.Refresh != "1//r" || !tok.Expiry.Equal(now.Add(3599*time.Second)) {
		t.Errorf("token = %+v", tok)
	}
	tok, err = ParseToken([]byte(`{"access_token":"ya29.b","token_type":"Bearer"}`), 200, now)
	if err != nil || tok.Refresh != "" || !tok.Expiry.IsZero() {
		t.Errorf("refresh response = %+v, %v", tok, err)
	}
	_, err = ParseToken([]byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`), 400, now)
	var te *TokenError
	if !errors.As(err, &te) || te.Code != "invalid_grant" || te.Description != "Token has been expired or revoked." {
		t.Errorf("error response = %v", err)
	}
	for name, c := range map[string]struct {
		body   string
		status int
	}{
		"no access token": {`{"token_type":"Bearer"}`, 200},
		"not JSON":        {`<html>bad gateway</html>`, 502},
		"status only":     {`{}`, 500},
		"negative expiry": {`{"access_token":"x","expires_in":-5}`, 200},
	} {
		if _, err := ParseToken([]byte(c.body), c.status, now); err == nil {
			t.Errorf("%s: no error", name)
		} else if errors.As(err, &te) {
			t.Errorf("%s: a TokenError without an error field: %v", name, err)
		}
	}
}

func TestXOAuth2(t *testing.T) {
	got := XOAuth2("ann@gmail.com", "ya29.a")
	if string(got) != "user=ann@gmail.com\x01auth=Bearer ya29.a\x01\x01" {
		t.Errorf("XOAuth2 = %q", got)
	}
	payload := `{"status":"401","schemes":"Bearer","scope":"https://mail.google.com/"}`
	for name, in := range map[string][]byte{
		"decoded": []byte(payload),
		"base64":  []byte(base64.StdEncoding.EncodeToString([]byte(payload))),
	} {
		err := ParseXOAuth2Error(in)
		var ce *ChallengeError
		if !errors.As(err, &ce) || ce.Status != "401" || ce.Schemes != "Bearer" || ce.Scope != "https://mail.google.com/" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := ParseXOAuth2Error([]byte("garbage!")); err == nil {
		t.Error("garbage was read")
	}
}
