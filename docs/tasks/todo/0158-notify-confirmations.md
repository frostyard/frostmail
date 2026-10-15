---
id: "0158"
title: Asking before invitees hear, in the editor, the views and undo
milestone: M5.5
size: M
touch:
  - app/src/app/EventWindow.tsx
  - app/src/app/CalendarModule.tsx
  - app/src/app/MainWindow.tsx
given:
  - app/src/app/Notify.calendar.test.tsx
acceptance: make ui-vitest F="src/app/Notify src/app/Calendar src/app/EventWindow src/app/Undo src/app/MainWindow"
---
# T-0158: Asking before invitees hear, in the editor, the views and undo

## Goal

maild refuses a calendar write that would tell invitees until the user
says yes (`notifyRequired`, ADR-0035). T-0156 made the notify sheet and
`withNotify`; this card runs every calendar write the app makes through
them (docs/specs/pim-ui.md, "Sending to invitees", and the editor's,
the views' and undo's notes on it). Send repeats the write with `notify:
true`; Cancel writes nothing.

## Read first

- `docs/specs/pim-ui.md`: "Sending to invitees (`NotifySheet`)", "Event
  editor window" (Save), "Event pane" (Delete), "Changing events in the
  views", "Undo in Calendar and People".
- `app/src/lib/notify.ts` (`withNotify`) and
  `app/src/features/calendar/NotifySheet.tsx` (`useNotify`).
- `app/src/app/pimUndo.ts`: `undoPim(client, module, ask)`.
- `app/src/app/EventWindow.tsx` (`perform`), `app/src/app/CalendarModule.tsx`
  (`useCalendarDrag`'s `update`, the pane's `deleteEvent`) and
  `app/src/app/MainWindow.tsx` (`calendarCommand`'s `undo`,
  `WindowHandles`).
- The given test, and `CalendarModule.drag.test.tsx` and
  `Undo.pim.test.tsx` for the harness.
- `docs/tasks/EXECUTOR.md`

## Contract

- **Event editor window:** Save (`calendar.create` or `calendar.update`)
  and Delete Event (`calendar.delete`) go through `withNotify` with the
  window's own `useNotify()`; its sheet renders in the window. Send
  sends the same call with `notify: true` and goes on as before
  (announce the restore, close). Cancel leaves the window open with the
  draft and writes nothing; the scope sheet, when there was one, is
  already closed.
- **Calendar:** a drop's `calendar.update` (after the scope sheet for a
  series) and the pane's and the view's delete (`calendar.delete`, after
  the scope sheet for a series) go through `withNotify`; their sheets
  render in the module. A cancelled write keeps no restore and, for a
  delete, leaves the selection as it was.
- **Ctrl+Z in Calendar** passes the main window's ask to
  `undoPim(client, "calendar", ask)`: the main window has a
  `useNotify()` and renders its sheet. (On Cancel, `undoPim` keeps the
  write on the list.)
- Writes that tell nobody (maild answers without `notifyRequired`) show
  no sheet. People's writes do not change.

## Tests (given, do not edit)

`app/src/app/Notify.calendar.test.tsx`; the existing Calendar,
EventWindow, Undo and MainWindow tests must still pass.

## Gotchas

- The given test turns invitations on in the mock's writable
  calendars; the fixture's own calendars do not invite, so the earlier
  tests write without a sheet.
- `withNotify` returns `null` when the user cancels: check for it
  before reading `restoreId`.
- The sheet stops its keys from reaching the window, so Escape in it
  does not also close the editor.

## Out of scope

The Invitees field (T-0159); every file not under `touch`.

## Done when

`make accept T=0158` and `make ui-check` pass, and only the files under
`touch` changed.
