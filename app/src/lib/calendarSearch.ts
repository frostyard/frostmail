// Calendar's search results as rows (docs/specs/pim-ui.md, Calendar search).
import type { Occurrence } from "../rpc/gen/api";
import { timeRange } from "./eventText";

/** SearchRow is a heading or an occurrence of the results. */
export type SearchRow = { heading: string } | { occurrence: Occurrence };

/** searchRows splits occurrences in start order at now: those not yet ended
 * under Upcoming, then the rest under Earlier, the latest first. */
export function searchRows(occurrences: Occurrence[], now: Date): SearchRow[] {
  const at = now.getTime();
  let first = occurrences.findIndex((o) => Date.parse(o.end) > at);
  if (first < 0) first = occurrences.length;
  const upcoming = occurrences.slice(first);
  const earlier = occurrences.slice(0, first).reverse();
  return [
    ...(upcoming.length > 0 ? [{ heading: "Upcoming" }, ...upcoming.map((occurrence) => ({ occurrence }))] : []),
    ...(earlier.length > 0 ? [{ heading: "Earlier" }, ...earlier.map((occurrence) => ({ occurrence }))] : []),
  ];
}

/** searchWhen writes a result's day, time, place and state. */
export function searchWhen(o: Occurrence, timeZone: string, locale: string): string {
  const day = new Intl.DateTimeFormat(locale, {
    weekday: "short",
    month: "short",
    day: "numeric",
    year: "numeric",
    timeZone: o.allDay ? "UTC" : timeZone,
  }).format(new Date(o.allDay ? `${o.startDate}T00:00:00Z` : o.start));
  return [
    day,
    o.allDay ? "All day" : timeRange(o.start, o.end, timeZone, locale),
    ...(o.location ? [o.location] : []),
    ...(o.status === "cancelled" ? ["Cancelled"] : []),
  ].join(" · ");
}

/** searchSubtitle counts the results under the toolbar's title. */
export function searchSubtitle(occurrences: Occurrence[] | null, failed: boolean): string {
  if (failed) return "The search failed";
  if (occurrences === null) return "Searching…";
  return occurrences.length === 1 ? "1 result" : `${occurrences.length} results`;
}
