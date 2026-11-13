package engine_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// P-211: the list's preview lines are a preference maild keeps, 2 until
// the user chooses, and changing it leaves the other preferences.
func TestSettingsPreviewLines(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	s, err := c.Settings().Get(ctx, &api.SettingsGetParams{})
	if err != nil || s.PreviewLines == nil || *s.PreviewLines != 2 {
		t.Fatalf("a new store's previewLines = %v, %v", s.PreviewLines, err)
	}
	if _, err := c.Settings().Set(ctx, &api.SettingsSetParams{UndoDelay: ptr(int64(30))}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int64{0, 5, 1} {
		s, err = c.Settings().Set(ctx, &api.SettingsSetParams{PreviewLines: &n})
		if err != nil || s.PreviewLines == nil || *s.PreviewLines != n || s.UndoDelay != 30 {
			t.Fatalf("set %d: %+v, %v", n, s, err)
		}
	}
	if s, err = c.Settings().Set(ctx, &api.SettingsSetParams{UndoDelay: ptr(int64(10))}); err != nil || *s.PreviewLines != 1 {
		t.Errorf("another preference changed previewLines: %+v, %v", s, err)
	}
	for _, n := range []int64{-1, 6} {
		if _, err := c.Settings().Set(ctx, &api.SettingsSetParams{PreviewLines: &n}); code(err) != api.CodeInvalidParams {
			t.Errorf("previewLines %d: %v", n, err)
		}
	}
	if got, err := c.Settings().Get(ctx, &api.SettingsGetParams{}); err != nil || *got.PreviewLines != 1 {
		t.Errorf("after the refusals: %+v, %v", got, err)
	}
}
