// The event editor's draft (docs/specs/pim-ui.md, Event editor window): an
// event's editable fields as the form shows them, and the EventFields a
// save sends (ADR-0030, ADR-0033).
import type { CalendarEvent, EventFields, PartStat } from "../rpc/gen/api";
import { isValidAddress } from "./addressParse";
import { addDays, instantAt, zoned } from "./calendarDates";
import { followStart } from "./repeat";

/** DraftInvitee is an invitee's editable address and role, with their current answer. */
export interface DraftInvitee {
  email: string;
  name: string;
  role: "required" | "optional";
  answer?: PartStat;
}

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
  invitees?: DraftInvitee[];
}

/** DraftErrors names each invalid field's problem. */
export type DraftErrors = Partial<Record<"start" | "end" | "timeZone" | "invitees", string>>;

function wallTime(minutes: number): string {
  return `${String(Math.floor(minutes / 60)).padStart(2, "0")}:${String(minutes % 60).padStart(2, "0")}`;
}

function minutesOf(time: string): number {
  return Number(time.slice(0, 2)) * 60 + Number(time.slice(3, 5));
}

/** ruleOf is an event's RRULE value from its recurrence lines. */
export function ruleOf(recurrence: string): string {
  return (
    recurrence
      .split("\n")
      .find((line) => line.startsWith("RRULE:"))
      ?.slice(6)
      .trim() ?? ""
  );
}

/** draftOf is the editable copy of an event (or an occurrence), its times read in its own zone, else appZone. */
export function draftOf(event: CalendarEvent, appZone: string): EventDraft {
  const invitees: DraftInvitee[] = event.attendees
    .filter((attendee) => !attendee.isUser)
    .map(({ email, name, role, answer }) => ({
      email,
      name,
      role: role === "optional" ? "optional" : "required",
      answer,
    }));
  return {
    ...newDraft(event, event.calendarId, event.allDay ? appZone : event.timeZone || appZone),
    summary: event.summary,
    location: event.location,
    description: event.description,
    rrule: ruleOf(event.recurrence),
    alarms: [...event.alarms],
    transparent: event.transparent,
    ...(invitees.length > 0 ? { invitees } : {}),
  };
}

/** newDraft is a new event's draft: its first times (instants, or the first day and the day after the last). */
export function newDraft(
  times: { allDay: boolean; start: string; end: string; startDate: string; endDate: string },
  calendarId: number,
  appZone: string,
): EventDraft {
  const start = times.allDay ? { date: times.startDate, minutes: 540 } : zoned(times.start, appZone);
  const end = times.allDay ? { date: addDays(times.endDate, -1), minutes: 600 } : zoned(times.end, appZone);
  return {
    summary: "",
    location: "",
    description: "",
    allDay: times.allDay,
    startDate: start.date,
    startTime: wallTime(start.minutes),
    endDate: end.date,
    endTime: wallTime(end.minutes),
    timeZone: appZone,
    calendarId,
    rrule: "",
    alarms: [],
    transparent: false,
  };
}

/** instants are a timed draft's start and end. */
export function instants(d: EventDraft): { start: Date; end: Date } {
  return {
    start: instantAt(d.startDate, minutesOf(d.startTime), d.timeZone),
    end: instantAt(d.endDate, minutesOf(d.endTime), d.timeZone),
  };
}

/** validate names what keeps a draft from being saved. */
export function validate(d: EventDraft): DraftErrors {
  const errors = validateTimes(d);
  if (d.invitees?.some(({ email }) => !isValidAddress(email))) {
    errors.invitees = "Check the invitees' addresses";
  }
  return errors;
}

function validateTimes(d: EventDraft): DraftErrors {
  const dateShape = /^\d{4}-\d{2}-\d{2}$/;
  const timeShape = /^(?:[01]\d|2[0-3]):[0-5]\d$/;
  if (!dateShape.test(d.startDate) || (!d.allDay && !timeShape.test(d.startTime))) {
    return { start: "Enter a start" };
  }
  if (!dateShape.test(d.endDate) || (!d.allDay && !timeShape.test(d.endTime))) {
    return { end: "Enter an end" };
  }
  if (!d.allDay && !d.timeZone) return { timeZone: "Choose a time zone" };
  if (d.allDay) return d.endDate < d.startDate ? { end: "The end is before the start" } : {};
  const { start, end } = instants(d);
  return end <= start ? { end: "The end is before the start" } : {};
}

/** moveStart moves a draft's start, keeping its length, and a rule naming the old start's day follows. */
export function moveStart(d: EventDraft, startDate: string, startTime = d.startTime): EventDraft {
  const moved = { ...d, startDate, startTime, rrule: followStart(d.rrule, d.startDate, startDate) };
  if (d.allDay) {
    const days = (new Date(startDate).getTime() - new Date(d.startDate).getTime()) / 86_400_000;
    return { ...moved, endDate: addDays(d.endDate, days) };
  }
  const { start, end } = instants(d);
  const newStart = instantAt(startDate, minutesOf(startTime), d.timeZone);
  const newEnd = zoned(new Date(newStart.getTime() + end.getTime() - start.getTime()), d.timeZone);
  return { ...moved, endDate: newEnd.date, endTime: wallTime(newEnd.minutes) };
}

/** changedFields is what a save sends: every field for a new event (empty text left out), else only what changed;
 *  a change to any of the times sends all of them. */
export function changedFields(original: EventDraft | null, d: EventDraft): EventFields {
  const fields: EventFields = {};
  for (const key of ["summary", "location", "description", "rrule"] as const) {
    if (original ? original[key] !== d[key] : d[key] !== "") fields[key] = d[key];
  }
  if (original ? original.transparent !== d.transparent : d.transparent) fields.transparent = d.transparent;
  if (original && original.calendarId !== d.calendarId) fields.calendarId = d.calendarId;
  const alarmsChanged = original
    ? original.alarms.length !== d.alarms.length || original.alarms.some((alarm, index) => alarm !== d.alarms[index])
    : d.alarms.length > 0;
  if (alarmsChanged) fields.alarms = [...d.alarms];
  if (inviteesChanged(original, d)) {
    fields.attendees = (d.invitees ?? []).map(({ email, name, role }) => ({
      email,
      ...(name ? { name } : {}),
      role,
    }));
  }
  const timesChanged =
    !original ||
    original.allDay !== d.allDay ||
    original.startDate !== d.startDate ||
    original.endDate !== d.endDate ||
    (!d.allDay &&
      (original.startTime !== d.startTime || original.endTime !== d.endTime || original.timeZone !== d.timeZone));
  if (timesChanged) {
    fields.allDay = d.allDay;
    if (d.allDay) {
      fields.startDate = d.startDate;
      fields.endDate = addDays(d.endDate, 1);
    } else {
      const { start, end } = instants(d);
      fields.start = start.toISOString();
      fields.end = end.toISOString();
      fields.timeZone = d.timeZone;
    }
  }
  return fields;
}

function inviteesChanged(original: EventDraft | null, draft: EventDraft): boolean {
  const keys = (invitees: DraftInvitee[]) => invitees.map((i) => `${i.email.toLowerCase()}:${i.role}`).sort();
  const before = keys(original?.invitees ?? []);
  const after = keys(draft.invitees ?? []);
  return before.length !== after.length || before.some((key, index) => key !== after[index]);
}

/** sendsInvitees reports whether a save writes a nonempty invitee list. */
export function sendsInvitees(original: EventDraft | null, draft: EventDraft): boolean {
  return (draft.invitees?.length ?? 0) > 0 && inviteesChanged(original, draft);
}
