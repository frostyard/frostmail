// The Tasks module's list: tasks with check circles, or flagged messages
// (docs/specs/pim-ui.md, Task list). Task T-0083 builds it.
import type { Ref } from "react";

import type { TaskLine } from "../../lib/taskText";
import type { MessageSummary } from "../../rpc/gen/api";

/** TaskListProps are the task list's inputs. */
export interface TaskListProps {
  /** The source's name: the listbox's name. */
  title: string;
  /** The tasks to show; ignored when messages is set. */
  lines: TaskLine[];
  /** Flagged Mail's messages, or null for a source of tasks. */
  messages: MessageSummary[] | null;
  /** The selected task's (or message's) ID. */
  selected: number | null;
  /** The list has keyboard focus: the selection uses the accent color. */
  focused: boolean;
  /** The New Task field shows. */
  canCreate: boolean;
  /** Today, YYYY-MM-DD, for the due text. */
  today: string;
  /** Now, for the messages' dates. */
  now: Date;
  locale: string;
  /** The New Task field, which the container focuses for Ctrl+N. */
  newTaskRef?: Ref<HTMLInputElement>;
  onSelect: (id: number) => void;
  /** Ticks or unticks a task; for a message, completed true clears its flag. */
  onToggle: (id: number, completed: boolean) => void;
  /** Creates a task with the trimmed title. */
  onCreate: (title: string) => void;
}

/** TaskList shows a source's tasks or flagged messages. */
export function TaskList(_props: TaskListProps) {
  return null;
}
