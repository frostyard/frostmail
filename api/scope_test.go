package api

import (
	"errors"
	"testing"
)

func TestParseScope(t *testing.T) {
	for in, want := range map[string]string{
		"account=*":              "account=*",
		"account=3":              "account=3",
		"account=7,2,7":          "account=2,7",
		"account=*;write":        "account=*;write",
		"account=1;send":         "account=1;send",
		"account=2,1;write;send": "account=1,2;write;send",
	} {
		s, err := ParseScope(in)
		if err != nil || s.String() != want {
			t.Errorf("ParseScope(%q) = %v, %v; want %s", in, s, err, want)
		}
	}
	for _, bad := range []string{"", "account=", "account=0", "account=-1", "account=01", "account=1,", "account=a",
		"vault=1", "account=* ", " account=1", "account=1,*", "account=1;", "account=1;read", "account=1;send;write",
		"account=1;write;write", "account=1;write;", "account=;write", "account=1; write", "account=1;WRITE"} {
		if _, err := ParseScope(bad); !errors.Is(err, ErrScope) {
			t.Errorf("ParseScope(%q) = %v, want ErrScope", bad, err)
		}
	}
	s, _ := ParseScope("account=2,7")
	if !s.Admits(2) || !s.Admits(7) || s.Admits(3) {
		t.Errorf("account=2,7 admits %v", s)
	}
	if all, _ := ParseScope("account=*"); !all.Admits(99) {
		t.Error("account=* does not admit 99")
	}
	for in, want := range map[string][3]bool{
		"account=1":            {true, false, false},
		"account=1;write":      {true, true, false},
		"account=1;send":       {true, false, true},
		"account=1;write;send": {true, true, true},
	} {
		s, _ := ParseScope(in)
		got := [3]bool{s.Grants(NeedRead), s.Grants(NeedWrite), s.Grants(NeedSend)}
		if got != want {
			t.Errorf("%s grants read, write, send = %v, want %v", in, got, want)
		}
	}
}
