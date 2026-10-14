---
id: "0148"
title: The event editor's form and the Custom Repeat sheet
milestone: M5.5
size: L
touch:
  - app/src/features/calendar/EventEditor.tsx
  - app/src/features/calendar/RepeatSheet.tsx
given:
  - app/src/features/calendar/EventEditor.test.tsx
  - app/src/features/calendar/RepeatSheet.test.tsx
acceptance: make ui-vitest F="src/features/calendar/EventEditor src/features/calendar/RepeatSheet"
---
# T-0148: The event editor's form and the Custom Repeat sheet

## Goal

The event editor window (docs/specs/pim-ui.md, "Event editor window")
shows an event's fields top to bottom and edits a draft
(`lib/eventDraft.ts`, T-0147). Build the form as a controlled component
that holds nothing but the open Custom Repeat sheet, and the sheet. The
window that loads and saves the draft is the next card's.

## Read first

- `docs/specs/pim-ui.md`: "Event editor window" (fields, alerts, the
  repeat sheet, read-only and invited).
- `app/src/lib/eventDraft.ts`: `EventDraft`, `DraftErrors`, `moveStart`.
- `app/src/lib/repeat.ts` (T-0143): `presetOf`, `presetRule`, `endOf`,
  `withEnd`, `repeatOf`, `ruleOf`, `defaultRepeat`, `nthWeekday`,
  `WEEKDAYS`, `Repeat`; `app/src/lib/eventText.ts`: `describeRecurrence`,
  `alarmText`, `calendarColor`; `app/src/lib/calendarDates.ts`:
  `localeWeekStart`.
- `app/src/features/people/ContactEditor.tsx` (rows, labels, inputs),
  `app/src/features/later/TimeSheet.tsx` (a sheet's frame) and
  `features/settings/labels.ts` (`FIELD`, `BUTTON`, `PRIMARY_BUTTON`).
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

### `EventEditor` (`features/calendar/EventEditor.tsx`)

- Exports `CalendarChoice` (`id`, `name`, `color`, `account`),
  `EventEditorProps` and `EventEditor`. Props: `draft`, `onChange(draft)`,
  `errors`, `calendars` (the choices), `zones` (the zones to offer, in
  order), `locale`, and optional `readOnly` and `invited`.
- Every change calls `onChange` with a whole new draft; nothing is
  written.
- **Controls**, by their accessible names, top to bottom:
  - **Title** (placeholder "New Event"), 17/22 600, and **Location**
    (placeholder "Add Location");
  - then rows, each with a label 12/16 `--text-secondary` (80 wide):
    - **All-day**, a checkbox;
    - **Start date** and **Start time**, **End date** and **End time**
      (no times for an all-day draft). A change to the start goes through
      `moveStart` (the end keeps its distance; a rule naming the start's
      day follows it); a change to the end sets it alone. An `errors.start`
      or `errors.end` marks those inputs `aria-invalid` and shows its text
      after them;
    - **Time zone** (timed only): a select of `zones`, with the draft's
      own zone added at the end when the list lacks it;
    - **Repeat**: the options Never, Every Day, Every Weekday, Every Week,
      Every 2 Weeks, Every Month, Every Year with values `never`, `daily`,
      `weekdays`, `weekly`, `biweekly`, `monthly`, `yearly`; then, for a
      rule the presets do not cover (`presetOf` is `custom`), an option
      `custom` showing `describeRecurrence("RRULE:" + rule, locale)`; then
      Custom… (value `edit`). The select shows `presetOf(rule, start)`.
      Choosing a preset writes `presetRule(preset)` with the rule's end
      kept (`withEnd(…, endOf(rule, zone), allDay, zone)`); Custom… opens
      the sheet;
    - **End repeat** (when the draft repeats): Never, On Date, After
      (values `never`, `date`, `count`), as `endOf` reads the rule. On Date
      starts on the draft's start date and shows an **End repeat date**
      input; After starts at 10 and shows a **Repeat count** number input
      and "times". Each writes the rule with `withEnd`;
    - **Calendar**: a select of the choices by ID, in an `optgroup` per
      account (labeled with it, in the choices' order), each option the
      calendar's name after a colored dot;
    - **Alerts**: a select per alert, named **Alert 1**, **Alert 2**, …,
      with None (value `none`, which removes it), the presets
      (`TIMED_ALERTS`, or `ALL_DAY_ALERTS` for an all-day draft; export
      both as `[minutes, label][]`), and the alert's own value as
      `alarmText` when it is no preset. **Add Alert**, a text button,
      appends the first preset not yet used;
    - **Show as**: Busy and Free (values `busy`, `free`; `transparent`).
  - **Notes**, a textarea at least 120 high.
- The presets: timed, At time of event (0), 5, 10, 15 and 30 minutes
  before, 1 hour (60) and 2 hours (120) before, 1 day (1440), 2 days
  (2880) and 1 week (10080) before; all-day, On the day at 9:00 AM
  (−540), 1 day before at 9:00 AM (900), 2 days before at 9:00 AM (2340),
  1 week before at 9:00 AM (9540).
- **`readOnly`** disables every control. **`invited`** disables all but
  the alerts (their selects and Add Alert).

### `RepeatSheet` (`features/calendar/RepeatSheet.tsx`)

- Props: `initial` (a `Repeat`), `start` (the first day), `locale`,
  `onChoose(r)`, `onCancel()`.
- A modal `role="dialog"` named "Custom Repeat", 420 wide, in
  `TimeSheet`'s frame; Escape cancels and keys do not reach the window.
- **Frequency**, a select (Daily, Weekly, Monthly, Yearly; values the
  `Frequency`); **Every**, a number input, followed by the unit (day,
  week, month, year; plural when not 1).
- Weekly: a group named "Days" of seven toggle buttons, named by the
  days' English names, with `aria-pressed`, starting on the locale's
  first day (`localeWeekStart`, Sunday 0). Switching to Weekly with no
  days chosen chooses the start's day.
- Monthly: two radios, "On day <the start's day>" and "On the <first,
  second, third, fourth or last> <weekday>" (`nthWeekday(start)`), which
  set `byWeekday`.
- **OK** (disabled while the interval is not a whole number of at least
  1, or a weekly rule has no days) calls `onChoose` with the repeat, its
  weekdays in `WEEKDAYS` order; **Cancel** calls `onCancel`.
- In the editor, OK writes `ruleOf(r, start)` with the rule's end kept.
  The sheet starts from `repeatOf(rule, start)`, else `defaultRepeat`.

## Tests (given, do not edit)

`app/src/features/calendar/EventEditor.test.tsx` and
`app/src/features/calendar/RepeatSheet.test.tsx`.

## Gotchas

- Biome forbids array-index keys; an alert's place in the list is its
  only identity, so say so in a `biome-ignore` comment on that key.
- `fireEvent.change` on a `<select>` needs the option's value to exist.

## Out of scope

Loading and saving (`EventWindow`, the next card); every file not under
`touch`.

## Done when

`make accept T=0148` and `make ui-check` pass, and only the files under
`touch` changed.
