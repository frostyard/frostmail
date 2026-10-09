// The Tasks module's sidebar (docs/specs/pim-ui.md, Tasks sidebar). Task
// T-0083 builds it.
import type { TasksSource } from "../../lib/taskText";

/** TaskListRow is one task list in the sidebar. */
export interface TaskListRow {
  id: number;
  name: string;
  readOnly: boolean;
  /** Its open tasks. */
  count: number;
}

/** TaskListSection is an account and its task lists. */
export interface TaskListSection {
  accountId: number;
  title: string;
  lists: TaskListRow[];
}

/** TasksSidebarProps are the Tasks sidebar's inputs. */
export interface TasksSidebarProps {
  sections: TaskListSection[];
  /** The smart lists' counts: open tasks due by today, every open task, flagged messages. */
  counts: { today: number; all: number; flagged: number };
  selected: TasksSource;
  /** The sidebar has keyboard focus: the selection uses the accent color. */
  focused: boolean;
  onSelect: (source: TasksSource) => void;
}

/** TasksSidebar shows the smart lists and each account's task lists. */
export function TasksSidebar(_props: TasksSidebarProps) {
  return null;
}
