---
id: "0086"
title: Build the To-Do bar
milestone: M4.5
size: L
touch:
  - app/src/features/tasks/ToDoBar.tsx
  - app/src/app/ToDoBarContainer.tsx
  - app/src/app/MainWindow.tsx
  - app/src/app/useCalendar.ts
  - app/src/app/useTasks.ts
given:
  - app/src/app/ToDoBar.test.tsx
acceptance: make ui-vitest F="src/app"
---
# T-0086: Build the To-Do bar

## Goal

Mail's optional right-hand pane (ADR-0020): the small month, the next
events and what is due, with tasks and flagged mail one click from done.
The toolbar's To-Do Bar button and the persisted `todoBar` setting exist
(T-0084, T-0085); this card builds the pane and puts it beside the
reader.

## Read first

- `docs/specs/pim-ui.md`: "To-Do bar", and "Task list" for the rows'
  look.
- `app/src/app/useTasks.ts` (T-0085: tasks, flagged messages, toggle,
  clear flag, open message; active in Mail while `todoBar` is on),
  `app/src/app/useCalendar.ts` (`useCalendarFrame`, `useCalendarColors`,
  `openOccurrence`, the range loading you may export),
  `app/src/app/MainWindow.tsx` (`MailPanes`).
- `app/src/features/calendar/MiniMonth.tsx` and `UpcomingList.tsx` (reuse
  both), `app/src/features/tasks/TaskList.tsx` (the check circle and
  flag icon to match), `app/src/lib/taskText.ts` (`dueSoon`, `dueText`,
  `isOverdue`), `app/src/lib/eventLayout.ts` (`busyDates`).
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

- **ToDoBar** (`features/tasks/ToDoBar.tsx`, presentational): an `aside`
  (`complementary`) named "To-Do Bar", 280 wide, `bg-sidebar`, a 1px
  `border-separator` left border, scrolling as a whole. In order:
  - `MiniMonth` (selected date, today, week start, busy days, locale);
    selecting a day calls back with it.
  - Upcoming: `UpcomingList` with the given occurrences when there are
    any (it brings its own "Upcoming" heading and region); otherwise an
    `h3` "Upcoming" and "No Upcoming Events".
  - An `h3` "Tasks", then a New Task input (`aria-label` and placeholder
    "New Task", 32 high): Enter calls back with the trimmed title when
    not empty and empties the field.
  - A `list` named "Due" (`aria-label`) of `listitem`s with `data-kind`
    `task` or `message` and `data-id`: the tasks first, then the
    messages. A task's item: the check circle (a `button` with
    `role="checkbox"`, `aria-label` "Completed", `aria-checked`), a
    `button` whose name is the task's title (opens the task), and the
    due text (`dueText`) at the right in its own element, with
    `text-flag-1` when `isOverdue`. A message's item: the same check
    circle (clears the flag), a `button` with the subject ("No
    Subject"), and a filled `Flag` with `aria-label` "Flagged <name>"
    and `text-flag-<color>`. With neither: "Nothing Due".
- **ToDoBarContainer** (`app/ToDoBarContainer.tsx`) feeds it:
  - the small month: `useCalendarFrame`'s locale, week start and today;
    selected date `calendarDate` or today; busy days from
    `calendar.range` over the month's 42 days (`monthGrid`,
    `busyDates`); a day shows Calendar on it (`setCalendarDate`,
    `setModule("calendar")`, the view kept);
  - upcoming: `calendar.range` from today to today + 8 days in the app's
    zone, keeping occurrences whose end is after now, at most 5, with
    `useCalendarColors`; refetched on `calendar.changed` and when the
    minute clock moves; clicking one is `openOccurrence`;
  - tasks: `dueSoon(tasks, today, 10)` from `useTasks`; messages: the
    first 5 of its flagged messages (the view is newest first);
  - ticking a task is `toggle`; ticking a message is `clearFlag`; New
    Task is `tasks.create({ title })` (the default list, no due date;
    add an action to `useTasks` for it); a task's title opens it in
    Tasks (`setModule("tasks")`, `setTasksSource(task.listId)`, then
    `selectTask(task.id)`); a message's subject is `revealMessage`
    (Mail stays).
- **MainWindow:** `MailPanes` puts the container after the reader when
  `todoBar` is on; the reader keeps `min-w-0 flex-1` and narrows.

## Tests (given, do not edit)

`app/src/app/ToDoBar.test.tsx` drives `MainWindow` over MockTransport
(the fixture's task lists, flagged mail and calendars, dated from now).
The other app tests must keep passing.

## Gotchas

- `setTasksSource` clears the selection: select the task after it.
- `UpcomingList` returns null without occurrences; render the empty
  state yourself.
- Only Mail shows the bar; `useTasks` is already inactive elsewhere.
- Keep files under ~200 lines.

## Out of scope

The Tasks module (T-0085), the toolbar (T-0084), and every file not
under `touch`.

## Done when

`make accept T=0086` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
