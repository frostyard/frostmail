import { addDays, dayStart, weekday, zoned } from "./calendarDates";

/** Weekday is a day as RRULE names it. */
export type Weekday = "MO" | "TU" | "WE" | "TH" | "FR" | "SA" | "SU";

/** WEEKDAYS lists the days in RRULE order, Monday first. */
export const WEEKDAYS: readonly Weekday[] = ["MO", "TU", "WE", "TH", "FR", "SA", "SU"];

/** Frequency is how often a rule the repeat sheet writes repeats. */
export type Frequency = "DAILY" | "WEEKLY" | "MONTHLY" | "YEARLY";

/** RepeatPreset is one of the editor's repeat choices. */
export type RepeatPreset = "never" | "daily" | "weekdays" | "weekly" | "biweekly" | "monthly" | "yearly" | "custom";

/** Repeat is a rule as the repeat sheet shows it, without its end. */
export interface Repeat {
  frequency: Frequency;
  interval: number;
  /** Weekly: the days, Monday first. */
  weekdays: Weekday[];
  /** Monthly: on the start's n-th (or last) weekday, not its day of the month. */
  byWeekday: boolean;
}

/** RepeatEnd is when a rule stops. */
export type RepeatEnd = { kind: "never" } | { kind: "date"; date: string } | { kind: "count"; count: number };

function ruleParts(rule: string): Map<string, string> | undefined {
  const parts = new Map<string, string>();
  for (const part of rule.toUpperCase().split(";")) {
    const pair = part.split("=");
    const [name, value] = pair;
    if (pair.length !== 2 || !name || !value || parts.has(name)) return undefined;
    parts.set(name, value);
  }
  return parts.has("FREQ") ? parts : undefined;
}

function writeParts(parts: Map<string, string>): string {
  return Array.from(parts, ([name, value]) => `${name}=${value}`).join(";");
}

function positiveInteger(value: string): boolean {
  return /^\d+$/.test(value) && Number.isSafeInteger(Number(value)) && Number(value) > 0;
}

function isWeekday(value: string): value is Weekday {
  return WEEKDAYS.some((day) => day === value);
}

function startDay(start: string): Weekday {
  return WEEKDAYS[(weekday(start) + 6) % 7] ?? "MO";
}

function isLastWeek(start: string): boolean {
  return addDays(start, 7).slice(0, 7) !== start.slice(0, 7);
}

function matchesWeekday(value: string, start: string): boolean {
  return value === nthWeekday(start) || (value === `-1${startDay(start)}` && isLastWeek(start));
}

function matchesNumber(value: string | undefined, expected: number): boolean {
  return value !== undefined && /^\+?\d+$/.test(value) && Number(value) === expected;
}

/** nthWeekday is the start's weekday in its month, as BYDAY writes it: 1 to 4, or -1 for a fifth. */
export function nthWeekday(start: string): string {
  const ordinal = Math.ceil(Number(start.slice(8, 10)) / 7);
  return `${ordinal === 5 ? -1 : ordinal}${startDay(start)}`;
}

/** repeatOf reads a rule as the repeat sheet shows it, or undefined when the sheet cannot. */
export function repeatOf(rule: string, start: string): Repeat | undefined {
  const parts = ruleParts(rule);
  if (!parts) return undefined;
  const frequency = parts.get("FREQ");
  if (frequency !== "DAILY" && frequency !== "WEEKLY" && frequency !== "MONTHLY" && frequency !== "YEARLY") {
    return undefined;
  }
  const interval = parts.get("INTERVAL") ?? "1";
  const weekStart = parts.get("WKST");
  if (!positiveInteger(interval) || (weekStart !== undefined && !isWeekday(weekStart))) return undefined;
  const allowed = ["FREQ", "INTERVAL", "WKST", "COUNT", "UNTIL"];
  if (frequency === "WEEKLY" || frequency === "MONTHLY") allowed.push("BYDAY");
  if (frequency === "MONTHLY" || frequency === "YEARLY") allowed.push("BYMONTHDAY");
  if (frequency === "YEARLY") allowed.push("BYMONTH");
  if (Array.from(parts.keys()).some((name) => !allowed.includes(name))) return undefined;
  const repeat: Repeat = { frequency, interval: Number(interval), weekdays: [], byWeekday: false };
  const byDay = parts.get("BYDAY");
  const monthDay = parts.get("BYMONTHDAY");
  const month = parts.get("BYMONTH");
  if (frequency === "WEEKLY") {
    const days = byDay === undefined ? [startDay(start)] : byDay.split(",");
    if (!days.every(isWeekday)) return undefined;
    repeat.weekdays = WEEKDAYS.filter((day) => days.includes(day));
  } else if (byDay !== undefined) {
    if (monthDay !== undefined || !matchesWeekday(byDay, start)) return undefined;
    repeat.byWeekday = true;
  }
  if (monthDay !== undefined && !matchesNumber(monthDay, Number(start.slice(8, 10)))) return undefined;
  if (month !== undefined && !matchesNumber(month, Number(start.slice(5, 7)))) return undefined;
  return repeat;
}

/** defaultRepeat is what the repeat sheet starts from for an event that does not repeat. */
export function defaultRepeat(start: string): Repeat {
  return { frequency: "WEEKLY", interval: 1, weekdays: [startDay(start)], byWeekday: false };
}

