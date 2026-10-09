// The Tasks module's detail pane (docs/specs/pim-ui.md, Task pane). Task
// T-0083 builds it.
import type { Ref } from "react";

import type { MessageSummary, Task } from "../../rpc/gen/api";

/** TaskEdit is one change the pane commits. */
export interface TaskEdit {
  title?: string;
  notes?: string;
  /** YYYY-MM-DD, or "" to clear. */
  due?: string;
}

/** TaskPaneProps are the task pane's inputs. */
export interface TaskPaneProps {
  task: Task | null;
  /** The selected flagged message, in Flagged Mail. */
  message: MessageSummary | null;
  /** The task's list and its account's email. */
  list: { name: string; account: string } | null;
  timeZone: string;
  locale: string;
  /** Now, for the message's date. */
  now: Date;
  /** The Title field, which the container focuses for Enter. */
  titleRef?: Ref<HTMLInputElement>;
  onChange: (id: number, edit: TaskEdit) => void;
  onDelete: (id: number) => void;
  onOpenMessage: (messageId: number) => void;
  onClearFlag: (messageId: number) => void;
}

/** TaskPane shows and edits the selected task, or the flagged message. */
export function TaskPane(_props: TaskPaneProps) {
  return null;
}
