// Tasks and flagged messages, with completion circles and focus-sensitive selection.
import { Check, Flag, Mail, Plus } from "lucide-react";
import { type ReactNode, type Ref, useEffect, useRef } from "react";
import { flagName } from "../../lib/flags";
import { displayName, formatListDate } from "../../lib/format";
import { dueText, isOverdue, type TaskLine } from "../../lib/taskText";
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

const FLAG_CLASSES: Record<number, string> = {
  1: "text-flag-1",
  2: "text-flag-2",
  3: "text-flag-3",
  4: "text-flag-4",
  5: "text-flag-5",
  6: "text-flag-6",
  7: "text-flag-7",
};

function Row({
  id,
  completed = false,
  readOnly = false,
  level = 0,
  title,
  detail,
  icon,
  props,
}: {
  id: number;
  completed?: boolean;
  readOnly?: boolean;
  level?: 0 | 1;
  title: string;
  detail: ReactNode;
  icon?: ReactNode;
  props: TaskListProps;
}) {
  const selected = id === props.selected;
  const contrast = selected && props.focused;
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (selected) ref.current?.scrollIntoView?.({ block: "nearest" });
  }, [selected]);
  return (
    <div
      ref={ref}
      role="option"
      tabIndex={-1}
      data-id={id}
      aria-selected={selected}
      style={{ paddingLeft: level === 1 ? "40px" : "12px" }}
      className={`relative flex min-h-9 items-start gap-[10px] py-2 pr-3 ${selected ? (contrast ? "bg-accent text-accent-contrast" : "bg-selection-inactive") : ""}`}
      onClick={() => props.onSelect(id)}
      onKeyDown={(event) => {
        if (event.target === event.currentTarget && (event.key === "Enter" || event.key === " ")) {
          event.preventDefault();
          props.onSelect(id);
        }
      }}
    >
      {/* biome-ignore lint/a11y/useSemanticElements: the task contract requires a button with checkbox semantics, matching CalendarSidebar. */}
      <button
        type="button"
        role="checkbox"
        aria-label="Completed"
        aria-checked={completed}
        disabled={readOnly}
        className={`flex size-[18px] shrink-0 items-center justify-center rounded-full border-[1.5px] ${completed ? "border-accent bg-accent text-accent-contrast" : contrast ? "border-accent-contrast" : "border-tertiary"}`}
        onClick={(event) => {
          event.stopPropagation();
          props.onToggle(id, !completed);
        }}
      >
        {completed && <Check size={12} aria-hidden="true" />}
      </button>
      <div className="min-w-0 flex-1">
        <div className={`truncate text-list-subject ${completed ? "text-tertiary" : ""}`}>{title}</div>
        {detail && (
          <div className={`truncate text-list-preview ${contrast ? "text-accent-contrast" : "text-secondary"}`}>
            {detail}
          </div>
        )}
      </div>
      {icon}
      {!selected && <div className="absolute bottom-0 left-10 right-0 h-px bg-separator" />}
    </div>
  );
}

/** TaskList shows a source's tasks or flagged messages. */
export function TaskList(props: TaskListProps) {
  const { title, lines, messages, canCreate, newTaskRef, today, now, locale, onCreate } = props;
  const box = useRef<HTMLDivElement>(null);
  return (
    <div
      ref={box}
      role="listbox"
      aria-label={title}
      tabIndex={0}
      className="flex h-full flex-col overflow-y-auto bg-window outline-none"
    >
      {canCreate && (
        <div className="flex h-10 shrink-0 items-center gap-3 border-b border-separator px-3">
          <Plus size={16} aria-hidden="true" className="text-tertiary" />
          <input
            ref={newTaskRef}
            aria-label="New Task"
            placeholder="New Task"
            className="min-w-0 flex-1 border-0 bg-transparent text-list-subject outline-none"
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                event.stopPropagation();
                const value = event.currentTarget.value.trim();
                if (value) onCreate(value);
                event.currentTarget.value = "";
              } else if (event.key === "Escape") {
                event.preventDefault();
                event.stopPropagation();
                event.currentTarget.value = "";
                box.current?.focus();
              }
            }}
          />
        </div>
      )}
      {messages !== null
        ? messages.map((message) => (
            <Row
              key={message.id}
              id={message.id}
              props={props}
              title={message.subject || "No Subject"}
              detail={
                <>
                  {displayName(message.from)} · {formatListDate(new Date(message.date), now, locale)}
                </>
              }
              icon={
                <Flag
                  size={14}
                  aria-label={`Flagged ${flagName(message.flags.flagColor)}`}
                  className={`shrink-0 fill-current ${FLAG_CLASSES[message.flags.flagColor] ?? ""}`}
                />
              }
            />
          ))
        : lines.map((line, index) => {
            const next = lines[index + 1];
            if (line.kind === "header")
              return (
                <div
                  key={`header:${line.listId}:${next?.kind === "task" ? next.task.id : line.name}`}
                  role="presentation"
                  data-list={line.listId}
                  className="flex h-7 shrink-0 items-center pl-3 text-sidebar-section text-secondary"
                >
                  {line.name}
                </div>
              );
            const { task, level } = line;
            const notes = task.notes.split(/\r?\n/)[0] ?? "";
            return (
              <Row
                key={task.id}
                id={task.id}
                props={props}
                level={level}
                completed={task.completed}
                readOnly={task.readOnly}
                title={task.title}
                detail={
                  task.due || notes ? (
                    <>
                      {task.due && (
                        <span className={isOverdue(task, today) ? "text-flag-1" : ""}>
                          {dueText(task.due, today, locale)}
                        </span>
                      )}
                      {task.due && notes ? " · " : ""}
                      {notes}
                    </>
                  ) : null
                }
                icon={
                  task.messageId ? (
                    <Mail
                      size={14}
                      aria-label="From mail"
                      className={`shrink-0 ${task.id === props.selected && props.focused ? "text-accent-contrast" : "text-secondary"}`}
                    />
                  ) : null
                }
              />
            );
          })}
      {(messages !== null ? messages.length === 0 : lines.length === 0) && (
        <div className="flex flex-1 items-center justify-center text-empty text-secondary">
          {messages !== null ? "No Flagged Mail" : "No Tasks"}
        </div>
      )}
    </div>
  );
}
