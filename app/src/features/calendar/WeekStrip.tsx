// A narrow window's date picker over Calendar's day (docs/specs/ui.md,
// Narrow windows): the selected date's week, a day a tap, with the weeks
// before and after a chevron away.
import { ChevronLeft, ChevronRight } from "lucide-react";

import { addDays, startOfWeek, step, viewTitle } from "../../lib/calendarDates";
import type { MiniMonthProps } from "./MiniMonth";

/** WeekStrip shows the selected date's week and selects a day of it. */
export function WeekStrip({ selected, today, weekStart, busy, locale, onSelect }: MiniMonthProps) {
  const first = startOfWeek(selected, weekStart);
  const days = Array.from({ length: 7 }, (_, i) => addDays(first, i));
  const format = (date: string, options: Intl.DateTimeFormatOptions) =>
    new Intl.DateTimeFormat(locale, { ...options, timeZone: "UTC" }).format(new Date(date));
  const chevron =
    "flex size-11 shrink-0 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive";
  return (
    <fieldset
      aria-label="Week"
      className="m-0 flex shrink-0 items-center border-0 border-b border-separator bg-window p-0 px-1"
    >
      <button
        type="button"
        aria-label="Previous Week"
        className={chevron}
        onClick={() => onSelect(step("week", selected, -1))}
      >
        <ChevronLeft size={20} aria-hidden="true" />
      </button>
      <div className="grid min-w-0 flex-1 grid-cols-7">
        {days.map((date) => {
          const current = date === today;
          const active = date === selected;
          const circle = active
            ? current
              ? "bg-accent text-accent-contrast"
              : "bg-selection-sidebar"
            : current
              ? "font-semibold text-accent"
              : "";
          return (
            <button
              key={date}
              type="button"
              aria-label={viewTitle("day", date, weekStart, locale)}
              aria-pressed={active}
              aria-current={current ? "date" : undefined}
              onClick={() => onSelect(date)}
              className="flex h-14 min-w-0 flex-col items-center justify-center gap-0.5"
            >
              <span className="text-[11px] leading-[14px] text-tertiary">{format(date, { weekday: "narrow" })}</span>
              <span
                className={`relative flex size-8 items-center justify-center rounded-full text-[15px] leading-5 ${circle}`}
              >
                {format(date, { day: "numeric" })}
                {busy.has(date) && (
                  <span
                    data-busy=""
                    className="absolute -bottom-0.5 left-1/2 size-1 -translate-x-1/2 rounded-full bg-tertiary"
                  />
                )}
              </span>
            </button>
          );
        })}
      </div>
      <button
        type="button"
        aria-label="Next Week"
        className={chevron}
        onClick={() => onSelect(step("week", selected, 1))}
      >
        <ChevronRight size={20} aria-hidden="true" />
      </button>
    </fieldset>
  );
}
