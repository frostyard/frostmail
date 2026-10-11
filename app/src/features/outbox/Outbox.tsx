// Sending feedback in the main window: the undo toast for a message waiting
// out its undo delay, and the outbox section for messages not yet sent
// (docs/specs/compose-ui.md, Undo toast and Outbox).
import { formatAddressList } from "../../lib/format";
import { whenText } from "../../lib/later";
import type { OutboxItem } from "../../rpc/gen/api";

/** subjectShown is the subject, or "(no subject)" when it is blank. */
function subjectShown(subject: string): string {
  return subject.trim() === "" ? "(no subject)" : subject;
}

/** UndoToastProps are the undo toast's inputs. */
export interface UndoToastProps {
  /** The message's subject; "(no subject)" shows when it is empty. */
  subject: string;
  /** Whole seconds until the message goes out; at 0 or less Undo is gone. */
  secondsLeft: number;
  onUndo: () => void;
}

/** UndoToast offers to cancel a message that is about to be sent. */
export function UndoToast({ subject, secondsLeft, onUndo }: UndoToastProps) {
  return (
    <output className="flex h-9 items-center gap-3 rounded-lg border border-separator bg-toolbar px-3 text-[13px] leading-[18px] shadow-md">
      <span className="min-w-0 max-w-[320px] truncate">{`Sending “${subjectShown(subject)}”…`}</span>
      {secondsLeft > 0 && (
        <>
          <button type="button" className="font-semibold text-accent" onClick={onUndo}>
            Undo
          </button>
          <span className="text-[12px] leading-4 tabular-nums text-secondary">{`${secondsLeft}s`}</span>
        </>
      )}
    </output>
  );
}

/** retryMinutes is the whole minutes from now to sendAt, rounded up, at least 1. */
function retryMinutes(sendAt: string | undefined, now: Date): number {
  const parsed = sendAt === undefined ? Number.NaN : Date.parse(sendAt);
  const target = Number.isNaN(parsed) ? now.getTime() : parsed;
  return Math.max(1, Math.ceil((target - now.getTime()) / 60_000));
}

/** stateLine describes where an unsent message is on its way out. */
function stateLine(item: OutboxItem, now: Date): string {
  if (item.state === "sending") {
    return "Sending…";
  }
  if (item.state === "accepted") {
    return "Saving to Sent…";
  }
  if (item.state === "failed") {
    return item.error ? `Not sent: ${item.error}` : "Not sent";
  }
  if (item.attempts === 0) {
    return "Waiting to send";
  }
  const minutes = retryMinutes(item.sendAt, now);
  return item.error ? `Retrying in ${minutes} min: ${item.error}` : `Retrying in ${minutes} min`;
}

/** OutboxStatusProps are the outbox section's inputs. */
export interface OutboxStatusProps {
  items: OutboxItem[];
  /** The current time, for "Retrying in N min". */
  now: Date;
  onRetry: (id: number) => void;
  onEdit: (item: OutboxItem) => void;
}

/** OutboxStatus lists the messages that are not yet sent, with their state and what can be done. */
export function OutboxStatus({ items, now, onRetry, onEdit }: OutboxStatusProps) {
  const pending = items.filter((item) => item.state !== "sent" && !isSendLater(item));
  if (pending.length === 0) {
    return null;
  }
  return (
    <section aria-label="Outbox">
      <h2 className="px-4 pt-3 pb-1 text-sidebar-section text-secondary">Outbox</h2>
      <ul>
        {pending.map((item) => (
          <li key={item.id} className="flex flex-col px-4 py-2">
            <span className="truncate text-[13px] leading-[18px] font-semibold">{subjectShown(item.subject)}</span>
            <span className="truncate text-[12px] leading-4 text-secondary">{`To: ${formatAddressList(item.to, 2)}`}</span>
            <span className={`text-[12px] leading-4 ${item.state === "failed" ? "text-flag-1" : "text-secondary"}`}>
              {stateLine(item, now)}
            </span>
            {(item.state === "queued" || item.state === "failed") && (
              <span className="flex gap-3">
                {item.state === "failed" && (
                  <button type="button" className="text-[12px] leading-4 text-accent" onClick={() => onRetry(item.id)}>
                    Retry
                  </button>
                )}
                <button type="button" className="text-[12px] leading-4 text-accent" onClick={() => onEdit(item)}>
                  Edit
                </button>
              </span>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

/** isSendLater identifies a message queued for its chosen Send Later time. */
export function isSendLater(item: OutboxItem): boolean {
  return item.state === "queued" && item.scheduled;
}

/** SendLaterStatusProps supplies the waiting messages, local time and actions. */
export interface SendLaterStatusProps {
  items: OutboxItem[];
  now: Date;
  timeZone: string;
  locale: string;
  onEdit: (item: OutboxItem) => void;
  onSendNow: (item: OutboxItem) => void;
  onChangeTime: (item: OutboxItem) => void;
}

/** SendLaterStatus lists scheduled messages soonest first, with their send actions. */
export function SendLaterStatus({
  items,
  now,
  timeZone,
  locale,
  onEdit,
  onSendNow,
  onChangeTime,
}: SendLaterStatusProps) {
  const waiting = items.filter(isSendLater).sort((a, b) => Date.parse(a.sendAt ?? "") - Date.parse(b.sendAt ?? ""));
  if (waiting.length === 0) return null;
  return (
    <section aria-label="Send Later">
      <h2 className="px-4 pt-3 pb-1 text-sidebar-section text-secondary">Send Later</h2>
      <ul>
        {waiting.map((item) => (
          <li key={item.id} className="flex flex-col px-4 py-2">
            <span className="truncate text-[13px] leading-[18px] font-semibold">{subjectShown(item.subject)}</span>
            <span className="truncate text-[12px] leading-4 text-secondary">{`To: ${formatAddressList(item.to, 2)}`}</span>
            <span className="text-[12px] leading-4 text-secondary">
              {`Sends ${whenText(new Date(item.sendAt ?? now.toISOString()), now, timeZone, locale)}`}
            </span>
            <span className="flex gap-3">
              <button type="button" className="text-[12px] leading-4 text-accent" onClick={() => onEdit(item)}>
                Edit
              </button>
              <button type="button" className="text-[12px] leading-4 text-accent" onClick={() => onSendNow(item)}>
                Send Now
              </button>
              <button type="button" className="text-[12px] leading-4 text-accent" onClick={() => onChangeTime(item)}>
                Change Time…
              </button>
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
