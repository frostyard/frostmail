//go:build linux

package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/frostyard/std/problems"

	"github.com/frostyard/frostmail/internal/mimex"
	"github.com/frostyard/frostmail/internal/problemstest"
)

// unparsable is a stored message mimex cannot read: its first header line
// has no colon.
var unparsable = []byte("no colon here\r\n\r\nbody\r\n")

// A message that mimex cannot parse records mime.parse_failed, from Render
// and from Part, with the error's text as the record's MESSAGE. The reader
// still gets the error.
func TestUnparsableMessageRecordsMimeParseFailed(t *testing.T) {
	j := problemstest.Listen(t)
	r := &Renderer{Parts: &PartsCache{Root: t.TempDir()}, Problems: j.Recorder(t, problems.ComponentDaemon)}

	_, err := r.Render(t.Context(), 1, unparsable, false)
	if !errors.Is(err, mimex.ErrParse) {
		t.Fatalf("Render = %v, want a parse error", err)
	}
	rec := j.NextError(t, problems.ComponentDaemon, CodeParseFailed)
	if rec.Message() != err.Error() {
		t.Errorf("MESSAGE = %q, want %q", rec.Message(), err)
	}
	j.None(t)

	if _, err := r.Part(1, unparsable, "1"); !errors.Is(err, mimex.ErrParse) {
		t.Fatalf("Part = %v, want a parse error", err)
	}
	j.NextError(t, problems.ComponentDaemon, CodeParseFailed)
	j.None(t)
}

// Other failures record nothing: a part that is not there, and a message
// that renders.
func TestOtherRenderFailuresRecordNothing(t *testing.T) {
	j := problemstest.Listen(t)
	r := &Renderer{Parts: &PartsCache{Root: t.TempDir()}, Problems: j.Recorder(t, problems.ComponentDaemon)}
	raw := newsletter(t, "https://example.test")
	if _, err := r.Part(7, raw, "9"); !errors.Is(err, ErrNoPart) {
		t.Fatalf("Part = %v, want ErrNoPart", err)
	}
	if _, err := r.Render(t.Context(), 7, raw, false); err != nil {
		t.Fatal(err)
	}
	j.None(t)

	// Without a recorder, an unparsable message only fails.
	r.Problems = nil
	if _, err := r.Render(t.Context(), 1, unparsable, false); err == nil || !strings.Contains(err.Error(), "no colon here") {
		t.Fatalf("Render = %v", err)
	}
	j.None(t)
}
