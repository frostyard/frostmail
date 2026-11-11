package mainwin

import "github.com/frostyard/frostui"

// OpenLink opens a link through the opener, which refuses all but http,
// https and mailto; it waits for the system, so call it from a worker.
func (w *Window) OpenLink(raw string) {
	if err := w.links().OpenURL(raw); err != nil {
		w.toast("Could not open the link", err)
	}
}

func (w *Window) links() frostui.Opener {
	return frostui.Opener{Launch: w.o.Launch}
}
