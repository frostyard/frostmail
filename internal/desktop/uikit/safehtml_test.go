package uikit_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

func TestDocumentCarriesThePolicyFirst(t *testing.T) {
	doc := uikit.Document(`<p>hi</p>`, "", "")
	policy := strings.Index(doc, `http-equiv="Content-Security-Policy"`)
	body := strings.Index(doc, `<body>`)
	if policy < 0 || body < policy {
		t.Fatalf("policy at %d, body at %d: %s", policy, body, doc)
	}
	for _, want := range []string{"default-src 'none'", "img-src data:", "base-uri 'none'", "form-action 'none'", `<base target="_blank">`} {
		if !strings.Contains(doc, want) {
			t.Errorf("no %q in %s", want, doc)
		}
	}
	for _, never := range []string{"script-src", "connect-src", "unsafe-eval", "http:", "https:"} {
		if strings.Contains(uikit.PageCSP, never) {
			t.Errorf("the policy allows %q", never)
		}
	}
}

func TestPrintHeaderIsPrintOnly(t *testing.T) {
	doc := uikit.Document("body", "<div>From: Ann</div>", "")
	if !strings.Contains(doc, `<div class="print-only"><div>From: Ann</div></div>body`) {
		t.Errorf("print header missing: %s", doc)
	}
	if !strings.Contains(doc, `@media screen{.print-only{display:none}}`) {
		t.Errorf("the print header shows on screen: %s", doc)
	}
}

func TestPageRulesBlockAllButDataAndAbout(t *testing.T) {
	var rules []struct {
		Trigger struct {
			URLFilter string `json:"url-filter"`
		} `json:"trigger"`
		Action struct {
			Type string `json:"type"`
		} `json:"action"`
	}
	if err := json.Unmarshal([]byte(uikit.PageRules), &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 || rules[0].Action.Type != "block" || rules[0].Trigger.URLFilter != ".*" {
		t.Fatalf("rules = %+v", rules)
	}
	for _, r := range rules[1:] {
		if r.Action.Type != "ignore-previous-rules" || (r.Trigger.URLFilter != "^data:" && r.Trigger.URLFilter != "^about:") {
			t.Errorf("rule allows more than data: and about: %+v", r)
		}
	}
}
