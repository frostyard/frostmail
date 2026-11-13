package mail

import (
	"fmt"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestPreviewable(t *testing.T) {
	for _, tc := range []struct {
		part api.Part
		want bool
	}{
		{api.Part{ContentType: "image/png", Filename: "a.png", Size: 10}, true},
		{api.Part{ContentType: "IMAGE/JPEG", Filename: "a", Size: 10}, true},
		{api.Part{ContentType: "image/gif", Size: 10}, true},
		{api.Part{ContentType: "image/webp", Size: 10}, true},
		{api.Part{ContentType: "image/bmp", Size: 10}, true},
		{api.Part{ContentType: "application/octet-stream", Filename: "Photo.JPG", Size: 10}, true}, // no type: by its name
		{api.Part{ContentType: "", Filename: "a.png", Size: 10}, true},
		{api.Part{ContentType: "image/svg+xml", Filename: "a.svg", Size: 10}, false}, // a document, not pixels
		{api.Part{ContentType: "image/tiff", Filename: "a.tiff", Size: 10}, false},
		{api.Part{ContentType: "application/pdf", Filename: "a.png", Size: 10}, false}, // the type decides when given
		{api.Part{ContentType: "text/html", Filename: "a.jpg", Size: 10}, false},
		{api.Part{ContentType: "application/octet-stream", Filename: "setup.exe", Size: 10}, false},
		{api.Part{ContentType: "image/png", Size: 0}, false},
		{api.Part{ContentType: "image/png", Size: MaxPreviewBytes}, true},
		{api.Part{ContentType: "image/png", Size: MaxPreviewBytes + 1}, false},
	} {
		if got := Previewable(tc.part); got != tc.want {
			t.Errorf("Previewable(%+v) = %v", tc.part, got)
		}
	}
}

func TestPreviewsKeepTheFirstFew(t *testing.T) {
	var parts []api.Part
	for i := range MaxPreviews + 3 {
		parts = append(parts, api.Part{Path: fmt.Sprint(2*i + 1), ContentType: "application/pdf", Size: 10},
			api.Part{Path: fmt.Sprint(2*i + 2), ContentType: "image/png", Size: 10})
	}
	got := Previews(parts)
	if len(got) != MaxPreviews || got[0].Path != "2" || got[MaxPreviews-1].Path != fmt.Sprint(2*MaxPreviews) {
		t.Errorf("previews = %d, first %q, last %q", len(got), got[0].Path, got[len(got)-1].Path)
	}
	if got := Previews(nil); len(got) != 0 {
		t.Errorf("no attachments gave %d", len(got))
	}
}

func TestPreviewSize(t *testing.T) {
	for _, tc := range []struct {
		w, h         int
		wantW, wantH float32
	}{
		{440, 220, 220, 110}, // wide
		{240, 240, 120, 120}, // the height decides
		{100, 50, 100, 50},   // small: its own size
		{50, 400, 15, 120},   // tall
		{0, 10, 0, 0},
	} {
		if w, h := PreviewSize(tc.w, tc.h, 220, 120); w != tc.wantW || h != tc.wantH {
			t.Errorf("PreviewSize(%d, %d) = %v × %v, want %v × %v", tc.w, tc.h, w, h, tc.wantW, tc.wantH)
		}
	}
}