/** ruleOf writes the sheet's rule, without an end; the start's own day is left to DTSTART. */
export function ruleOf(r: Repeat, start: string): string {
  let rule = `FREQ=${r.frequency}`;
  if (r.interval > 1) rule += `;INTERVAL=${r.interval}`;
  if (r.frequency === "WEEKLY") {
    const days = WEEKDAYS.filter((day) => r.weekdays.includes(day));
    if (days.length > 0 && !(days.length === 1 && days[0] === startDay(start))) {
      rule += `;BYDAY=${days.join(",")}`;
    }
  } else if (r.frequency === "MONTHLY" && r.byWeekday) {
    rule += `;BYDAY=${nthWeekday(start)}`;
  }
  return rule;
}

/** presetOf is the editor's choice a rule shows as; its end does not count. */
export function presetOf(rule: string, start: string): RepeatPreset {
  if (rule === "") return "never";
  const repeat = repeatOf(rule, start);
  if (!repeat) return "custom";
  const { frequency, interval, weekdays, byWeekday } = repeat;
  if (frequency === "WEEKLY") {
    if (interval === 1 && weekdays.join(",") === "MO,TU,WE,TH,FR") return "weekdays";
    if (weekdays.length === 1 && weekdays[0] === startDay(start)) {
      if (interval === 1) return "weekly";
      if (interval === 2) return "biweekly";
    }
  } else if (interval === 1) {
    if (frequency === "DAILY") return "daily";
    if (frequency === "MONTHLY" && !byWeekday) return "monthly";
    if (frequency === "YEARLY") return "yearly";
  }
  return "custom";
}

/** presetRule is the rule a preset writes, without an end; "" for never. */
export function presetRule(preset: Exclude<RepeatPreset, "custom">): string {
  const rules: Record<Exclude<RepeatPreset, "custom">, string> = {
    never: "",
    daily: "FREQ=DAILY",
    weekdays: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
    weekly: "FREQ=WEEKLY",
    biweekly: "FREQ=WEEKLY;INTERVAL=2",
    monthly: "FREQ=MONTHLY",
    yearly: "FREQ=YEARLY",
  };
  return rules[preset];
}

function untilDate(value: string, timeZone: string): string | undefined {
  if (!/^\d{8}(T\d{6}Z?)?$/.test(value)) return undefined;
  const date = `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)}`;
  const day = new Date(date);
  if (!Number.isFinite(day.getTime()) || day.toISOString().slice(0, 10) !== date) return undefined;
  if (value.length === 8) return date;
  const hours = Number(value.slice(9, 11));
  const minutes = Number(value.slice(11, 13));
  const seconds = Number(value.slice(13, 15));
  if (hours > 23 || minutes > 59 || seconds > 60) return undefined;
  if (!value.endsWith("Z")) return date;
  // Date cannot represent a leap second; use the preceding second's day.
  day.setUTCHours(hours, minutes, Math.min(seconds, 59));
  return zoned(day, timeZone).date;
}

/** endOf reads when a rule stops; an UNTIL time is read as its date in timeZone. */
export function endOf(rule: string, timeZone: string): RepeatEnd {
  const parts = ruleParts(rule);
  if (!parts) return { kind: "never" };
  const count = parts.get("COUNT");
  if (count !== undefined && positiveInteger(count)) return { kind: "count", count: Number(count) };
  const until = parts.get("UNTIL");
  const date = until === undefined ? undefined : untilDate(until, timeZone);
  return date === undefined ? { kind: "never" } : { kind: "date", date };
}

/** withEnd is rule ending as end says: on a date (its last day, in timeZone for a timed event) or after a count. */
export function withEnd(rule: string, end: RepeatEnd, allDay: boolean, timeZone: string): string {
  const parts = ruleParts(rule);
  if (!parts) return rule;
  parts.delete("COUNT");
  parts.delete("UNTIL");
  if (end.kind === "count") {
    const count = Number.isFinite(end.count) ? Math.max(1, Math.trunc(end.count)) : 1;
    parts.set("COUNT", String(count));
  } else if (end.kind === "date") {
    const until = allDay
      ? end.date.replaceAll("-", "")
      : new Date(dayStart(addDays(end.date, 1), timeZone).getTime() - 1000)
          .toISOString()
          .replaceAll(/[-:]/g, "")
          .replace(".000", "");
    parts.set("UNTIL", until);
  }
  return writeParts(parts);
}

/** followStart moves the parts of a rule that name the start's day when the start moves from one date to another. */
export function followStart(rule: string, from: string, to: string): string {
  if (from === to) return rule;
  const parts = ruleParts(rule);
  if (!parts) return rule;
  const frequency = parts.get("FREQ");
  const byDay = parts.get("BYDAY");
  if (frequency === "WEEKLY" && byDay === startDay(from)) {
    parts.set("BYDAY", startDay(to));
  } else if (frequency === "MONTHLY" && byDay !== undefined && matchesWeekday(byDay, from)) {
    parts.set("BYDAY", byDay.startsWith("-1") && isLastWeek(to) ? `-1${startDay(to)}` : nthWeekday(to));
  }
  if (matchesNumber(parts.get("BYMONTHDAY"), Number(from.slice(8, 10)))) {
    parts.set("BYMONTHDAY", String(Number(to.slice(8, 10))));
  }
  if (frequency === "YEARLY" && matchesNumber(parts.get("BYMONTH"), Number(from.slice(5, 7)))) {
    parts.set("BYMONTH", String(Number(to.slice(5, 7))));
  }
  return writeParts(parts);
}
