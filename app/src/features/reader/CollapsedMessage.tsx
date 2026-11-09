// A collapsed message of a conversation (docs/specs/ui.md, Reader,
// Conversation; parity P-310): one row with the sender, the preview and the
// date, which expands the message on a click, Enter or Space.
import { avatarTone, displayName, formatHeaderDate, formatListDate, initials } from "../../lib/format";
import type { MessageSummary } from "../../rpc/gen/api";
import { AVATAR_CLASSES } from "./MessageHeader";

/** CollapsedMessageProps are a collapsed row's inputs. */
export interface CollapsedMessageProps {
  summary: MessageSummary;
  /** now dates the row in the list's short form. */
  now: Date;
  onExpand: () => void;
}

/** CollapsedMessage shows a message of a conversation as 1 row. */
export function CollapsedMessage({ summary, now, onExpand }: CollapsedMessageProps) {
  const date = new Date(summary.date);
  const name = displayName(summary.from);
  // A native button: Enter and Space click it, as a click does. Find in the
  // message skips it: a collapsed row's preview is not part of what the
  // reader shows of the message (docs/specs/ui.md, Find in the message).
  return (
    <button
      type="button"
      data-find-skip
      aria-expanded={false}
      aria-label={`${name}, ${formatHeaderDate(date)}`}
      title={summary.from.address}
      onClick={onExpand}
      className="flex min-h-11 w-full items-center gap-2 px-5 py-2.5 text-left"
    >
      <span
        aria-hidden="true"
        className={`flex size-6 shrink-0 items-center justify-center rounded-full text-[10px] font-semibold text-white ${
          AVATAR_CLASSES[avatarTone(summary.from.address)] ?? ""
        }`}
      >
        {initials(summary.from)}
      </span>
      <span className="text-list-sender max-w-[40%] shrink-0 truncate">{name}</span>
      <span className="text-list-preview min-w-0 flex-1 truncate text-secondary">{summary.preview}</span>
      <span className="text-list-date shrink-0 tabular-nums text-secondary">{formatListDate(date, now)}</span>
    </button>
  );
}
