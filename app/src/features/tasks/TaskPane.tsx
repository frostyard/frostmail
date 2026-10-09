// Editable task details and the flagged message's actions.
import { Lock, X } from "lucide-react";
import { type ReactNode, type Ref, useState } from "react";
import { displayName, formatHeaderDate } from "../../lib/format";
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

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="flex min-h-8 items-center justify-end text-[12px] leading-4 text-secondary">{label}</dt>
      <dd className="flex min-h-8 min-w-0 items-center text-[13px] leading-[18px]">{children}</dd>
    </>
  );
}

function DraftTitle({ task, titleRef, onChange }: Pick<TaskPaneProps, "titleRef" | "onChange"> & { task: Task }) {
  const [draft, setDraft] = useState(task.title);
  const commit = (input: HTMLInputElement) => {
    if (task.readOnly) return;
    const title = input.value.trim() || task.title;
    input.value = title;
    setDraft(title);
    if (title !== task.title) onChange(task.id, { title });
  };
  return (
    <input
      ref={titleRef}
      aria-label="Title"
      readOnly={task.readOnly}
      value={draft}
      className="w-full rounded-md border border-transparent bg-transparent text-[17px] font-semibold leading-[22px] hover:border-separator focus:border-separator"
      onChange={(event) => setDraft(event.currentTarget.value)}
      onBlur={(event) => commit(event.currentTarget)}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          event.preventDefault();
          commit(event.currentTarget);
        } else if (event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          event.currentTarget.value = task.title;
          setDraft(task.title);
          event.currentTarget.blur();
        }
      }}
    />
  );
}

function DraftNotes({ task, onChange }: { task: Task; onChange: TaskPaneProps["onChange"] }) {
  const [draft, setDraft] = useState(task.notes);
  return (
    <textarea
      aria-label="Notes"
      readOnly={task.readOnly}
      value={draft}
      className="min-h-[120px] w-full rounded-md border border-separator bg-transparent p-2 text-[13px] leading-[18px]"
      onChange={(event) => setDraft(event.currentTarget.value)}
      onBlur={(event) => {
        if (!task.readOnly && event.currentTarget.value !== task.notes)
          onChange(task.id, { notes: event.currentTarget.value });
      }}
    />
  );
}

/** TaskPane shows and edits the selected task, or the flagged message. */
export function TaskPane(props: TaskPaneProps) {
  const { task, message, list, timeZone, locale, titleRef, onChange, onDelete, onOpenMessage, onClearFlag } = props;
  if (!task && !message)
    return (
      <div className="flex h-full items-center justify-center bg-window p-5 text-[13px] leading-4 text-tertiary">
        No Task Selected
      </div>
    );
  if (!task && message)
    return (
      <div className="h-full overflow-y-auto bg-window p-5">
        <h2 className="text-[17px] font-semibold leading-[22px]">{message.subject || "No Subject"}</h2>
        <div className="text-[13px] leading-[18px] text-secondary">
          {displayName(message.from)} · {formatHeaderDate(new Date(message.date), locale)}
        </div>
        <div className="mt-5 flex gap-3 text-accent">
          <button type="button" onClick={() => onOpenMessage(message.id)}>
            Open in Mail
          </button>
          <button type="button" onClick={() => onClearFlag(message.id)}>
            Clear Flag
          </button>
        </div>
      </div>
    );
  if (!task) return null;
  return (
    <div className="h-full overflow-y-auto bg-window p-5">
      <DraftTitle key={JSON.stringify([task.id, task.title])} task={task} titleRef={titleRef} onChange={onChange} />
      {task.readOnly && (
        <div className="mt-1 flex items-center gap-1 text-[12px] leading-4 text-tertiary">
          <Lock size={12} aria-hidden="true" />
          <span>Read-only</span>
        </div>
      )}
      <dl className="my-5 grid grid-cols-[80px_minmax(0,1fr)] gap-x-3">
        <Field label="Due">
          <input
            type="date"
            aria-label="Due"
            disabled={task.readOnly}
            value={task.due}
            className="min-w-0 bg-transparent"
            onChange={(event) => {
              if (!task.readOnly) onChange(task.id, { due: event.currentTarget.value });
            }}
          />
          {task.due && !task.readOnly && (
            <button
              type="button"
              aria-label="Clear Due Date"
              className="ml-1 text-secondary"
              onClick={() => onChange(task.id, { due: "" })}
            >
              <X size={14} aria-hidden="true" />
            </button>
          )}
        </Field>
        {list && (
          <Field label="List">
            <span className="truncate" title={`${list.name} · ${list.account}`}>
              {list.name}
              <span className="text-tertiary"> · {list.account}</span>
            </span>
          </Field>
        )}
        {task.completed && (
          <Field label="Completed">
            {task.completedAt
              ? new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short", timeZone }).format(
                  new Date(task.completedAt),
                )
              : ""}
          </Field>
        )}
        {task.messageId !== undefined && (
          <Field label="From mail">
            <button
              type="button"
              className="text-accent"
              onClick={() => {
                if (task.messageId !== undefined) onOpenMessage(task.messageId);
              }}
            >
              Open Message
            </button>
          </Field>
        )}
      </dl>
      <h3 className="mb-2 text-[12px] leading-4 text-secondary">Notes</h3>
      <DraftNotes key={JSON.stringify([task.id, task.notes])} task={task} onChange={onChange} />
      {!task.readOnly && (
        <button type="button" className="mt-5 text-[13px] leading-4 text-flag-1" onClick={() => onDelete(task.id)}>
          Delete Task
        </button>
      )}
    </div>
  );
}
