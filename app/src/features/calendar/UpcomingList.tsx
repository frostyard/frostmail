// A person's coming occurrences on the contact card and the person pane
// (docs/specs/pim-ui.md, Contact card).
import { useId } from "react";
import { upcomingWhen } from "../../lib/eventText";
import type { Occurrence } from "../../rpc/gen/api";
import { eventStyle } from "./eventStyle";

/** UpcomingListProps are the Upcoming section's inputs. */
export interface UpcomingListProps {
  occurrences: Occurrence[];
  /** Each calendar's color: #rrggbb, or "" for the accent color. */
  colors: ReadonlyMap<number, string>;
  timeZone: string;
  locale: string;
  now: Date;
  onOpen: (occurrence: Occurrence) => void;
}

/** UpcomingList shows occurrences under an Upcoming heading, or nothing. */
export function UpcomingList({ occurrences, colors, timeZone, locale, now, onOpen }: UpcomingListProps) {
  const headingId = useId();
  if (occurrences.length === 0) return null;
  return (
    <section aria-labelledby={headingId}>
      <h3 id={headingId} className="mb-2 text-sidebar-section text-secondary uppercase">
        Upcoming
      </h3>
      {occurrences.map((occurrence) => {
        const title = occurrence.summary.trim() || "No Title";
        const when = upcomingWhen(occurrence, timeZone, locale, now);
        return (
          <button
            key={`${occurrence.eventId}:${occurrence.recurrenceId}`}
            type="button"
            aria-label={`${title}, ${when}`}
            style={eventStyle(occurrence, colors)}
            className="flex h-8 w-full items-center gap-3 text-left hover:bg-selection-inactive"
            onClick={() => onOpen(occurrence)}
          >
            <span aria-hidden="true" className="h-[6px] w-[6px] shrink-0 rounded-full bg-[var(--event-color)]" />
            <span className="min-w-0 flex-1 truncate text-list-subject">{title}</span>
            <span className="shrink-0 text-list-date text-secondary tabular-nums">{when}</span>
          </button>
        );
      })}
    </section>
  );
}
