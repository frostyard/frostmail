// The optional Mail pane: small month, upcoming events, tasks and flagged mail.
import { Check, Flag, Plus } from "lucide-react";
import { flagName } from "../../lib/flags";
import { dueText, isOverdue } from "../../lib/taskText";
import type { MessageSummary, Task } from "../../rpc/gen/api";
import { MiniMonth, type MiniMonthProps } from "../calendar/MiniMonth";
import { UpcomingList, type UpcomingListProps } from "../calendar/UpcomingList";

/** ToDoBarProps are the pane's data and navigation and completion callbacks. */
export interface ToDoBarProps extends MiniMonthProps, UpcomingListProps {
  tasks: Task[];
  messages: MessageSummary[];
  onCreate: (title: string) => void;
  onToggle: (id: number, completed: boolean) => void;
  onClearFlag: (id: number) => void;
  onOpenTask: (task: Task) => void;
  onOpenMessage: (id: number) => void;
}

const flagClasses: Record<number, string> = {
  1: "text-flag-1",
  2: "text-flag-2",
  3: "text-flag-3",
  4: "text-flag-4",
  5: "text-flag-5",
  6: "text-flag-6",
  7: "text-flag-7",
};

function Completion({
  completed = false,
  disabled = false,
  onToggle,
}: {
  completed?: boolean;
  disabled?: boolean;
  onToggle: () => void;
}) {
  return (
    // biome-ignore lint/a11y/useSemanticElements: the card requires a button with checkbox semantics.
    <button
      type="button"
      role="checkbox"
      aria-label="Completed"
      aria-checked={completed}
      disabled={disabled}
      onClick={onToggle}
      className={`flex size-4 shrink-0 items-center justify-center rounded-full border-[1.5px] ${completed ? "border-accent bg-accent text-accent-contrast" : "border-tertiary"}`}
    >
      {completed && <Check size={12} aria-hidden="true" />}
    </button>
  );
}

/** ToDoBar presents the small month, coming events and due items beside Mail. */
export function ToDoBar(props: ToDoBarProps) {
  return (
    <aside
      aria-label="To-Do Bar"
      className="h-full w-[280px] shrink-0 overflow-y-auto border-l border-separator bg-sidebar"
    >
      <MiniMonth {...props} />
      <div className="mt-4 px-3">
        {props.occurrences.length ? (
          <UpcomingList {...props} />
        ) : (
          <>
            <h3 className="mb-2 text-sidebar-section text-secondary uppercase">Upcoming</h3>
            <p className="text-[12px] leading-4 text-tertiary">No Upcoming Events</p>
          </>
        )}
      </div>
      <h3 className="mt-4 px-3 text-sidebar-section text-secondary uppercase">Tasks</h3>
      <div className="flex h-8 items-center gap-[10px] px-3">
        <Plus size={16} aria-hidden="true" className="shrink-0 text-tertiary" />
        <input
          aria-label="New Task"
          placeholder="New Task"
          className="h-8 min-w-0 flex-1 border-0 bg-transparent text-[13px] leading-4 outline-none"
          onKeyDown={(event) => {
            if (event.key !== "Enter") return;
            event.preventDefault();
            event.stopPropagation();
            const title = event.currentTarget.value.trim();
            if (title) props.onCreate(title);
            event.currentTarget.value = "";
          }}
        />
      </div>
      <ul aria-label="Due">
        {props.tasks.map((task) => (
          <li key={task.id} data-kind="task" data-id={task.id} className="flex h-8 items-center gap-[10px] px-3">
            <Completion
              completed={task.completed}
              disabled={task.readOnly}
              onToggle={() => props.onToggle(task.id, !task.completed)}
            />
            <button
              type="button"
              className="min-w-0 flex-1 truncate text-left text-[13px] leading-4"
              onClick={() => props.onOpenTask(task)}
            >
              {task.title}
            </button>
            <span
              className={`shrink-0 text-[12px] leading-4 ${isOverdue(task, props.today) ? "text-flag-1" : "text-secondary"}`}
            >
              {dueText(task.due, props.today, props.locale)}
            </span>
          </li>
        ))}
        {props.messages.map((message) => (
          <li
            key={message.id}
            data-kind="message"
            data-id={message.id}
            className="flex h-8 items-center gap-[10px] px-3"
          >
            <Completion onToggle={() => props.onClearFlag(message.id)} />
            <button
              type="button"
              className="min-w-0 flex-1 truncate text-left text-[13px] leading-4"
              onClick={() => props.onOpenMessage(message.id)}
            >
              {message.subject || "No Subject"}
            </button>
            <Flag
              size={12}
              aria-label={`Flagged ${flagName(message.flags.flagColor)}`}
              className={`shrink-0 fill-current ${flagClasses[message.flags.flagColor] ?? ""}`}
            />
          </li>
        ))}
      </ul>
      {!props.tasks.length && !props.messages.length && (
        <p className="px-3 text-[12px] leading-4 text-tertiary">Nothing Due</p>
      )}
    </aside>
  );
}
