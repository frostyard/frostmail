// The event editor's draft (docs/specs/pim-ui.md, Event editor window): an
// event's editable fields as the form shows them, and the EventFields a
// save sends (ADR-0030, ADR-0033).
import type { CalendarEvent, EventFields } from "../rpc/gen/api";

/** EventDraft is an event as the editor shows it. Dates are YYYY-MM-DD and
 *  times HH:MM, in timeZone for a timed event; an all-day event's endDate
 *  is its last day. */
export interface EventDraft {
  summary: string;
  location: string;
  allDay: boolean;
  startDate: string;
  startTime: string;
  endDate: string;
  endTime: string;
  timeZone: string;
  /** The RRULE value; "" when it does not repeat. */
  rrule: string;
  calendarId: number;
  /** Minutes before the start (negative: after), in order. */
  alarms: number[];
  /** Show as Free. */
  transparent: boolean;
  description: string;
}

/** DraftErrors names each invalid field's problem. */
export type DraftErrors = Partial<Record<"start" | "end" | "timeZone", string>>;

function notYet(): never {
  throw new Error("lib/eventDraft comes with task card T-0147");
}

/** ruleOf is an event's RRULE value from its recurrence lines. */
export function ruleOf(_recurrence: string): string {
  return notYet();
}

/** draftOf is the editable copy of an event (or an occurrence), its times read in its own zone, else appZone. */
export function draftOf(_event: CalendarEvent, _appZone: string): EventDraft {
  return notYet();
}

/** newDraft is a new event's draft: its first times (instants, or the first day and the day after the last). */
export function newDraft(
  _times: { allDay: boolean; start: string; end: string; startDate: string; endDate: string },
  _calendarId: number,
  _appZone: string,
): EventDraft {
  return notYet();
}

/** instants are a timed draft's start and end. */
export function instants(_d: EventDraft): { start: Date; end: Date } {
  return notYet();
}

/** validate names what keeps a draft from being saved. */
export function validate(_d: EventDraft): DraftErrors {
  return notYet();
}

/** moveStart moves a draft's start, keeping its length, and a rule naming the old start's day follows. */
export function moveStart(_d: EventDraft, _startDate: string, _startTime?: string): EventDraft {
  return notYet();
}

/** changedFields is what a save sends: every field for a new event (empty text left out), else only what changed;
 *  a change to any of the times sends all of them. */
export function changedFields(_original: EventDraft | null, _d: EventDraft): EventFields {
  return notYet();
}
