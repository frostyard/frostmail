// The reminder window's words (docs/specs/pim-ui.md, Reminder window).
// Task T-0076 writes them.
import type { Reminder } from "../rpc/gen/api";

/** SnoozeChoice is one entry of the Snooze menu. */
export interface SnoozeChoice {
  label: string;
  until: Date;
}

/** reminderWhen says when a reminder's occurrence starts, relative to now. */
export function reminderWhen(_r: Reminder, _now: Date, _timeZone: string, _locale: string): string {
  return "";
}

/** snoozeChoices are 5, 10 and 15 minutes, an hour, and 9:00 tomorrow in the zone. */
export function snoozeChoices(_now: Date, _timeZone: string): SnoozeChoice[] {
  return [];
}
