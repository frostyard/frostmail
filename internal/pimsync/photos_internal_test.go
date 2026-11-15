package pimsync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/graph"
)

// photoServer answers each contact's photo with its id, 404 for ids that
// start with "none", and 500 for "bad".
func photoServer(t *testing.T) *graph.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/me/contacts/"), "/photo/$value")
		switch {
		case id == "bad":
			http.Error(w, `{"error": {"code": "InternalServerError", "message": "no"}}`, http.StatusInternalServerError)
		case strings.HasPrefix(id, "none"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": {"code": "ImageNotFound", "message": "no photo"}}`))
		default:
			_, _ = w.Write([]byte("photo of " + id))
		}
	}))
	t.Cleanup(srv.Close)
	return &graph.Client{Base: srv.URL + "/", HTTP: srv.Client(),
		Token: func(context.Context) (string, error) { return "token", nil }}
}

func TestContactPhotosReadsEachByID(t *testing.T) {
	c := photoServer(t)
	var ids []string
	for i := range 10 {
		ids = append(ids, fmt.Sprint("c", i), fmt.Sprint("none", i))
	}
	photos, err := contactPhotos(t.Context(), c, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != len(ids) {
		t.Fatalf("%d photos for %d contacts", len(photos), len(ids))
	}
	for i := range 10 {
		if got := string(photos[fmt.Sprint("c", i)]); got != fmt.Sprint("photo of c", i) {
			t.Errorf("c%d's photo = %q", i, got)
		}
		if got := photos[fmt.Sprint("none", i)]; got != nil {
			t.Errorf("none%d's photo = %q, want none", i, got)
		}
	}
	if photos, err := contactPhotos(t.Context(), c, nil); err != nil || len(photos) != 0 {
		t.Errorf("no contacts: %v, %v", photos, err)
	}
}

func TestContactPhotosStopsOnAnError(t *testing.T) {
	c := photoServer(t)
	ids := []string{"c1", "c2", "bad", "c3", "c4", "c5", "c6"}
	if _, err := contactPhotos(t.Context(), c, ids); err == nil || !strings.Contains(err.Error(), "contact photo") {
		t.Errorf("a failed photo read = %v, want its error", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := contactPhotos(ctx, c, ids); !errors.Is(err, context.Canceled) {
		t.Errorf("an ended context = %v, want context.Canceled", err)
	}
}
