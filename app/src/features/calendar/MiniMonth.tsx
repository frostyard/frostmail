// The Calendar sidebar's small month.
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useState } from "react";
import { monthGrid, step, viewTitle } from "../../lib/calendarDates";
/** MiniMonthProps are the small month's inputs. */
export interface MiniMonthProps {
  /** The selected date, YYYY-MM-DD. */
  selected: string;
  today: string;
  /** The week's first day; 0 is Sunday. */
  weekStart: number;
  /** Dates with occurrences: a dot under the number. */
  busy: ReadonlySet<string>;
  locale: string;
  onSelect: (date: string) => void;
}

/** MiniMonth shows six weeks around the selected date's month. */
export function MiniMonth({ selected, today, weekStart, busy, locale, onSelect }: MiniMonthProps) {
  const [selection, setSelection] = useState(selected);
  const [shown, setShown] = useState(selected);
  if (selection !== selected) {
    setSelection(selected);
    setShown(selected);
  }
  const days = monthGrid(shown, weekStart);
  const title = viewTitle("month", shown, weekStart, locale);
  const format = (date: string, options: Intl.DateTimeFormatOptions) =>
    new Intl.DateTimeFormat(locale, { ...options, timeZone: "UTC" }).format(new Date(date));
  return (
    <div className="p-3">
      <header className="mb-2 flex items-center justify-between">
        <button
          type="button"
          aria-label="Previous Month"
          className="flex size-6 items-center justify-center"
          onClick={() => setShown(step("month", shown, -1))}
        >
          <ChevronLeft size={14} aria-hidden="true" />
        </button>
        <span className="text-[13px] font-semibold leading-4">{title}</span>
        <button
          type="button"
          aria-label="Next Month"
          className="flex size-6 items-center justify-center"
          onClick={() => setShown(step("month", shown, 1))}
        >
          <ChevronRight size={14} aria-hidden="true" />
        </button>
      </header>
      {/* biome-ignore lint/a11y/noNoninteractiveElementToInteractiveRole: a calendar grid is a table with interactive day controls. */}
      <table role="grid" className="w-full" aria-label={title}>
        <thead>
          <tr className="grid grid-cols-7">
            {days.slice(0, 7).map((date) => (
              <th key={date} scope="col" className="text-center text-[11px] leading-[14px] text-tertiary">
                {format(date, { weekday: "narrow" })}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: 6 }, (_, week) => (
            <tr key={days[week * 7]} className="grid grid-cols-7">
              {days.slice(week * 7, week * 7 + 7).map((date) => {
                const current = date === today;
                const active = date === selected;
                const color = active
                  ? current
                    ? "bg-accent text-accent-contrast"
                    : "bg-selection-sidebar"
                  : current
                    ? "font-semibold text-accent"
                    : date.slice(0, 7) !== shown.slice(0, 7)
                      ? "text-tertiary"
                      : "";
                return (
                  <td key={date} className="text-center">
                    <button
                      type="button"
                      aria-label={viewTitle("day", date, weekStart, locale)}
                      aria-pressed={active}
                      aria-current={current ? "date" : undefined}
                      onClick={() => onSelect(date)}
                      className={`relative size-6 rounded-full text-[12px] leading-4 ${color}`}
                    >
                      {format(date, { day: "numeric" })}
                      {busy.has(date) && (
                        <span
                          data-busy=""
                          className="absolute bottom-0 left-1/2 size-1 -translate-x-1/2 rounded-full bg-tertiary"
                        />
                      )}
                    </button>
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
