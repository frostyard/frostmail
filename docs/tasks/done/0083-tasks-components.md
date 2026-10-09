---
id: "0083"
title: Build the Tasks module's components
milestone: M4.5
size: L
touch:
  - app/src/lib/taskText.ts
  - app/src/features/tasks/TasksSidebar.tsx
  - app/src/features/tasks/TaskList.tsx
  - app/src/features/tasks/TaskPane.tsx
given:
  - app/src/features/tasks/fixtures.ts
  - app/src/lib/taskText.test.ts
  - app/src/features/tasks/TasksSidebar.test.tsx
  - app/src/features/tasks/TaskList.test.tsx
  - app/src/features/tasks/TaskPane.test.tsx
acceptance: make ui-vitest F="src/lib/taskText.test.ts src/features/tasks"
---
# T-0083: Build the Tasks module's components

## Goal

The pure helpers and the presentational pieces of the Tasks module: due
text, which tasks a source shows, the list's lines, the Tasks sidebar,
the task list (tasks or flagged messages) and the task pane. They take
data and callbacks by props; T-0085 wires them to maild.

## Read first

- `docs/specs/pim-ui.md`: "Tasks module" and its subsections (the
  measurements, colors and behavior come from there), and "Rules".
- The stubs you replace: `app/src/lib/taskText.ts` and
  `app/src/features/tasks/{TasksSidebar,TaskList,TaskPane}.tsx`. Keep
  their exported names, types and props exactly; replace the bodies and
  the "Task T-0083 builds it" lines.
- `app/src/features/people/PeopleSidebar.tsx`: the sidebar to mirror
  (sections, rows, selection colors, keys); `PersonRow.tsx` and
  `app/src/features/list/MessageRow.tsx` (the flag icon and its
  `aria-label`); `app/src/features/calendar/EventPane.tsx` (a pane's
  `dl` rows).
- `app/src/lib/calendarDates.ts` (`addDays`), `lib/format.ts`
  (`formatListDate`, `formatHeaderDate`, `displayName`), `lib/flags.ts`.
- `app/src/styles/app.css`: the theme's utility names.
- The given tests and `fixtures.ts`.
- `docs/tasks/EXECUTOR.md`

## Contract

The tests pin these; the spec gives the rest of the look.

