// The reminder window's words (docs/specs/pim-ui.md, Reminder window).

import type { Reminder } from "../rpc/gen/api";
import { addDays, dayStart, today, zoned } from "./calendarDates";

/** SnoozeChoice is one entry of the Snooze menu. */
export interface SnoozeChoice {
  label: string;
  until: Date;
}

/** reminderWhen says when a reminder's occurrence starts, relative to now. */
export function reminderWhen(r: Reminder, now: Date, timeZone: string, locale: string): string {
  const minutes = (Date.parse(r.start) - now.getTime()) / 60_000;
  if (!r.allDay) {
    if (minutes > 0 && minutes < 60) {
      const n = Math.ceil(minutes);
      return `In ${n} minute${n === 1 ? "" : "s"}`;
    }
    if (minutes <= 0 && minutes > -5) return "Now";
    if (minutes <= -5 && minutes > -60) {
      const n = Math.floor(-minutes);
      return `${n} minutes ago`;
    }
  }
  const current = today(timeZone, now);
  const date = r.allDay ? r.startDate : zoned(r.start, timeZone).date;
  const day =
    date === current
      ? "Today"
      : date === addDays(current, 1)
        ? "Tomorrow"
        : date === addDays(current, -1)
          ? "Yesterday"
          : new Intl.DateTimeFormat(locale, {
              weekday: "short",
              month: "short",
              day: "numeric",
              timeZone: "UTC",
            }).format(new Date(date));
  if (r.allDay) return day;
  const time = new Intl.DateTimeFormat(locale, { hour: "numeric", minute: "2-digit", timeZone }).format(
    new Date(r.start),
  );
  return `${day}, ${time}`;
}

/** snoozeChoices are 5, 10 and 15 minutes, an hour, and 9:00 tomorrow in the zone. */
export function snoozeChoices(now: Date, timeZone: string): SnoozeChoice[] {
  return [
    ...[5, 10, 15, 60].map((minutes) => ({
      label: minutes === 60 ? "1 hour" : `${minutes} minutes`,
      until: new Date(now.getTime() + minutes * 60_000),
    })),
    {
      label: "Tomorrow",
      until: new Date(dayStart(addDays(today(timeZone, now), 1), timeZone).getTime() + 9 * 3_600_000),
    },
  ];
}
