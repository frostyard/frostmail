// Calendar's search results as rows (docs/specs/pim-ui.md, Calendar search).
import type { Occurrence } from "../rpc/gen/api";
import { timeRange } from "./eventText";

/** SearchRow is a heading or an occurrence of the results. */
export type SearchRow = { heading: string } | { occurrence: Occurrence };

/** searchRows puts each occurrence, given in start order, under Upcoming
 * when it has not ended, and under Earlier, the latest first, when it has.
 * Each is judged by its own end: a long event still under way starts before
 * shorter ones that are over, so the list has no one place to split. */
export function searchRows(occurrences: Occurrence[], now: Date): SearchRow[] {
  const at = now.getTime();
  const upcoming = occurrences.filter((o) => Date.parse(o.end) > at);
  const earlier = occurrences.filter((o) => Date.parse(o.end) <= at).reverse();
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
