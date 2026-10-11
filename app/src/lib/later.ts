import { addDays, today, zoned } from "./calendarDates";

/** LaterChoice is a menu choice and its scheduled instant. */
export interface LaterChoice {
  label: string;
  at: Date;
}

/** atLocal finds a date's wall-clock time in the supplied time zone. */
export function atLocal(date: string, hour: number, minute: number, timeZone: string): Date {
  const target = new Date(date).getTime() + (hour * 60 + minute) * 60_000;
  let instant = target;
  for (let correction = 0; correction < 3; correction++) {
    const wall = zoned(new Date(instant), timeZone);
    const difference = target - (new Date(wall.date).getTime() + wall.minutes * 60_000);
    instant += difference;
    if (difference === 0) break;
  }
  return new Date(instant);
}

function timeText(at: Date, timeZone: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { hour: "numeric", minute: "2-digit", timeZone }).format(at);
}

/** whenText describes a scheduled time relative to today's local date. */
export function whenText(at: Date, now: Date, timeZone: string, locale: string): string {
  const current = today(timeZone, now);
  const date = today(timeZone, at);
  const day =
    date === current
      ? "Today"
      : date === addDays(current, 1)
        ? "Tomorrow"
        : new Intl.DateTimeFormat(locale, {
            weekday: "short",
            month: "short",
            day: "numeric",
            ...(date.slice(0, 4) !== current.slice(0, 4) ? { year: "numeric" } : {}),
            timeZone,
          }).format(at);
  return `${day} at ${timeText(at, timeZone, locale)}`;
}

/** sendChoices offers tonight before 21:00, followed by tomorrow at 08:00. */
export function sendChoices(now: Date, timeZone: string, locale: string): LaterChoice[] {
  const date = today(timeZone, now);
  const tonight = atLocal(date, 21, 0, timeZone);
  const tomorrow = atLocal(addDays(date, 1), 8, 0, timeZone);
  return [
    ...(now < tonight ? [{ label: `Send ${timeText(tonight, timeZone, locale)} Tonight`, at: tonight }] : []),
    { label: `Send ${timeText(tomorrow, timeZone, locale)} Tomorrow`, at: tomorrow },
  ];
}

/** remindChoices offers an hour from now, tonight when future, and tomorrow. */
export function remindChoices(now: Date, timeZone: string): LaterChoice[] {
  const date = today(timeZone, now);
  const tonight = atLocal(date, 21, 0, timeZone);
  return [
    { label: "Remind Me in 1 Hour", at: new Date(now.getTime() + 3_600_000) },
    ...(now < tonight ? [{ label: "Remind Me Tonight", at: tonight }] : []),
    { label: "Remind Me Tomorrow", at: atLocal(addDays(date, 1), 8, 0, timeZone) },
  ];
}