- **lib/taskText.ts**
  - `dueText`: "" for no date; "Today", "Tomorrow", "Yesterday" by
    days from today (across a year's end too); otherwise in today's year
    `{ weekday: "short", month: "short", day: "numeric" }` ("Fri, Oct
    16"), else `{ month: "short", day: "numeric", year: "numeric" }`
    ("Oct 9, 2027"), formatted from the date's UTC midnight with
    `timeZone: "UTC"`.
  - `isOverdue`: not completed, a due date, and due before today
    (`YYYY-MM-DD` strings compare as text).
  - `sourceTasks`: keeps the order. Today: due today or earlier. All
    Tasks: every task. A list: its tasks. Flagged Mail: none. A
    completed task stays only when it is in `ticked`, or when
    `showCompleted` is on and the source is a list or All Tasks (never
    Today).
  - `openCount`: the source's tasks that are not completed.
  - `taskLines`: level 1 when the task's `parentId` is a task shown
    earlier in the same call, else 0. With `names`, a header line before
    the first task and before each task whose `listId` differs from the
    previous task's.
  - `dueSoon`: open tasks with a due date on or before today + 7 days,
    sorted by due date, ties in their given order, at most `limit`.
- **TasksSidebar:** as `PeopleSidebar`: a `tree` named "Task Lists" with
  `tabIndex` 0; section header buttons ("Tasks", then each section's
  title; sections without lists are left out) with `aria-expanded`;
  `treeitem` rows with `data-key` (`today`, `all`, `flagged`,
  `list:<id>`), `aria-selected`, icons `Sun`, `ListChecks`, `Flag` and
  `List`, the names "Today", "All Tasks", "Flagged Mail" and the list's
  name, a `Lock` with `aria-label="Read-only"` for a read-only list, and
  a count element with `data-count` when the count is above 0
  (`text-accent-contrast` in the focused selection). Selected rows are
  `bg-accent` (with `text-accent-contrast`) when `focused`, else
  `bg-selection-sidebar`. Clicking a row calls `onSelect` with its
  source (`"today"`, `"all"`, `"flagged"` or the list's ID); ArrowUp,
  ArrowDown, Home and End on the tree move within the shown rows.
- **TaskList:** a `listbox` named `title` with `tabIndex` 0.
  - `canCreate` shows the New Task field first: an input with
    `aria-label` and `placeholder` "New Task", given `newTaskRef`.
    Enter calls `onCreate(trimmed)` when the trimmed text is not empty
    and empties the field; Escape empties it and focuses the listbox.
  - Lines: a header line is an element with `role="presentation"`,
    `data-list` (the list ID) and the list's name as its only text. A
    task line is an `option` with `data-id`, `aria-selected`, and
    `style.paddingLeft` `12px`, or `40px` at level 1.
  - Inside an option: a `button` with `role="checkbox"`, `aria-label`
    "Completed", `aria-checked` the completion, no text, disabled when
    the task is read-only; clicking it calls `onToggle(id,
    !completed)` and stops propagation so the row is not selected. Then
    the title (its own element; `text-tertiary` when completed), and
    when there is a due date or notes a second line: the due text
    (`dueText`) as its own element (`text-flag-1` when `isOverdue`),
    " · " between it and the notes' first line when both show. A task
    with `messageId` shows the `Mail` icon with `aria-label` "From mail".
    A task with neither due date nor notes has only its title as text.
  - Selection: `aria-selected` for `selected`; `bg-accent` with
    `text-accent-contrast` when `focused`, else `bg-selection-inactive`.
    Clicking an option calls `onSelect(id)`. Keep the selected option in
    view (`scrollIntoView({ block: "nearest" })` when it changes; happy-dom
    may lack it, so call it optionally).
  - With `messages` (not null) the options are the messages instead,
    with `data-id` the message ID: the same checkbox (unchecked; clicking
    calls `onToggle(id, true)`), the subject or "No Subject", then
    `displayName(from)` (the name, else the address), " · " and
    `formatListDate(new Date(date), now, locale)`, and a filled `Flag`
    with `aria-label` "Flagged <flagName(color)>" and the class
    `text-flag-<color>`.
  - Empty: "No Tasks", or "No Flagged Mail" with `messages`, after the
    field.
- **TaskPane:**
  - No task and no message: "No Task Selected".
  - A task: an input with `aria-label` "Title" (given `titleRef`;
    `readOnly` when the task is), keeping a draft that starts over when
    another task (`id`) or a new title arrives. Enter and blur commit a
    trimmed, non-empty draft that differs from the title as
    `onChange(id, { title })`; an empty draft returns to the title;
    Escape returns to it without committing. Read-only: a "Read-only"
    line (its own text) with a `Lock`.
  - Then a `dl` of `dt`/`dd` rows: **Due**: an `<input type="date">`
    with `aria-label` "Due" (disabled when read-only) whose change
    calls `onChange(id, { due })`, and when there is a date and the
    task is writable a `button` "Clear Due Date" (`X` icon) calling
    `onChange(id, { due: "" })`; **List**: `list.name`, " · ",
    `list.account`; **Completed** only when completed:
    `Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle:
    "short", timeZone })` of `completedAt`; **From mail** only with a
    `messageId`: a `button` "Open Message" calling
    `onOpenMessage(messageId)`.
  - **Notes:** a textarea with `aria-label` "Notes" (`readOnly` when
    read-only), with a draft like the title's; blur commits a changed
    draft as `onChange(id, { notes })`.
  - A `button` "Delete Task" calling `onDelete(id)`, left out when
    read-only.
  - A message: an `h2` with the subject (or "No Subject"), immediately
    followed by an element with `displayName(from)`, " · " and
    `formatHeaderDate(new Date(date), locale)`; buttons "Open in Mail"
    (`onOpenMessage(id)`) and "Clear Flag" (`onClearFlag(id)`).

## Tests (given, do not edit)

`app/src/lib/taskText.test.ts` and
`app/src/features/tasks/{TasksSidebar,TaskList,TaskPane}.test.tsx`, with
their shared `fixtures.ts`. They run in UTC with `en-US`; texts are
compared after replacing the thin and narrow no-break spaces `Intl` uses
with spaces.

## Gotchas

- `getByText` matches an element's own text: keep each tested text
  (the due text, "Read-only", "No Tasks") as the whole text of one
  element.
- A blur handler that reads state from its closure sees the old draft
  after Escape resets it; reset through the input's value or a ref, or
  make Escape not blur.
- `noUncheckedIndexedAccess` is on. Keep components under ~150 lines;
  split helpers inside the touched files.

## Out of scope

Containers, stores, the toolbar, keys, data fetching (T-0084 and
T-0085), the To-Do bar (T-0086), and every file not under `touch`.

## Done when

`make accept T=0083` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
