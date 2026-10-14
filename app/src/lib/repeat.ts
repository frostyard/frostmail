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

function notYet(): never {
  throw new Error("lib/repeat comes with task card T-0143");
}

/** nthWeekday is the start's weekday in its month, as BYDAY writes it: 1 to 4, or -1 for a fifth. */
export function nthWeekday(_start: string): string {
  return notYet();
}

/** repeatOf reads a rule as the repeat sheet shows it, or undefined when the sheet cannot. */
export function repeatOf(_rule: string, _start: string): Repeat | undefined {
  return notYet();
}

/** defaultRepeat is what the repeat sheet starts from for an event that does not repeat. */
export function defaultRepeat(_start: string): Repeat {
  return notYet();
}

/** ruleOf writes the sheet's rule, without an end; the start's own day is left to DTSTART. */
export function ruleOf(_r: Repeat, _start: string): string {
  return notYet();
}

/** presetOf is the editor's choice a rule shows as; its end does not count. */
export function presetOf(_rule: string, _start: string): RepeatPreset {
  return notYet();
}

/** presetRule is the rule a preset writes, without an end; "" for never. */
export function presetRule(_preset: Exclude<RepeatPreset, "custom">): string {
  return notYet();
}

/** endOf reads when a rule stops; an UNTIL time is read as its date in timeZone. */
export function endOf(_rule: string, _timeZone: string): RepeatEnd {
  return notYet();
}

/** withEnd is rule ending as end says: on a date (its last day, in timeZone for a timed event) or after a count. */
export function withEnd(_rule: string, _end: RepeatEnd, _allDay: boolean, _timeZone: string): string {
  return notYet();
}

/** followStart moves the parts of a rule that name the start's day when the start moves from one date to another. */
export function followStart(_rule: string, _from: string, _to: string): string {
  return notYet();
}
