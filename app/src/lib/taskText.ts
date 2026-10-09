// Due text, sources and the task list's lines (docs/specs/pim-ui.md, Tasks
// module and To-Do bar).
import type { Task } from "../rpc/gen/api";
import { addDays } from "./calendarDates";

/** TasksSource is what the task list shows: a smart list or a list's ID. */
export type TasksSource = "today" | "all" | "flagged" | number;

/** TaskLine is one line of the task list. */
export type TaskLine = { kind: "header"; listId: number; name: string } | { kind: "task"; task: Task; level: 0 | 1 };

/** SourceOptions decide which completed tasks a source keeps. */
export interface SourceOptions {
  /** Today, YYYY-MM-DD, in the app's zone. */
  today: string;
  /** Show Completed is on (it applies to lists and All Tasks). */
  showCompleted: boolean;
  /** Tasks ticked here since the source was chosen: they stay. */
  ticked: ReadonlySet<number>;
}

/**
 * dueText names a due date from today: "Today", "Tomorrow", "Yesterday",
 * "Fri, Oct 9" in today's year, else "Oct 9, 2027"; "" for no date.
 */
export function dueText(due: string, today: string, locale: string): string {
  if (!due) return "";
  if (due === today) return "Today";
  if (due === addDays(today, 1)) return "Tomorrow";
  if (due === addDays(today, -1)) return "Yesterday";
  const options: Intl.DateTimeFormatOptions =
    due.slice(0, 4) === today.slice(0, 4)
      ? { weekday: "short", month: "short", day: "numeric" }
      : { month: "short", day: "numeric", year: "numeric" };
  return new Intl.DateTimeFormat(locale, { ...options, timeZone: "UTC" }).format(new Date(due));
}

/** isOverdue tells whether an open task was due before today. */
export function isOverdue(task: Pick<Task, "due" | "completed">, today: string): boolean {
  return !task.completed && task.due !== "" && task.due < today;
}

/**
 * sourceTasks picks a source's tasks from every task, keeping their order:
 * Today's are due today or earlier, All Tasks' every task, a list's its
 * own; Flagged Mail has none. Completed tasks stay out unless ticked here,
 * or Show Completed is on for a list or All Tasks.
 */
export function sourceTasks(all: readonly Task[], source: TasksSource, opts: SourceOptions): Task[] {
  if (source === "flagged") return [];
  return all.filter((task) => {
    const belongs =
      source === "all" || (source === "today" ? task.due !== "" && task.due <= opts.today : task.listId === source);
    return belongs && (!task.completed || opts.ticked.has(task.id) || (opts.showCompleted && source !== "today"));
  });
}

/** openCount counts a source's open tasks; Flagged Mail's is 0 here. */
export function openCount(all: readonly Task[], source: TasksSource, today: string): number {
  return sourceTasks(all, source, { today, showCompleted: false, ticked: new Set() }).length;
}

/**
 * taskLines lays tasks out in their order: a subtask is at level 1 when
 * its parent is shown before it, else at 0. With names (list ID to name),
 * a header comes before each list's first task.
 */
export function taskLines(tasks: readonly Task[], names?: ReadonlyMap<number, string>): TaskLine[] {
  const lines: TaskLine[] = [];
  const seen = new Set<number>();
  let previous: number | undefined;
  for (const task of tasks) {
    if (names && task.listId !== previous)
      lines.push({ kind: "header", listId: task.listId, name: names.get(task.listId) ?? "" });
    lines.push({ kind: "task", task, level: task.parentId !== undefined && seen.has(task.parentId) ? 1 : 0 });
    seen.add(task.id);
    previous = task.listId;
  }
  return lines;
}

/**
 * dueSoon is the To-Do bar's tasks: open ones due within seven days of
 * today or earlier, by due date and then their order, at most limit.
 */
export function dueSoon(all: readonly Task[], today: string, limit: number): Task[] {
  const end = addDays(today, 7);
  return all
    .filter((task) => !task.completed && task.due !== "" && task.due <= end)
    .sort((a, b) => (a.due < b.due ? -1 : a.due > b.due ? 1 : 0))
    .slice(0, Math.max(0, limit));
}
