---
id: "0146"
title: The scope sheet, and the event pane's Edit and Delete
milestone: M5.5
size: M
touch:
  - app/src/features/calendar/ScopeSheet.tsx
  - app/src/features/calendar/EventPane.tsx
  - app/src/app/CalendarModule.tsx
given:
  - app/src/features/calendar/ScopeSheet.test.tsx
  - app/src/app/CalendarModule.edit.test.tsx
acceptance: make ui-vitest F="src/features/calendar/ScopeSheet src/app/CalendarModule src/features/calendar/EventPane"
---
# T-0146: The scope sheet, and the event pane's Edit and Delete

## Goal

Events become editable from Calendar (docs/specs/pim-ui.md, "Event pane"
and "Scope sheet"; ADR-0031). The event pane gets Edit and Delete; Edit
opens the event editor window, and Delete deletes the occurrence, asking
first which occurrences of a repeating event it takes. The calendar
module offers both to the main window for its keys (a later card).

## Read first

- `docs/specs/pim-ui.md`: "Event pane" (Edit and Delete), "Scope sheet",
  "Calendar behavior".
- `app/src/features/calendar/EventPane.tsx`, `app/src/app/CalendarModule.tsx`
  (`CalendarHandle`, the pane's calendar and section), and
  `app/src/app/useCalendar.ts` (`CalendarData`: `event`, `sections`).
- `app/src/app/eventWindow.ts`: `openEvent`, `EditorTarget`.
- `app/src/features/later/TimeSheet.tsx`: the dialog frame the sheet
  shares; `features/settings/labels.ts`: `BUTTON`, `PRIMARY_BUTTON`.
- `schema/rpc/calendar.yaml`: `calendar.delete`, `EventScope`,
  `CalendarEvent.first`, `recurring`, `organizer`.
- The given tests, and `app/src/app/CalendarModule.test.tsx` for the
  harness.
- `docs/tasks/EXECUTOR.md`

## Contract

### `ScopeSheet` (new, `features/calendar/ScopeSheet.tsx`)

- Props: `action` (`"change"` or `"delete"`), `first` (the occurrence is
  its series' first), `onChoose(scope: EventScope)`, `onCancel()`.
- A modal `role="dialog"` (`aria-modal`), 420 wide, over a dimmed window,
  as `TimeSheet` is. Its title (its accessible name) is "Change Repeating
  Event" or "Delete Repeating Event"; under it the text "Change only this
  event, this and all future events, or every event in the series?"
  ("Delete only…" for a delete).
- Buttons stacked at the right, in order: **This Event** (primary, and
  focused when the sheet opens), **All Future Events** (absent when
  `first`), **All Events**, **Cancel**. They call `onChoose("this")`,
  `("future")`, `("all")` and `onCancel`. Escape cancels; Tab and
  Shift+Tab stay within the sheet; keys do not reach the window. Focus
  returns where it was when the sheet closes.

### `EventPane`

- New optional props `onEdit` and `onDelete`. When given, the pane's
  heading row ends with 28 × 28 icon buttons: `Pencil` named "Edit Event"
  and `Trash2` named "Delete Event" (each with a matching `title`). The
  summary, location and Cancelled line keep their place at its left.
  Nothing else changes.

### `CalendarModule`

- The selected event is **writable** when its calendar (from the
  sections) is not read-only and the event is not read-only. It is an
  **invitation** when it has an organizer who is not the user.
- **Edit** (for a writable event): `openEvent({kind: "edit", id,
  recurrenceId}, summary)` with the event's own `id` and `recurrenceId`.
- **Delete** (for a writable event that is not an invitation): an
  occurrence of a series (`recurring` with a `recurrenceId`) opens the
  `ScopeSheet` with `action="delete"` and `first` from the event; its
  answer calls `calendar.delete {id, recurrenceId, scope}`, and Cancel
  writes nothing. Anything else calls `calendar.delete {id}` at once.
  After a delete the selection is cleared (`selectOccurrence(null)`);
  a failed delete is logged with `console.warn`.
- `CalendarHandle` gains `edit()` and `remove()`, which do what the
  pane's buttons do for the selected event and return whether there was
  one to edit or delete.

## Tests (given, do not edit)

`app/src/features/calendar/ScopeSheet.test.tsx` and
`app/src/app/CalendarModule.edit.test.tsx`; the existing EventPane and
CalendarModule tests must still pass.

## Gotchas

- In the tests the app is not in Tauri, so `openEvent` opens a tab with
  `window.open`, which they spy on.
- The view refetches after `calendar.changed`; the deleted occurrence
  disappears from it without more work.

## Out of scope

The editor itself; New Event, Ctrl+N and the view's keys (later cards);
undo (Phase 4); every file not under `touch`.

## Done when

`make accept T=0146` and `make ui-check` pass, and only the files under
`touch` changed.
