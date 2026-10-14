---
id: "0149"
title: The event editor window
milestone: M5.5
size: M
touch:
  - app/src/app/EventWindow.tsx
given:
  - app/src/app/EventWindow.test.tsx
acceptance: make ui-vitest F=src/app/EventWindow
---
# T-0149: The event editor window

## Goal

The event editor window (docs/specs/pim-ui.md, "Event editor window")
loads what it edits, keeps the draft, and saves it: `calendar.create` for
a new event, `calendar.update` with the fields that changed, after the
scope sheet for an occurrence of a series (ADR-0031). Its footer deletes
the event, and closing it with changes asks first. The form is T-0148's
`EventEditor`, the draft T-0147's `lib/eventDraft.ts`.

## Read first

- `docs/specs/pim-ui.md`: "Event editor window" (Footer, Save, Read-only
  and invited) and "Scope sheet".
- `app/src/app/EventWindow.tsx` (the placeholder: keep `EventWindow`'s
  props) and `app/src/app/eventWindow.ts` (`EditorTarget`).
- `app/src/features/calendar/EventEditor.tsx` (T-0148),
  `app/src/features/calendar/ScopeSheet.tsx` (T-0146),
  `app/src/lib/eventDraft.ts` (T-0147).
- `app/src/app/ComposeWindow.tsx`: how a window closes itself and sets
  its title, in Tauri and in a browser.
- `features/settings/labels.ts`: `BUTTON`, `PRIMARY_BUTTON`, `ALERT`.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

- **Loading.** `account.list` and `account.collections {kind:
  "calendar"}` give the calendars; the writable ones are not read-only,
  in an account that is not read-only, offered as `CalendarChoice`s with
  the account's email. Then:
  - a new event's draft is `newDraft(target, calendarId, appZone)` with
    the target's `calendarId`, else the writable default calendar
    (`isDefault`), else the first writable one;
  - an existing event is `calendar.event {id, recurrenceId?}` (no
    `recurrenceId` when the target's is empty), its draft
    `draftOf(event, appZone)`. It is **read-only** when the event is, or
    its calendar is not writable (then the only calendar offered is its
    own), and an **invitation** when its organizer is not the user.
  - `appZone` is `Intl.DateTimeFormat().resolvedOptions().timeZone`.
    While loading the window shows "Loading…"; a failed load shows its
    error (`role="alert"`).
- **The window:** a 32-high title strip (a drag region) with the title
  and a **Close** button (an `X`), then the form scrolling with 20px
  padding (the form shows its own "Read-only" line), then the footer.
  The form gets the zones (the
  app's, the event's own, then `Intl.supportedValuesOf("timeZone")`), the
  locale (`navigator.language`), `readOnly` and `invited`.
- **The title**, in the strip and the window's (`setTitle` in Tauri,
  `document.title` in a browser): the draft's summary, else "New Event".
- **Footer:** **Delete Event** (13/16 `--flag-1`) at the left for an
  existing event that is writable and not an invitation; at the right
  **Cancel** and **Save** (primary, a submit button, disabled while saving
  or while `validate` finds anything). A read-only event's footer is only
  **Close**. Enter in a field saves (the form's submit).
- **Save:** for an existing event with no changed fields, the window
  just closes. Otherwise the fields are `changedFields(original, draft)`
  (`original` null for a new event). A new event is
  `calendar.create {calendarId: draft.calendarId, fields}`; a single
  event `calendar.update {id, fields}`; an occurrence of a series (the
  event `recurring` with a `recurrenceId`) first opens the scope sheet
  (`action="change"`, `first` from the event), and its answer sends
  `calendar.update {id, recurrenceId, scope, fields}`. Success closes the
  window; a refusal shows maild's message (`role="alert"`, `ALERT`) and
  keeps it open.
- **Delete Event:** a series' occurrence through the scope sheet
  (`action="delete"`): `calendar.delete {id, recurrenceId, scope}`;
  anything else `calendar.delete {id}`. Success closes the window.
- **Closing:** Cancel, the strip's Close and Escape close at once when
  nothing changed (a new event: when its title is still empty). With
  changes they ask, in a sheet like the scope sheet: a dialog named
  `Save changes to "<title>"?` with **Save** (saves as above), **Don't
  Save** (closes) and **Cancel** (and Escape: back to the editor).
- Closing is `getCurrentWindow().close()` in Tauri and `window.close()`
  in a browser.

## Tests (given, do not edit)

`app/src/app/EventWindow.test.tsx`.

## Gotchas

- In the tests the app is not in Tauri; they spy on `window.close`.
- The mock repeats only daily and weekly: a monthly rule is refused, which
  is the refusal the test shows.

## Out of scope

New Event, Ctrl+N and the view's keys (T-0150); every file not under
`touch`.

## Done when

`make accept T=0149` and `make ui-check` pass, and only the files under
`touch` changed.
