// Six weeks of date cells and occurrence lines.
import { viewTitle } from "../../lib/calendarDates";
import { monthItems } from "../../lib/eventLayout";
import type { Occurrence } from "../../rpc/gen/api";
import { eventClasses, eventSelected, eventStyle, eventTitleClasses } from "./eventStyle";
import type { OccurrenceKey } from "./TimeGrid";

/** MonthGridProps are the month view's inputs. */
export interface MonthGridProps {
  /** The 42 days shown (monthGrid). */
  days: string[];
  /** The selected date; its month is the one shown, the others dimmed. */
  selectedDate: string;
  occurrences: Occurrence[];
  colors: ReadonlyMap<number, string>;
  timeZone: string;
  locale: string;
  today: string;
  selected: OccurrenceKey | null;
  /** The most lines a cell shows, "N more" included; at least 2. */
  lines: number;
  onSelect: (occurrence: Occurrence) => void;
  onSelectDate: (date: string) => void;
  onShowDay: (date: string) => void;
}

function MonthCell({ date, props }: { date: string; props: MonthGridProps }) {
  const items = monthItems(props.occurrences, date, props.timeZone);
  const visible = items.length > props.lines ? props.lines - 1 : props.lines;
  const more = items.length - visible;
  const label = viewTitle("day", date, 0, props.locale);
  const current = date === props.today;
  const day = new Intl.DateTimeFormat(props.locale, {
    timeZone: "UTC",
    day: "numeric",
    ...(date.endsWith("-01") ? { month: "short" } : {}),
  }).format(new Date(date));
  return (
    <td
      // biome-ignore lint/a11y/noNoninteractiveElementToInteractiveRole: selectable table cells use the ARIA gridcell role.
      role="gridcell"
      aria-label={label}
      aria-selected={date === props.selectedDate}
      aria-current={current ? "date" : undefined}
      tabIndex={0}
      onClick={() => props.onSelectDate(date)}
      onDoubleClick={() => props.onShowDay(date)}
      onKeyDown={(event) => {
        if (event.target === event.currentTarget && (event.key === "Enter" || event.key === " ")) {
          event.preventDefault();
          props.onSelectDate(date);
        }
      }}
      className={`@container min-w-0 border-b border-r border-separator p-1 ${date === props.selectedDate ? "bg-selection-inactive" : ""}`}
    >
      <div className="mb-1 flex justify-end">
        <span
          className={`flex h-5 min-w-5 items-center justify-center rounded-full text-[12px] leading-4 ${current ? "bg-accent text-accent-contrast" : date.slice(0, 7) !== props.selectedDate.slice(0, 7) ? "text-tertiary" : ""}`}
        >
          {day}
        </span>
      </div>
      {items.slice(0, visible).map((occurrence) => {
        const start = occurrence.allDay
          ? ""
          : new Intl.DateTimeFormat(props.locale, {
              hour: "numeric",
              minute: "2-digit",
              timeZone: props.timeZone,
            }).format(new Date(occurrence.start));
        const active = eventSelected(occurrence, props.selected);
        return (
          <button
            key={`${occurrence.eventId}:${occurrence.recurrenceId}`}
            type="button"
            aria-label={`${occurrence.summary || "No Title"}, ${occurrence.allDay ? "all day" : start}`}
            aria-pressed={active}
            onClick={(event) => {
              event.stopPropagation();
              props.onSelect(occurrence);
            }}
            onDoubleClick={(event) => event.stopPropagation()}
            className={`flex h-[18px] w-full items-center gap-1 rounded-[4px] px-1 text-left text-[11px] leading-[14px] ${occurrence.allDay ? "font-semibold" : ""} ${eventClasses(occurrence, active, occurrence.allDay)}`}
            style={eventStyle(occurrence, props.colors)}
          >
            {!occurrence.allDay && <span className="size-[6px] shrink-0 rounded-full bg-[var(--event-color)]" />}
            <span className={`min-w-0 flex-1 truncate ${eventTitleClasses(occurrence)}`}>
              {occurrence.summary || "No Title"}
            </span>
            {!occurrence.allDay && (
              <span className="hidden shrink-0 text-secondary tabular-nums @[9rem]:inline">{start}</span>
            )}
          </button>
        );
      })}
      {more > 0 && (
        <button
          type="button"
          aria-label={`Show ${more} more on ${label}`}
          onClick={(event) => {
            event.stopPropagation();
            props.onShowDay(date);
          }}
          onDoubleClick={(event) => event.stopPropagation()}
          className="h-[18px] text-[11px] leading-[14px] text-secondary"
        >
          {more} more
        </button>
      )}
    </td>
  );
}

/** MonthGrid shows six weeks of days and their occurrences. */
export function MonthGrid(props: MonthGridProps) {
  return (
    <table
      // biome-ignore lint/a11y/noNoninteractiveElementToInteractiveRole: calendar tables use the ARIA grid pattern.
      role="grid"
      aria-label={viewTitle("month", props.selectedDate, 0, props.locale)}
      className="flex h-full flex-col bg-window"
    >
      <thead>
        <tr className="grid h-6 shrink-0 grid-cols-7">
          {props.days.slice(0, 7).map((date) => (
            <th key={date} scope="col" className="text-center text-[11px] leading-[14px] text-secondary">
              {new Intl.DateTimeFormat(props.locale, { weekday: "short", timeZone: "UTC" }).format(new Date(date))}
            </th>
          ))}
        </tr>
      </thead>
      <tbody className="flex min-h-0 flex-1 flex-col">
        {Array.from({ length: 6 }, (_, week) => (
          <tr key={props.days[week * 7]} className="grid min-h-0 flex-1 grid-cols-7">
            {props.days.slice(week * 7, week * 7 + 7).map((date) => (
              <MonthCell key={date} date={date} props={props} />
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
