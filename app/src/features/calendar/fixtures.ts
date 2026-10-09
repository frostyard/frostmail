// CONTRACT TEST FIXTURES for task card T-0071 (docs/tasks). Do not edit.
import type { Occurrence } from "../../rpc/gen/api";

let next = 100;

/** timed makes a timed occurrence in calendar 1 (or extra's). */
export function timed(summary: string, start: string, end: string, extra: Partial<Occurrence> = {}): Occurrence {
  return {
    eventId: next++,
    recurrenceId: "",
    calendarId: 1,
    accountId: 1,
    summary,
    location: "",
    allDay: false,
    start,
    end,
    startDate: "",
    endDate: "",
    status: "confirmed",
    transparent: false,
    recurring: false,
    ...extra,
  };
}

/** allDay makes an all-day occurrence over [startDate, endDate). */
export function allDay(
  summary: string,
  startDate: string,
  endDate: string,
  extra: Partial<Occurrence> = {},
): Occurrence {
  return timed(summary, `${startDate}T00:00:00Z`, `${endDate}T00:00:00Z`, {
    allDay: true,
    startDate,
    endDate,
    ...extra,
  });
}

/** norm replaces the thin and narrow no-break spaces Intl puts in times. */
export const norm = (s: string) => s.replace(/[   ]/g, " ");

/** named matches an accessible name after norm. */
export const named = (want: string) => (name: string) => norm(name) === want;

export const colors: ReadonlyMap<number, string> = new Map([
  [1, "#3366cc"],
  [2, ""],
]);

export const WEEK = ["2026-10-04", "2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09", "2026-10-10"];
