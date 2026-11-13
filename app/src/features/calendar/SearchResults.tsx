// The results of Calendar's search, in the view's place
// (docs/specs/pim-ui.md, Calendar search).
import type { KeyboardEvent } from "react";
import { type SearchRow, searchRows, searchWhen } from "../../lib/calendarSearch";
import type { Occurrence } from "../../rpc/gen/api";
import { eventSelected, eventStyle } from "./eventStyle";
import type { OccurrenceKey } from "./TimeGrid";

/** SearchResultsProps are the results' inputs. */
export interface SearchResultsProps {
  /** In start order; null while the search is asked. */
  occurrences: Occurrence[] | null;
  failed: boolean;
  /** Each calendar's color: #rrggbb, or "" for the accent color. */
  colors: ReadonlyMap<number, string>;
  timeZone: string;
  locale: string;
  now: Date;
  selected: OccurrenceKey | null;
  onSelect: (occurrence: Occurrence) => void;
  /** Enter or a double click on the selected row. */
  onOpen: () => void;
}

const isOccurrence = (row: SearchRow): row is { occurrence: Occurrence } => "occurrence" in row;

/** SearchResults lists the occurrences found under Upcoming and Earlier. */
export function SearchResults(props: SearchResultsProps) {
  const { occurrences, colors, timeZone, locale, selected } = props;
  if (occurrences === null || occurrences.length === 0) {
    const text = props.failed ? "The search failed" : occurrences === null ? "" : "No Results";
    return <div className="flex h-full items-center justify-center bg-window text-[15px] text-tertiary">{text}</div>;
  }
  const rows = searchRows(occurrences, props.now);
  const found = rows.filter(isOccurrence).map((row) => row.occurrence);
  const move = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Enter") {
      if (selected) props.onOpen();
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      const index = found.findIndex((o) => eventSelected(o, selected));
      const next = found[index < 0 ? 0 : index + (event.key === "ArrowDown" ? 1 : -1)];
      if (next) props.onSelect(next);
    } else return;
    event.preventDefault();
    event.stopPropagation();
  };
  return (
    <div
      role="listbox"
      aria-label="Search results"
      tabIndex={0}
      onKeyDown={move}
      className="h-full overflow-y-auto bg-window outline-none"
    >
      {rows.map((row) => {
        if (!isOccurrence(row))
          return (
            <div
              key={row.heading}
              role="presentation"
              className="flex h-6 items-center bg-toolbar px-3 text-sidebar-section text-secondary uppercase"
            >
              {row.heading}
            </div>
          );
        const o = row.occurrence;
        const title = o.summary.trim() || "No Title";
        const when = searchWhen(o, timeZone, locale);
        const chosen = eventSelected(o, selected);
        return (
          // biome-ignore lint/a11y/useKeyWithClickEvents: the listbox holds the keys.
          <div
            key={`${o.eventId}:${o.recurrenceId}`}
            role="option"
            tabIndex={-1}
            aria-selected={chosen}
            aria-label={`${title}, ${when}`}
            style={eventStyle(o, colors)}
            ref={chosen ? (element) => element?.scrollIntoView?.({ block: "nearest" }) : undefined}
            className={`flex h-11 items-center gap-[10px] border-b border-separator px-3 ${chosen ? "bg-selection-inactive" : "hover:bg-selection-inactive"}`}
            onClick={() => props.onSelect(o)}
            onDoubleClick={props.onOpen}
          >
            <span aria-hidden="true" className="h-2 w-2 shrink-0 rounded-full bg-[var(--event-color)]" />
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate text-[13px] leading-[18px] font-semibold">{title}</span>
              <span className="truncate text-[12px] leading-4 text-secondary">{when}</span>
            </span>
          </div>
        );
      })}
    </div>
  );
}
