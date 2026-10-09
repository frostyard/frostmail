// Due text, sources and the task list's lines (docs/specs/pim-ui.md, Tasks
// module and To-Do bar). Task T-0083 builds it.
import type { Task } from "../rpc/gen/api";

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
export function dueText(_due: string, _today: string, _locale: string): string {
  throw new Error("Task T-0083 builds it");
}

/** isOverdue tells whether an open task was due before today. */
export function isOverdue(_task: Pick<Task, "due" | "completed">, _today: string): boolean {
  throw new Error("Task T-0083 builds it");
}

/**
 * sourceTasks picks a source's tasks from every task, keeping their order:
 * Today's are due today or earlier, All Tasks' every task, a list's its
 * own; Flagged Mail has none. Completed tasks stay out unless ticked here,
 * or Show Completed is on for a list or All Tasks.
 */
export function sourceTasks(_all: readonly Task[], _source: TasksSource, _opts: SourceOptions): Task[] {
  throw new Error("Task T-0083 builds it");
}

/** openCount counts a source's open tasks; Flagged Mail's is 0 here. */
export function openCount(_all: readonly Task[], _source: TasksSource, _today: string): number {
  throw new Error("Task T-0083 builds it");
}

/**
 * taskLines lays tasks out in their order: a subtask is at level 1 when
 * its parent is shown before it, else at 0. With names (list ID to name),
 * a header comes before each list's first task.
 */
export function taskLines(_tasks: readonly Task[], _names?: ReadonlyMap<number, string>): TaskLine[] {
  throw new Error("Task T-0083 builds it");
}

/**
 * dueSoon is the To-Do bar's tasks: open ones due within seven days of
 * today or earlier, by due date and then their order, at most limit.
 */
export function dueSoon(_all: readonly Task[], _today: string, _limit: number): Task[] {
  throw new Error("Task T-0083 builds it");
}
