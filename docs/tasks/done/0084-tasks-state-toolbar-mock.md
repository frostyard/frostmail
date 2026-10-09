---
id: "0084"
title: Tasks state, toolbar and mock
milestone: M4.5
size: L
touch:
  - app/src/data/stores.ts
  - app/src/features/toolbar/Toolbar.tsx
  - app/src/rpc/mock/tasks.ts
  - app/src/rpc/mock/mock.ts
  - app/src/rpc/mock/fixture.ts
given:
  - app/src/data/stores.tasks.test.ts
  - app/src/features/toolbar/Toolbar.tasks.test.tsx
  - app/src/rpc/mock/tasks.test.ts
  - app/src/rpc/mock/calendar.test.ts
acceptance: make ui-vitest F="src/data src/features/toolbar src/rpc/mock"
---
# T-0084: Tasks state, toolbar and mock

## Goal

What the Tasks module and the To-Do bar need besides their components:
the window state, the toolbar's Tasks mode and To-Do Bar button, and
MockTransport's tasks domain with fixture task lists, so T-0085 can wire
the module against the mock.

## Read first

- `docs/specs/pim-ui.md`: "Tasks module" ("Tasks toolbar", "Tasks
  behavior") and "To-Do bar".
- `docs/design/pim.md`, "Tasks"; `schema/rpc/tasks.yaml` (the API) and
  `internal/engine/tasks.go` (maild's behavior, which the mock copies).
- `app/src/data/stores.ts` (the calendar and people state to mirror, and
  `partialize`), `app/src/features/toolbar/Toolbar.tsx` (Calendar's mode
  to mirror), `app/src/rpc/mock/calendar.ts` and `people.ts` (how a
  domain plugs into `mock.ts`, `RPCError`, `NOT_HANDLED`, the shared
  collections), `app/src/rpc/mock/fixture.ts`.
- The stub `app/src/rpc/mock/tasks.ts`: keep its exported names.
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

- **useUI** (`stores.ts`), beside the calendar and people state:
  `tasksSource: TasksSource` (from `lib/taskText.ts`; first `"today"`),
  `tasksSelected: number | null` (null), `tasksFocus: Pane` (`"list"`),
  `tasksShowCompleted: boolean` (false), `todoBar: boolean` (false), and
  the actions `setTasksSource(source)` (also clears `tasksSelected`),
  `selectTask(id | null)`, `setTasksFocus(pane)`,
  `toggleShowCompleted()` and `toggleTodoBar()`. `todoBar` joins the
  persisted part (`partialize`); the rest is window state.
- **Toolbar:**
  - `mode` gains `"tasks"`, with `tasks?: ToolbarTasks` (export it:
    `{ title: string; showCompleted: boolean | null; paneWidth: number
    }`; null hides Show Completed). `ToolbarCommand` gains `{ kind:
    "newTask" }`, `{ kind: "toggleCompleted" }` and `{ kind:
    "toggleTodoBar" }`. `ToolbarProps` gains `todoBar?: boolean`.
  - In Tasks the segments are `sidebar`, `tasks` and `pane` (all
    `data-tauri-drag-region`), laid out as Calendar's (the pane segment
    `paneWidth + 1` px wide). The `tasks` segment: a `ToolbarButton`
    "New Task" (`Plus`, shortcut "Ctrl+N", so its title is "New Task
    (Ctrl+N)") sending `newTask`, the title (15/20 600, truncated,
    draggable), and at the right, unless `showCompleted` is null, a text
    button "Show Completed" (28 high, 10px padding, 13px, `aria-pressed`
    = `showCompleted`) sending `toggleCompleted`. The pane segment has
    Settings and the window controls, no search and no Mail actions.
  - In Mail only, the reader segment gets a `ToolbarButton` "To-Do Bar"
    (`PanelRight`) with `aria-pressed` = `todoBar ?? false`, before the
    search field, sending `toggleTodoBar`.
- **MockTasks** (`rpc/mock/tasks.ts`), as maild's tasks domain over the
  shared collections (kind `tasklist`):
  - `tasks.list`: tasks of enabled lists (with `listId`, that list
    whether enabled or not; unknown or not a task list: `notFound`), list
    by list in collection order, each list's in stored order; completed
    ones only with `completed: true`; with `dueBefore` (a valid
    `YYYY-MM-DD`, else `invalidParams`) only tasks with a due date before
    it. `readOnly` is the list's.
  - `tasks.create`: the title trimmed (empty: `invalidParams`); `due`
    a valid date when given (`invalidParams`); the list given, else the
    default (`isDefault`) list, else the first task list; unknown:
    `notFound`, read-only: `conflict`; a `parentId` that does not exist:
    `notFound`, that is itself a subtask or in another list:
    `invalidParams`. The new task gets the next ID above every task's,
    goes first in its list, or right after its parent, and
    `tasks.changed` (`{ accountId }`) is emitted.
  - `tasks.update`: unknown: `notFound`; read-only: `conflict`; a title
    that trims to empty or a due date that is neither "" nor valid:
    `invalidParams`. Sets the given fields (the title trimmed);
    completing stamps `completedAt` with `data.now`, reopening removes
    it. Emits `tasks.changed`.
  - `tasks.delete`: unknown: `notFound`; read-only: `conflict`; removes
    the task and its subtasks; emits `tasks.changed`.
  - `mock.ts` constructs it with `data.tasks` (absent: no tasks), the
    shared collections and its `emit`, and dispatches to it beside the
    calendar and people domains. `MockData` gains `tasks?:
    MockTasksData`.
- **Fixture** (`fixture.ts`): export `TASK_LISTS = { tasks: 301,
  errands: 302, team: 303 }`, three `tasklist` collections of the
  fixture account after the calendars ("Tasks", the default; "Errands";
  "Team", read-only; color "", `tasks` true, `events` false, enabled),
  and these tasks, D being `now`'s UTC day, IDs from 401 in this order:
  - Tasks: "Send the Q3 report" (due D+1, notes "Numbers from
    Maria\nCharts in the shared folder"); "Draft the charts" (its
    subtask); "Renew the passport" (due D−2); "Reply about the offsite"
    (due D, `messageId` the flagged "Re: Offsite plan" message in the
    Inbox); "Book flights" (due D−1, completed at D−1 10:00 UTC).
  - Errands: "Pick up the dry cleaning" (due D); "Buy oat milk".
  - Team: "Quarterly planning" (due D+5).
  - `now` for the mock is the fixture's `now` as an ISO string.

## Tests (given, do not edit)

`app/src/data/stores.tasks.test.ts`,
`app/src/features/toolbar/Toolbar.tasks.test.tsx`,
`app/src/rpc/mock/tasks.test.ts`, and `app/src/rpc/mock/calendar.test.ts`
updated for the three more collections.

## Gotchas

- `account.setCollection` (in `calendar.ts`, not yours) already emits
  `tasks.changed` for a task list.
- The other domains' tests must keep passing: the Calendar toolbar's
  layout is shared with Tasks, so generalize, don't fork.
- Copy tasks when answering (`{ ...t }`); never hand out the stored
  objects.
- `noUncheckedIndexedAccess` is on; keep functions short.

## Out of scope

The Tasks components (T-0083), the module's containers and keys
(T-0085), the To-Do bar (T-0086), and every file not under `touch`.

## Done when

`make accept T=0084` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
