package main

import (
	"encoding/json"
	"log/slog"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
)

// The window is kept on the device (the kit's standard for an app, rule 8;
// ADR-0063): its size, its place where the platform says it, its panes'
// widths, and how the list is shown. A start shows the window as it was
// left. It is the device's, as the appearance is, and lives in the client's
// storage, not in maild.

const (
	panesKey = "pane-widths"
	viewKey  = "view"
	// todoKey keeps whether the To-Do Bar is open.
	todoKey = "todo-bar"
	// The least size of a kept window: under it the window opens at its
	// own first size, and the window does not go under it.
	minWidth, minHeight core.Points = 480, 360
	// The message list's first width, and its least.
	listFirst, listLeast core.Points = 390, 180
	// The To-Do Bar's first width.
	todoFirst core.Points = 280
)

// keptPanes are the widths of the panes: the shell's, and the message
// list's beside the reader.
type keptPanes struct {
	workspaceui.PaneWidths
	List core.Points
}

// keptView is how the list is shown: what the MyGo client kept in its
// window's state, up to 0.10.0.
type keptView struct {
	Photos, Conversations bool
	Sort                  api.ViewSort
	Ascending             bool
}

func loadKept[T any](cx *core.Scope, key string) (T, bool) {
	var v T
	text, ok := cx.Storage().GetItem(key)
	if !ok || json.Unmarshal([]byte(text), &v) != nil {
		var zero T
		return zero, false
	}
	return v, true
}

func (w *workspace) keep(key string, v any) {
	text, err := json.Marshal(v)
	if err == nil {
		err = w.cx.Storage().SetItem(key, string(text))
	}
	if err != nil {
		slog.Warn("keeping the window's state", "key", key, "err", err)
	}
}

// todoOpen is whether the device kept the To-Do Bar open.
func (w *workspace) todoOpen() bool {
	open, _ := loadKept[bool](w.cx, todoKey)
	return open
}

func (w *workspace) keepTodo(open bool) { w.keep(todoKey, open) }

// panes are the kept widths of the panes, with their first widths where
// the device keeps none.
func (w *workspace) panes() keptPanes {
	k, _ := loadKept[keptPanes](w.cx, panesKey)
	if k.Inspector <= 0 {
		k.Inspector = todoFirst
	}
	if k.List < listLeast {
		k.List = listFirst
	}
	return k
}

// keepPanes keeps the shell's widths, and keepList the message list's, as
// a drag of a divider changes them. Each leaves the other's as kept.
func (w *workspace) keepPanes(widths workspaceui.PaneWidths) {
	k := w.panes()
	k.PaneWidths = widths
	w.keep(panesKey, k)
}

func (w *workspace) keepList(width core.Points) {
	k := w.panes()
	k.List = width
	w.keep(panesKey, k)
}

// keptListState is the list's state at a start: how the device kept it,
// or conversations by date with photos.
func keptListState(cx *core.Scope) (mail.ListState, bool) {
	state, photos := mail.ListState{Conversations: true, Filter: mail.FilterAll, Sort: api.ViewSortDate}, true
	if k, ok := loadKept[keptView](cx, viewKey); ok {
		photos, state.Conversations, state.Ascending = k.Photos, k.Conversations, k.Ascending
		for _, s := range mail.Sorts {
			if s.Field == k.Sort {
				state.Sort = k.Sort
			}
		}
	}
	return state, photos
}

// keepView keeps how the list is shown, after a change of it.
func (w *workspace) keepView() {
	w.keep(viewKey, keptView{Photos: w.showPhotos.GetUntracked(), Conversations: w.listState.Conversations, Sort: w.listState.Sort, Ascending: w.listState.Ascending})
}
