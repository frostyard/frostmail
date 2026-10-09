---
id: "0085"
title: Wire the Tasks module
milestone: M4.5
size: L
touch:
  - app/src/app/TasksModule.tsx
  - app/src/app/useTasks.ts
  - app/src/app/MainWindow.tsx
  - app/src/app/ToolbarContainer.tsx
  - app/src/app/SidebarContainer.tsx
  - app/src/app/CalendarModule.tsx
  - app/src/app/PeopleModule.tsx
given:
  - app/src/app/TasksModule.test.tsx
acceptance: make ui-vitest F="src/app"
---
# T-0085: Wire the Tasks module

## Goal

The Tasks module in the main window: its data from maild, the sidebar,
task list and pane (T-0083) in the module's panes, the toolbar's Tasks
mode and the To-Do Bar setting (T-0084), the keys, writes, flagged mail,
and opening messages in Mail. The To-Do bar itself is T-0086's, built on
the data hook this card writes.

## Read first

- `docs/specs/pim-ui.md`: "Modules", "Tasks module" and every
  subsection, "To-Do bar" (what the data hook must also serve).
- `app/src/app/CalendarModule.tsx`, `useCalendar.ts` (`useCalendarFrame`,
  `useRefresh`'s debounced refetch, cancelling stale answers),
  `PeopleModule.tsx` and `usePeople.ts`: the module and data patterns to
  follow. `app/src/app/MainWindow.tsx`: key dispatch (`commandFor`,
  `moduleCommand`, `calendarCommand`, `focusPane`) and the module switch.
- `app/src/app/ToolbarContainer.tsx`, `SidebarContainer.tsx`.
- `app/src/app/openMessage.ts` (`revealMessage`), `app/src/app/commands.ts`
  (`setFlagColor` or the `message.setFlags` call it makes),
  `app/src/data/useView.ts` and `view.ts` (the flagged view).
- `app/src/lib/taskText.ts`, `app/src/features/tasks/*` (T-0083), the
  tasks state in `app/src/data/stores.ts` and the Toolbar's `tasks` and
  `todoBar` props (T-0084), `app/src/rpc/mock/tasks.ts` and the fixture's
  task lists.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

- **Module bar:** every module's bar lists `["mail", "calendar",
  "people", "tasks"]` (Mail's in `SidebarContainer`, Calendar's and
  People's in their modules, Tasks' in `TasksModule`).
- **useTasks** (`useTasks.ts`), called by `MainWindow` like `useCalendar`,
  active while the module is Tasks or the module is Mail with
  `todoBar` on, returns what `TasksModule` and the To-Do bar need:
  - the frame (`timeZone`, `locale`, `today`, `now`, from
    `useCalendarFrame`);
  - every task: `tasks.list({ completed: true })`, refetched 100 ms
    after `tasks.changed` (debounced, stale answers dropped);
  - the task lists: `account.collections({ kind: "tasklist" })` that
    are enabled, refetched on `account.changed`, as `TaskListSection`s
    per account (titled with the account's email from `useMail`'s
    accounts, lists in collection order, counts from `openCount`);
  - the smart lists' counts (`openCount` for Today and All Tasks; the
    flagged view's `count`);
  - the flagged messages: while active, a view of `{ flagged: true }`
    (no threads) with rows 0–200 ensured; closed when inactive;
  - the `ticked` set (tasks ticked here; emptied when `tasksSource`
    changes) and the actions below.
- **Writes:**
  - toggle: `tasks.update({ id, completed })`, shown at once (the task's
    `completed` flipped locally and its ID added to `ticked` when
    completing), refetching on failure;
  - create: `tasks.create({ title })` plus `listId` for a list source,
    or `due: today` for Today (nothing more for All Tasks);
  - edit: `tasks.update({ id, ...edit })` with only the changed field;
  - delete: `tasks.delete({ id })`, selecting the next shown row (else
    the previous, else none) at once;
  - clear a flag: `message.setFlags({ ids: [id], changes: { flagColor: 0
    } })`;
  - open a message: `revealMessage(client, id)` and the module set to
    Mail.
  Failures are logged with `console.warn`, as elsewhere.
- **TasksModule** (`TasksModule.tsx`), laid out as `CalendarModule`: the
  sidebar column (a focusable `fieldset` named "Task Lists" holding
  `TasksSidebar` and the module bar, with Mail's width and splitter), the
  list (a `section` named "Task list", `flex-1`, holding `TaskList`), and
  the pane (a `section` named "Task details", 320 wide, holding
  `TaskPane`). Clicking, focusing or typing in a column sets
  `tasksFocus` (`sidebar`, `list`, `reader`). The list's title is the
  source's name ("Today", "All Tasks", "Flagged Mail" or the list's);
  its lines are `taskLines(sourceTasks(...))` with list names for Today
  and All Tasks; `messages` are the flagged rows for Flagged Mail, else
  null; `canCreate` is false for Flagged Mail and a read-only list. The
  pane shows the selected task (or message in Flagged Mail) with its
  list's name and account email. Selecting a sidebar row calls
  `setTasksSource`. It exposes a handle with `focus(pane)`,
  `focusNewTask()` and `focusTitle()`.
- **Keys** (`MainWindow`): Ctrl+4 (`showTasks`) shows Tasks from any
  module. In Tasks, with `tasksFocus` `list`: `previous`/`next` move the
  selection by a row (with none, `next` takes the first), `first`/`last`
  to the ends, `pageDown` (Space) toggles the selected task or clears the
  selected message's flag, `delete` deletes the selected task (never a
  message), `open` (Enter) focuses the pane's Title (in Flagged Mail,
  opens the message in Mail). From any pane: `escape` clears the
  selection, `compose` (Ctrl+N) focuses the New Task field when it shows,
  `nextPane`/`previousPane` cycle the panes as in Calendar, and
  `focusSearch` does nothing. Other commands are not handled.
- **Toolbar** (`ToolbarContainer`): in Tasks, `mode="tasks"` with
  `tasks={{ title, showCompleted, paneWidth: 320 }}`, where
  `showCompleted` is `tasksShowCompleted` for a list or All Tasks and
  null for Today and Flagged Mail; `newTask` focuses the New Task field
  (through a callback from `MainWindow`), `toggleCompleted` calls
  `toggleShowCompleted`. In Mail, `todoBar={ui.todoBar}`, and
  `toggleTodoBar` calls `toggleTodoBar`.

## Tests (given, do not edit)

`app/src/app/TasksModule.test.tsx` drives `MainWindow` over MockTransport
with the fixture's task lists (IDs 401–408, dates relative to now). The
other app tests must keep passing.

## Gotchas

- `revealMessage` does not change the module; set it to Mail yourself.
- The flagged view is a `ViewModel`; read its rows with `row(i)` for
  `i < count`, re-rendering on its changes (`useSyncExternalStore`, as
  `useView` does).
- A task deleted elsewhere disappears on refetch: keep the selection
  only while its ID is shown, else clear it.
- Keep each file under ~250 lines; split helpers into the touched files.

## Out of scope

The To-Do bar's pane (T-0086), the components (T-0083), the stores,
toolbar and mock (T-0084), and every file not under `touch`.

## Done when

`make accept T=0085` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
