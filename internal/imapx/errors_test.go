package imapx_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/internal/imapx"
)

func TestIsServerError(t *testing.T) {
	e := &imap.Error{Type: imap.StatusResponseTypeNo}
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain", errors.New("plain"), false},
		{"server", e, true},
		{"wrapped", fmt.Errorf("x: %w", e), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := imapx.IsServerError(tc.err); got != tc.want {
				t.Errorf("IsServerError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
