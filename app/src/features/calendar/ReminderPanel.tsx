// The reminder window's content (docs/specs/pim-ui.md, Reminder window).
import { Check, ChevronDown, X } from "lucide-react";
import { type CSSProperties, useState } from "react";
import { calendarColor } from "../../lib/eventText";
import { reminderWhen, snoozeChoices } from "../../lib/reminderText";
import type { Reminder } from "../../rpc/gen/api";
import { ContextMenu } from "../menu/ContextMenu";

/** ReminderPanelProps are the reminder window's inputs. */
export interface ReminderPanelProps {
  reminders: Reminder[];
  /** Each calendar's color: #rrggbb, or "" for the accent color. */
  colors: ReadonlyMap<number, string>;
  timeZone: string;
  locale: string;
  now: Date;
  onSnooze: (id: string, until: Date) => void;
  onDismiss: (ids: string[]) => void;
  onOpen: (reminder: Reminder) => void;
  onClose: () => void;
}

/** ReminderPanel shows the title strip, the due reminders and Dismiss All. */
export function ReminderPanel(props: ReminderPanelProps) {
  const [menu, setMenu] = useState<{ id: string; x: number; y: number; anchor: HTMLButtonElement } | null>(null);
  const choices = snoozeChoices(props.now, props.timeZone);
  const closeMenu = () => {
    menu?.anchor.focus();
    setMenu(null);
  };
  return (
    <section
      role="dialog"
      aria-label="Reminders"
      className="flex h-full flex-col bg-window text-primary"
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.stopPropagation();
          props.onClose();
        }
      }}
    >
      <header
        data-tauri-drag-region
        className="flex h-[36px] shrink-0 items-center justify-between border-b border-separator px-3"
      >
        <h1 data-tauri-drag-region className="text-[13px]/[16px] font-semibold">
          Reminders
        </h1>
        <button
          type="button"
          aria-label="Close"
          className="flex h-7 w-7 items-center justify-center rounded-md hover:bg-selection-inactive"
          onClick={props.onClose}
        >
          <X size={14} />
        </button>
      </header>
      <ul aria-label="Reminders" className="min-h-0 flex-1 overflow-y-auto">
        {props.reminders.map((r) => (
          <li
            key={r.id}
            style={{ "--event-color": calendarColor(props.colors.get(r.calendarId) ?? "") } as CSSProperties}
            className="relative flex h-[56px] items-center gap-2 px-3"
          >
            <span className="absolute inset-y-3 left-0 w-1 rounded-sm bg-[var(--event-color)]" />
            <div className="min-w-0 flex-1">
              <button
                type="button"
                className="block max-w-full truncate text-left text-[13px]/[18px] font-semibold"
                onClick={() => props.onOpen(r)}
              >
                {r.summary || "No Title"}
              </button>
              <div className="truncate text-[12px]/[16px] text-secondary">
                {reminderWhen(r, props.now, props.timeZone, props.locale)}
                {r.location && ` · ${r.location}`}
              </div>
            </div>
            <button
              type="button"
              aria-haspopup="menu"
              aria-expanded={menu?.id === r.id}
              className="flex h-7 items-center gap-1 rounded-md px-2 text-[13px] hover:bg-selection-inactive"
              onClick={(event) => {
                const anchor = event.currentTarget;
                const rect = anchor.getBoundingClientRect();
                setMenu({ id: r.id, x: rect.left, y: rect.bottom, anchor });
              }}
            >
              Snooze
              <ChevronDown size={14} />
            </button>
            <button
              type="button"
              aria-label="Dismiss"
              className="flex h-7 w-7 items-center justify-center rounded-md hover:bg-selection-inactive"
              onClick={() => props.onDismiss([r.id])}
            >
              <Check size={14} />
            </button>
          </li>
        ))}
      </ul>
      <footer className="flex h-[44px] shrink-0 items-center justify-end border-t border-separator px-3">
        {props.reminders.length >= 2 && (
          <button
            type="button"
            className="h-7 rounded-md px-2 text-[13px] hover:bg-selection-inactive"
            onClick={() => props.onDismiss(props.reminders.map((r) => r.id))}
          >
            Dismiss All
          </button>
        )}
      </footer>
      {menu && (
        <ContextMenu
          x={menu.x}
          y={menu.y}
          items={choices.map((choice) => ({ kind: "item", id: choice.label, label: choice.label }))}
          onClose={closeMenu}
          onSelect={(label) => {
            const choice = choices.find((choice) => choice.label === label);
            if (choice) props.onSnooze(menu.id, choice.until);
          }}
        />
      )}
    </section>
  );
}
