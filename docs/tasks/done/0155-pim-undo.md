---
id: "0155"
title: Undo in Calendar and People
milestone: M5.5
size: M
touch:
  - app/src/app/MainWindow.tsx
  - app/src/app/CalendarModule.tsx
  - app/src/app/PeopleModule.tsx
  - app/src/app/EventWindow.tsx
given:
  - app/src/app/Undo.pim.test.tsx
acceptance: make ui-vitest F="src/app/Undo src/app/CalendarModule src/app/PeopleModule src/app/EventWindow"
---
# T-0155: Undo in Calendar and People

## Goal

Ctrl+Z undoes event and contact writes as Mail's undo does (docs/specs/
pim-ui.md, "Undo in Calendar and People"; ADR-0032): every create,
change, delete, move and photo change keeps its restore on its module's
list, and Ctrl+Z, outside a text field, restores the module's newest
write. The lists are `app/pimUndo.ts`'s; the event editor window, which
is another window, announces its writes to the main window.

## Read first

- `docs/specs/pim-ui.md`: "Undo in Calendar and People".
- `app/src/app/pimUndo.ts`: `rememberRestore`, `undoPim`,
  `announceRestore`, `watchRestores`, and its test.
- `app/src/app/MainWindow.tsx`: `useWindowEvents` (the key handler and
  the watchers), `moduleCommand` (People's commands), `calendarCommand`,
  and how Mail's `undo` command runs.
- `app/src/app/CalendarModule.tsx` (its delete), `app/src/app/
  PeopleModule.tsx` (the contact editor's save, photo and delete), and
  `app/src/app/EventWindow.tsx` (its create, update and delete).
- `schema/rpc/calendar.yaml` and `people.yaml`: `EventWrite` and
  `ContactWrite` carry `restoreId`.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

- **Keeping restores:**
  - Calendar's delete (the pane's Delete, and Delete in the view) keeps
    `rememberRestore({module: "calendar", restoreId})` from the
    `calendar.delete` result. (Drops already do, T-0154.)
  - People keeps `{module: "people"}` for each successful
    `people.create`, `people.update`, `people.setPhoto` and
    `people.delete` (a save that sends two writes keeps two).
  - The event editor window announces each successful `calendar.create`,
    `update` and `delete` with `announceRestore({module: "calendar",
    restoreId})` before it closes; a failed announcement is logged and
    the window still closes.
- **The main window** runs `watchRestores()` (as it runs the other
  watchers) for as long as it is open.
- **Ctrl+Z** (the `undo` command, outside a text field) in Calendar calls
  `undoPim(client, "calendar")`, and in People `undoPim(client,
  "people")`; nothing happens when the list is empty. Mail's undo stays
  as it is. While a contact is being edited, Ctrl+Z stays the editor's.
  Failures are logged with `console.warn`.

## Tests (given, do not edit)

`app/src/app/Undo.pim.test.tsx`; the existing MainWindow, CalendarModule,
PeopleModule and EventWindow tests must still pass.

## Gotchas

- In the tests the app is not in Tauri: announcements go through a
  `BroadcastChannel`, which the same document's `watchRestores` hears.
- The views refetch after `calendar.changed` and `people.changed`; a
  restored event or contact shows without more work.

## Out of scope

A restore that would tell invitees asking first (Phase 5); every file not
under `touch`.

## Done when

`make accept T=0155` and `make ui-check` pass, and only the files under
`touch` changed.
