// The day and week views (docs/specs/pim-ui.md, Day and week views). Task
// T-0071 builds it.
import type { Occurrence } from "../../rpc/gen/api";

/** OccurrenceKey names one occurrence. */
export interface OccurrenceKey {
  eventId: number;
  recurrenceId: string;
}

/** TimeGridProps are the day and week views' inputs. */
export interface TimeGridProps {
  /** The days shown: one (the day view) or seven (the week view). */
  days: string[];
  occurrences: Occurrence[];
  /** Each calendar's color: #rrggbb, or "" for the accent color. */
  colors: ReadonlyMap<number, string>;
  timeZone: string;
  locale: string;
  today: string;
  /** The current time, for the now line. */
  now: Date;
  selected: OccurrenceKey | null;
  onSelect: (occurrence: Occurrence) => void;
  /** Show a day in the day view. */
  onShowDay: (date: string) => void;
}

/** TimeGrid shows the days' all-day strip and hours. */
export function TimeGrid(_props: TimeGridProps) {
  return null;
}
