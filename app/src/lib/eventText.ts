import type { PartStat } from "../rpc/gen/api";
import { addDays } from "./calendarDates";

const dayNames: Readonly<Record<string, string>> = {
  MO: "Monday",
  TU: "Tuesday",
  WE: "Wednesday",
  TH: "Thursday",
  FR: "Friday",
  SA: "Saturday",
  SU: "Sunday",
};
const ordinals: Readonly<Record<string, string>> = {
  "1": "first",
  "2": "second",
  "3": "third",
  "4": "fourth",
  "-1": "last",
};
const units: Readonly<Record<string, string>> = { DAILY: "day", WEEKLY: "week", MONTHLY: "month", YEARLY: "year" };

function ruleParts(line: string): Map<string, string> | undefined {
  const parts = new Map<string, string>();
  const allowed = ["FREQ", "INTERVAL", "COUNT", "UNTIL", "WKST", "BYDAY", "BYMONTHDAY"];
  for (const part of line.slice(6).split(";")) {
    const pair = part.split("=");
    const [key, value] = pair;
    if (pair.length !== 2 || key === undefined || !value || !allowed.includes(key) || parts.has(key)) return undefined;
    parts.set(key, value);
  }
  if (parts.has("COUNT") && parts.has("UNTIL")) return undefined;
  return parts;
}

function positiveInteger(value: string): boolean {
  return /^\d+$/.test(value) && Number.isSafeInteger(Number(value)) && Number(value) > 0;
}

function dayDescription(parts: Map<string, string>): string | undefined {
  const frequency = parts.get("FREQ");
  const byDay = parts.get("BYDAY");
  const monthDay = parts.get("BYMONTHDAY");
  if (byDay !== undefined && monthDay !== undefined) return undefined;
  if (monthDay !== undefined) {
    if (
      frequency !== "MONTHLY" ||
      !/^-?\d+$/.test(monthDay) ||
      Number(monthDay) === 0 ||
      Math.abs(Number(monthDay)) > 31
    ) {
      return undefined;
    }
    return ` on day ${Number(monthDay)}`;
  }
  if (byDay === undefined) return "";
  if (frequency === "WEEKLY") {
    const names = byDay.split(",").map((day) => (Object.hasOwn(dayNames, day) ? dayNames[day] : undefined));
    if (names.some((name) => name === undefined)) return undefined;
    if (names.length === 1) return ` on ${names[0]}`;
    return ` on ${names.slice(0, -1).join(", ")} and ${names.at(-1)}`;
  }
  if (frequency !== "MONTHLY") return undefined;
  const match = /^(-1|[1-4])(MO|TU|WE|TH|FR|SA|SU)$/.exec(byDay);
  const ordinal = match?.[1];
  const day = match?.[2];
  return ordinal === undefined || day === undefined ? undefined : ` on the ${ordinals[ordinal]} ${dayNames[day]}`;
}

function untilText(value: string, locale: string): string | undefined {
  if (!/^\d{8}/.test(value)) return undefined;
  const date = `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)}`;
  const instant = new Date(date);
  if (!Number.isFinite(instant.getTime()) || instant.toISOString().slice(0, 10) !== date) return undefined;
  return new Intl.DateTimeFormat(locale, { timeZone: "UTC", month: "long", day: "numeric", year: "numeric" }).format(
    instant,
  );
}

/** describeRecurrence describes supported RRULEs with English words. */
export function describeRecurrence(recurrence: string, locale: string): string {
  if (recurrence === "") return "";
  const line = recurrence.split("\n").find((entry) => entry.startsWith("RRULE:"));
  const parts = line === undefined ? undefined : ruleParts(line.trimEnd());
  if (parts === undefined) return "Custom";
  const frequency = parts.get("FREQ") ?? "";
  const unit = Object.hasOwn(units, frequency) ? units[frequency] : undefined;
  const interval = parts.get("INTERVAL") ?? "1";
  const count = parts.get("COUNT");
  const until = parts.get("UNTIL");
  const days = dayDescription(parts);
  if (unit === undefined || !positiveInteger(interval) || days === undefined) return "Custom";
  if (count !== undefined && !positiveInteger(count)) return "Custom";
  let text = Number(interval) === 1 ? `Every ${unit}` : `Every ${Number(interval)} ${unit}s`;
  const weekdays = parts.get("BYDAY")?.split(",") ?? [];
  if (
    parts.get("FREQ") === "WEEKLY" &&
    Number(interval) === 1 &&
    weekdays.length === 5 &&
    ["MO", "TU", "WE", "TH", "FR"].every((day) => weekdays.includes(day))
  ) {
    text = "Every weekday";
  } else {
    text += days;
  }
  if (count !== undefined) text += Number(count) === 1 ? ", once" : `, ${Number(count)} times`;
  if (until !== undefined) {
    const end = untilText(until, locale);
    if (end === undefined) return "Custom";
    text += `, until ${end}`;
  }
  return text;
}

/** alarmText names the amount of time before or after an event's start. */
export function alarmText(minutes: number): string {
  if (minutes === 0) return "At time of event";
  const absolute = Math.abs(minutes);
  const divisor = absolute % 1440 === 0 ? 1440 : absolute % 60 === 0 ? 60 : 1;
  const unit = divisor === 1440 ? "day" : divisor === 60 ? "hour" : "minute";
  const amount = absolute / divisor;
  return `${amount} ${unit}${amount === 1 ? "" : "s"} ${minutes < 0 ? "after" : "before"}`;
}

/** timeRange formats an event's times in the requested zone and locale. */
export function timeRange(start: string, end: string, timeZone: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { hour: "numeric", minute: "2-digit", timeZone }).formatRange(
    new Date(start),
    new Date(end),
  );
}

/** dateText formats a timed event's start day or an all-day event's date span. */
export function dateText(
  e: { allDay: boolean; start: string; end: string; startDate: string; endDate: string },
  timeZone: string,
  locale: string,
): string {
  if (e.allDay && e.endDate > addDays(e.startDate, 1)) {
    return new Intl.DateTimeFormat(locale, {
      timeZone: "UTC",
      month: "long",
      day: "numeric",
      year: "numeric",
    }).formatRange(new Date(e.startDate), new Date(addDays(e.endDate, -1)));
  }
  return new Intl.DateTimeFormat(locale, {
    timeZone: e.allDay ? "UTC" : timeZone,
    weekday: "long",
    month: "long",
    day: "numeric",
    year: "numeric",
  }).format(new Date(e.allDay ? e.startDate : e.start));
}

/** answerText names an invitation answer. */
export function answerText(answer: PartStat): string {
  const labels: Record<PartStat, string> = {
    accepted: "Accepted",
    declined: "Declined",
    tentative: "Maybe",
    needsaction: "Not answered",
    delegated: "Delegated",
  };
  return labels[answer];
}

/** calendarColor returns a collection's color or the theme accent. */
export function calendarColor(color: string): string {
  return color || "var(--accent)";
}
