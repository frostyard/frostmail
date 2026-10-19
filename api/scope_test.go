package api

import (
	"errors"
	"testing"
)

func TestParseScope(t *testing.T) {
	for in, want := range map[string]string{
		"account=*":     "account=*",
		"account=3":     "account=3",
		"account=7,2,7": "account=2,7",
	} {
		s, err := ParseScope(in)
		if err != nil || s.String() != want {
			t.Errorf("ParseScope(%q) = %v, %v; want %s", in, s, err, want)
		}
	}
	for _, bad := range []string{"", "account=", "account=0", "account=-1", "account=01", "account=1,", "account=a",
		"account=*;write", "account=1;send", "vault=1", "account=* ", " account=1", "account=1,*"} {
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
}
