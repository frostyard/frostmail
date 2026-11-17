package engine_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// P-957: the desktop app's text size is a preference maild keeps, default
// until the user chooses, and changing another preference leaves it.
func TestSettingsTextSize(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	s, err := c.Settings().Get(ctx, &api.SettingsGetParams{})
	if err != nil || s.TextSize == nil || *s.TextSize != api.TextSizeDefault {
		t.Fatalf("a new store's textSize = %v, %v", s.TextSize, err)
	}
	for _, size := range []api.TextSize{api.TextSizeLargest, api.TextSizeSmaller, api.TextSizeLarger, api.TextSizeDefault} {
		s, err = c.Settings().Set(ctx, &api.SettingsSetParams{TextSize: &size})
		if err != nil || s.TextSize == nil || *s.TextSize != size {
			t.Fatalf("set %q: %+v, %v", size, s, err)
		}
		if got, err := c.Settings().Get(ctx, &api.SettingsGetParams{}); err != nil || *got.TextSize != size {
			t.Fatalf("get after set %q: %+v, %v", size, got, err)
		}
	}
	larger := api.TextSizeLarger
	if _, err := c.Settings().Set(ctx, &api.SettingsSetParams{TextSize: &larger}); err != nil {
		t.Fatal(err)
	}
	if s, err = c.Settings().Set(ctx, &api.SettingsSetParams{UndoDelay: ptr(int64(30)), PreviewLines: ptr(int64(4))}); err != nil || *s.TextSize != larger {
		t.Errorf("another preference changed textSize: %+v, %v", s, err)
	}
	for _, bad := range []api.TextSize{"huge", ""} {
		if _, err := c.Settings().Set(ctx, &api.SettingsSetParams{TextSize: &bad}); code(err) != api.CodeInvalidParams {
			t.Errorf("textSize %q: %v", bad, err)
		}
	}
	if got, err := c.Settings().Get(ctx, &api.SettingsGetParams{}); err != nil || *got.TextSize != larger {
		t.Errorf("after the refusals: %+v, %v", got, err)
	}
}
