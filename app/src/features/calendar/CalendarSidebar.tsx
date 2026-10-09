// The small month and account calendars.
import { Lock } from "lucide-react";
import { calendarColor } from "../../lib/eventText";
import type { MiniMonthProps } from "./MiniMonth";
import { MiniMonth } from "./MiniMonth";

/** CalendarRow is one calendar in the sidebar. */
export interface CalendarRow {
  id: number;
  name: string;
  /** #rrggbb, or "" for the accent color. */
  color: string;
  /** Shown (and synced). */
  enabled: boolean;
  readOnly: boolean;
}

/** CalendarSection is an account and its calendars. */
export interface CalendarSection {
  accountId: number;
  title: string;
  calendars: CalendarRow[];
}

/** CalendarSidebarProps are the Calendar sidebar's inputs. */
export interface CalendarSidebarProps extends MiniMonthProps {
  sections: CalendarSection[];
  onToggle: (id: number, enabled: boolean) => void;
}

/** CalendarSidebar shows the small month and the calendars to show. */
export function CalendarSidebar({ sections, onToggle, ...month }: CalendarSidebarProps) {
  return (
    <div className="bg-sidebar">
      <MiniMonth {...month} />
      {sections
        .filter((section) => section.calendars.length > 0)
        .map((section) => (
          <fieldset key={section.accountId} aria-label={section.title}>
            <h3 className="flex h-[26px] items-center truncate pl-3 text-sidebar-section text-secondary">
              {section.title}
            </h3>
            {section.calendars.map((calendar) => (
              // biome-ignore lint/a11y/useSemanticElements: the task contract requires a button with checkbox semantics and a read-only icon.
              <button
                key={calendar.id}
                type="button"
                role="checkbox"
                aria-checked={calendar.enabled}
                onClick={() => onToggle(calendar.id, !calendar.enabled)}
                className="flex h-7 w-full items-center gap-2 pl-3 pr-2 text-left text-[13px] leading-4"
              >
                <span
                  aria-hidden="true"
                  className="size-[14px] shrink-0 rounded-[3px] border-[1.5px]"
                  style={{
                    borderColor: calendarColor(calendar.color),
                    backgroundColor: calendar.enabled ? calendarColor(calendar.color) : undefined,
                  }}
                />
                <span className="truncate">{calendar.name}</span>
                {calendar.readOnly && <Lock size={12} aria-label="Read-only" className="shrink-0 text-secondary" />}
              </button>
            ))}
          </fieldset>
        ))}
    </div>
  );
}
