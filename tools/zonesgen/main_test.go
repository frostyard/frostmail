package main

import (
	"strings"
	"testing"
)

func TestParseAndGenerate(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><supplementalData><windowsZones><mapTimezones>`)
	b.WriteString(`<mapZone other="Eastern Standard Time" territory="001" type="America/New_York"/>`)
	b.WriteString(`<mapZone other="Eastern Standard Time" territory="US" type="America/New_York America/Detroit"/>`)
	b.WriteString(`<mapZone other="W. Europe Standard Time" territory="001" type="Europe/Berlin Europe/Busingen"/>`)
	for i := range 100 {
		b.WriteString(`<mapZone other="Zone ` + string(rune('A'+i%26)) + string(rune('a'+i/26)) + `" territory="001" type="Etc/UTC"/>`)
	}
	b.WriteString(`</mapTimezones></windowsZones></supplementalData>`)
	zones, err := parse([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if zones["Eastern Standard Time"] != "America/New_York" || zones["W. Europe Standard Time"] != "Europe/Berlin" {
		t.Errorf("zones = %v", zones)
	}
	src, err := generate(zones)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `"Eastern Standard Time": `) || !strings.HasPrefix(string(src), "// Code generated") {
		t.Errorf("generated:\n%s", src)
	}
	if _, err := parse([]byte(`<supplementalData/>`)); err == nil {
		t.Error("an empty file parsed")
	}
}
