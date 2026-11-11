package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/internal/desktop/pim"
)

// pimWindows keeps Calendar's windows by key. It belongs to the UI thread.
type pimWindows struct {
	windows map[string]*mygo.Window
	all     *windowSet
}

// open shows a window, raising it if it is open.
func (s *pimWindows) open(spec pim.WindowSpec) {
	if win := s.windows[spec.Key]; win != nil {
		win.Restore()
		win.Focus()
		return
	}
	style := mygo.TitleBarDefault
	if spec.Frameless {
		style = mygo.TitleBarHidden
	}
	win := mygo.NewWindow(mygo.WindowOptions{Title: spec.Title, Width: int(spec.Width), Height: int(spec.Height),
		MinWidth: int(spec.MinWidth), MinHeight: int(spec.MinHeight), AlwaysOnTop: spec.AlwaysOnTop,
		TitleBarStyle: style, Content: ui.View(spec.View)})
	s.windows[spec.Key] = win
	s.all.add(win)
	if spec.CloseRequested != nil {
		win.OnClose(func(e *mygo.CloseEvent) {
			if !spec.CloseRequested() {
				e.PreventDefault()
			}
		})
	}
	win.OnClosed(func() {
		delete(s.windows, spec.Key)
		if spec.OnClose != nil {
			spec.OnClose()
		}
	})
}

func (s *pimWindows) close(key string) {
	if win := s.windows[key]; win != nil {
		win.Close()
	}
}

func (s *pimWindows) setTitle(key, title string) {
	if win := s.windows[key]; win != nil {
		win.SetTitle(title)
	}
}

// redraw repaints the windows, whose content the main window's posts
// change.
func (s *pimWindows) redraw() {
	for _, win := range s.windows {
		win.Invalidate()
	}
}

// openImage asks for a contact's photo; "" when cancelled.
func openImage() (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "Choose a Photo",
		Filters: []mygo.FileFilter{{Name: "Images", Extensions: []string{"png", "jpg", "jpeg"}}}})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}
