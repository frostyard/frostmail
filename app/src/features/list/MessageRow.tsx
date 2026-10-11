// One row of the message list (docs/specs/ui.md, Message list): the sender
// and the date on line 1, the subject and its thread count on line 2, a
// two-line preview, and the unread dot and the flag in the 24px gutter. The
// list container owns focus, selection and virtualization; the row only
// reports what the pointer did to it.
import { Archive, Flag, Paperclip, Star, Trash2 } from "lucide-react";
import type { MouseEvent } from "react";

import { flagName } from "../../lib/flags";
import { displayName, formatListDate } from "../../lib/format";
import type { MessageSummary } from "../../rpc/gen/api";

/** ROW_HEIGHT is the fixed height of a list row in pixels. */
export const ROW_HEIGHT = 84;

/** SelectMode says how a click changes the selection. */
export type SelectMode = "replace" | "toggle" | "range";

/** RowAction is a command on a row's message alone. */
export type RowAction = "flag" | "archive" | "delete";

/** MessageRowProps are a row's inputs. */
export interface MessageRowProps {
  message: MessageSummary;
  vip?: boolean;
  selected: boolean;
  /** The list has keyboard focus: selected rows use the accent color. */
  focused: boolean;
  /** Conversation mode: show the thread count badge when above 1. */
  showThreadCount: boolean;
  /** The clock dates are formatted against. */
  now: Date;
  onSelect: (id: number, mode: SelectMode) => void;
  onContextMenu: (id: number, x: number, y: number) => void;
  onAction?: (id: number, action: RowAction) => void;
  canArchive?: boolean;
}

// Tailwind only generates a class it finds whole in the source, so the seven
// flag colors are listed rather than built from the color number.
const FLAG_CLASSES: Record<number, string> = {
  1: "text-flag-1",
  2: "text-flag-2",
  3: "text-flag-3",
  4: "text-flag-4",
  5: "text-flag-5",
  6: "text-flag-6",
  7: "text-flag-7",
};

/** joinClasses drops the empty parts and joins the rest with single spaces. */
function joinClasses(...parts: Array<string | false | undefined>): string {
  return parts.filter((part) => part !== undefined && part !== false).join(" ");
}

/** clickMode is the selection mode a click on a row asks for. */
function clickMode(event: MouseEvent<HTMLDivElement>): SelectMode {
  if (event.ctrlKey || event.metaKey) {
    return "toggle";
  }
  return event.shiftKey ? "range" : "replace";
}

/** MessageRow renders one message summary as a fixed-height list row. */
export function MessageRow(props: MessageRowProps) {
  const { message, selected, focused, showThreadCount, now, onSelect, onContextMenu, onAction, canArchive } = props;
  // A selected row in a focused list is accent with accent-contrast content;
  // in an unfocused list it is gray. Every secondary, tertiary and accent
  // part inside takes the contrast color with it.
  const contrast = selected && focused;
  const secondary = contrast ? "text-accent-contrast" : "text-secondary";
  const subject = message.subject.trim();
  const actions = [
    { action: "flag", label: message.flags.flagged ? "Unflag" : "Flag", shortcut: "Ctrl+Shift+L", Icon: Flag },
    { action: "archive", label: "Archive", shortcut: "Ctrl+Alt+A", Icon: Archive },
    { action: "delete", label: "Delete", shortcut: "Delete", Icon: Trash2 },
  ] as const;

  return (
    // biome-ignore lint/a11y/useKeyWithClickEvents: the list's listbox owns keyboard navigation; the row only reports the pointer.
    <div
      role="option"
      tabIndex={-1}
      aria-selected={selected}
      data-message-id={message.id}
      className={joinClasses(
        "group relative h-[84px] pt-2 pb-2 pl-6 pr-3",
        contrast && "bg-accent text-accent-contrast",
        selected && !contrast && "bg-selection-inactive",
      )}
      onClick={(event) => onSelect(message.id, clickMode(event))}
      onContextMenu={(event) => {
        event.preventDefault();
        onContextMenu(message.id, event.clientX, event.clientY);
      }}
    >
      {!selected && <div className="absolute bottom-0 left-6 right-0 h-px bg-separator" />}

      <div className="flex items-center gap-1">
        {props.vip && (
          <Star size={10} role="img" aria-label="VIP" className={joinClasses("shrink-0 fill-current", secondary)} />
        )}
        <span className="text-list-sender truncate flex-1">{displayName(message.from)}</span>
        <span className={joinClasses("flex shrink-0 items-center gap-1", onAction && "group-hover:hidden")}>
          {message.hasAttachments && (
            <Paperclip size={12} aria-label="Has attachments" className={joinClasses("shrink-0", secondary)} />
          )}
          <span className={joinClasses("text-list-date tabular-nums shrink-0", secondary)}>
            {formatListDate(new Date(message.date), now)}
          </span>
        </span>
        {onAction && (
          // biome-ignore lint/a11y/useSemanticElements: a fieldset's border and legend do not belong in a list row.
          <span role="group" aria-label="Message actions" className="hidden shrink-0 items-center group-hover:flex">
            {actions
              .filter(({ action }) => action !== "archive" || canArchive)
              .map(({ action, label, shortcut, Icon }) => (
                <button
                  key={action}
                  type="button"
                  tabIndex={-1}
                  aria-label={label}
                  title={`${label} (${shortcut})`}
                  className={joinClasses(
                    "flex h-[18px] w-[22px] items-center justify-center rounded border-0 bg-transparent p-0",
                    contrast
                      ? "text-accent-contrast hover:bg-accent-contrast/20"
                      : "text-secondary hover:bg-selection-inactive",
                  )}
                  onMouseDown={(event) => event.preventDefault()}
                  onDoubleClick={(event) => event.stopPropagation()}
                  onClick={(event) => {
                    event.stopPropagation();
                    onAction(message.id, action);
                  }}
                >
                  <Icon size={14} className={action === "flag" && message.flags.flagged ? "fill-current" : undefined} />
                </button>
              ))}
          </span>
        )}
      </div>

      <div className="flex items-center gap-1">
        <span className={joinClasses("text-list-subject truncate flex-1", subject === "" && "text-tertiary")}>
          {subject === "" ? "(No Subject)" : subject}
        </span>
        {showThreadCount && message.threadCount > 1 && (
          <span
            role="img"
            aria-label={`${message.threadCount} messages`}
            className="h-4 px-[5px] rounded-lg bg-badge text-badge-text text-[10px] font-semibold leading-4 shrink-0"
          >
            {message.threadCount}
          </span>
        )}
      </div>

      <div className={joinClasses("text-list-preview line-clamp-2", secondary)}>{message.preview}</div>

      {!message.flags.seen && (
        <span
          role="img"
          aria-label="Unread"
          className={joinClasses(
            "absolute left-[7.5px] top-[12.5px] size-[9px] rounded-full",
            contrast ? "bg-accent-contrast" : "bg-accent",
          )}
        />
      )}
      {message.flags.flagged && (
        <Flag
          size={12}
          aria-label={`Flagged ${flagName(message.flags.flagColor)}`}
          className={joinClasses(
            "absolute left-[6px] top-[29px] fill-current",
            FLAG_CLASSES[message.flags.flagColor] ?? "",
          )}
        />
      )}
    </div>
  );
}
