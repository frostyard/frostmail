// Package frame is a stand-in for the frame without a rail that frostui is
// getting (plan 0015): a top bar, full-height content, the status bar and
// the console, over frostui's palette, dialogs and theme. It holds only
// what frostui's Shell cannot do yet. When frostui's frame lands, the main
// window moves to it and this package goes.
package frame

import (
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// Spec is what one frame shows.
type Spec struct {
	// Bar draws the top bar's start, up to the palette field.
	Bar func()
	// Actions draws the buttons between the palette field and the menu.
	Actions func()
	// Menu builds the main menu, the last item before the window controls.
	Menu func(m *ui.Menu)
	// Content draws everything between the bars, at full height.
	Content func()
	// Commands are the palette's.
	Commands []frostui.Command
	// Status and StatusEnd are the status bar's items.
	Status, StatusEnd []frostui.StatusItem
	// Console is what the console shows, nil for none.
	Console *frostui.Console
}

// Shell draws a Spec. Keep one in the window's state.
type Shell struct {
	Palette frostui.Palette
	Console frostui.ConsoleView
	Dialogs *frostui.Dialogs
	Info    frostui.Info

	splitPercent float32
}

// View draws the frame; call it from the root view every frame.
func (s *Shell) View(c *ui.Context, f Spec) {
	t := frostui.Adapt(c)
	s.Palette.RunPending()
	if s.splitPercent == 0 {
		s.splitPercent = 70
	}
	ui.Column(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		s.topBar(c, f)
		ui.Box(c).Height(1).Background(t.Border)
		if s.Console.Open && f.Console != nil {
			ui.SplitVerticalPercent(c, &s.splitPercent, func() { content(c, f) }, func() { s.Console.View(c, f.Console) }).Grow(1)
		} else {
			content(c, f)
		}
		ui.Box(c).Height(1).Background(t.Border)
		ui.Row(c).Height(24).Padding(0, frostui.SpaceM).Gap(frostui.SpaceL).AlignItems(ui.Center).
			Background(frostui.Chrome(t)).Role(ui.RoleStatus).Children(func() {
			for i, it := range f.Status {
				statusItem(c.Key([2]any{"s", i}), it)
			}
			ui.Spacer(c)
			for i, it := range f.StatusEnd {
				statusItem(c.Key([2]any{"e", i}), it)
			}
		})
	})
	if s.Dialogs != nil {
		s.Dialogs.View(c)
	}
	s.Info.View(c)
	s.Palette.View(c, f.Commands)
}

func content(c *ui.Context, f Spec) {
	ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(f.Content)
}

func (s *Shell) topBar(c *ui.Context, f Spec) {
	t := c.Theme()
	tb := c.TitleBar()
	ui.Row(c).Height(frostui.TopBarHeight).Padding(0, frostui.SpaceM+tb.Right, 0, frostui.SpaceM+tb.Left).Gap(frostui.SpaceM).
		AlignItems(ui.Center).Background(frostui.Chrome(t)).DragWindow().Children(func() {
		if f.Bar != nil {
			f.Bar()
		}
		ui.Spacer(c)
		field := ui.Row(c).Width(300).Height(32).Padding(0, 10).Gap(frostui.SpaceS).Radius(16).AlignItems(ui.Center).
			Background(t.Background).Border(1, t.Border).Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).
			Label("Search or run a command").Tooltip("Command palette (Ctrl K)")
		if field.Hovered() {
			field.Border(1, t.Accent.Alpha(0.6))
		}
		if field.Clicked() {
			s.Palette.Show()
		}
		field.Children(func() {
			frostui.Icon(c, "system-search").TextColor(t.TextMuted)
			frostui.Muted(c, "Search or run a command").Grow(1).SingleLine()
			frostui.Shortcut(c, "Ctrl", "K")
		})
		if f.Actions != nil {
			f.Actions()
		}
		if f.Menu != nil {
			ui.MenuButton(c, "", f.Menu).Label("Main menu").Tooltip("Main menu").Children(func() {
				frostui.Icon(c, "open-menu")
			})
		}
	})
}

func statusItem(c *ui.Context, it frostui.StatusItem) {
	t := c.Theme()
	color := t.TextMuted
	if it.Tone != frostui.Neutral {
		color = frostui.ToneColor(t, it.Tone)
	}
	row := ui.Row(c).Gap(6).Padding(2, 4).Radius(4).AlignItems(ui.Center)
	if it.Tooltip != "" {
		row.Tooltip(it.Tooltip)
	}
	if it.OnClick != nil {
		label := it.Text
		if it.Tooltip != "" {
			label = it.Tooltip
		}
		row.Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).Label(label)
		if row.Hovered() {
			row.Background(t.SurfaceHover)
		}
		if row.Clicked() {
			it.OnClick()
		}
	}
	row.Children(func() {
		if it.Icon != "" {
			frostui.Icon(c, it.Icon).FontSize(12).TextColor(color)
		}
		ui.Text(c, it.Text).FontSize(12).TextColor(color).SingleLine()
	})
}

// Waker coalesces redraw requests from many goroutines into at most one
// every interval, as bursts of sync events would otherwise rebuild the
// view each time. frostui is getting ChairLift's; this goes with it.
type Waker struct {
	post  func(func())
	every time.Duration

	mu      sync.Mutex
	pending bool
}

// NewWaker returns a Waker that redraws through post.
func NewWaker(post func(func()), every time.Duration) *Waker {
	return &Waker{post: post, every: every}
}

// Wake asks for a redraw.
func (w *Waker) Wake() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending {
		return
	}
	w.pending = true
	time.AfterFunc(w.every, func() {
		w.mu.Lock()
		w.pending = false
		w.mu.Unlock()
		w.post(func() {})
	})
}
