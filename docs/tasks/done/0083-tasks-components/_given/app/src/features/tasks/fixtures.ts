// Shared data for the Tasks components' tests (task card T-0083).
import type { MessageSummary, Task } from "../../rpc/gen/api";

/** TODAY is the tests' today, a Friday. */
export const TODAY = "2026-10-09";

/** NOW is the tests' now, in TODAY (tests run in UTC). */
export const NOW = new Date("2026-10-09T15:00:00Z");

/** task builds a task in list 10 of account 1. */
export function task(over: Partial<Task> & Pick<Task, "id" | "title">): Task {
  return { listId: 10, accountId: 1, notes: "", due: "", completed: false, readOnly: false, ...over };
}

/** TASKS are three lists' tasks in maild's order. */
export const TASKS: Task[] = [
  task({ id: 1, title: "Report", due: "2026-10-10", notes: "Q3 numbers\nand charts" }),
  task({ id: 2, title: "Charts", parentId: 1 }),
  task({ id: 3, title: "Groceries", due: "2026-10-08", messageId: 77 }),
  task({ id: 4, title: "Done", completed: true, completedAt: "2026-10-09T19:04:00Z", due: "2026-10-01" }),
  task({ id: 5, title: "Paint", listId: 20, accountId: 2, due: "2026-10-09" }),
  task({ id: 6, title: "Shared", listId: 21, accountId: 2, readOnly: true }),
];

/** message builds a flagged message summary. */
export function message(over: Partial<MessageSummary> & Pick<MessageSummary, "id" | "subject">): MessageSummary {
  return {
    accountId: 1,
    mailboxIds: [1],
    threadId: over.id,
    from: { name: "Ada Lovelace", address: "ada@example.com" },
    date: "2026-10-08T10:00:00Z",
    preview: "",
    flags: { seen: true, flagged: true, answered: false, forwarded: false, draft: false, flagColor: 1 },
    hasAttachments: false,
    size: 1000,
    threadCount: 1,
    ...over,
  };
}

/** norm replaces the thin and narrow no-break spaces Intl uses with spaces. */
export const norm = (s: string | null | undefined) => (s ?? "").replace(/[   ]/g, " ");
