// The calendar domain of MockTransport: the fixture's events and their
// occurrences, as maild answers calendar.range and calendar.event
// (docs/specs/pim-ui.md; internal/engine/calendar.go).
import { addDays, dayStart } from "../../lib/calendarDates";
import {
  type CalendarEvent,
  type Collection,
  ErrorCode,
  type Event,
  type Invitation,
  type Occurrence,
  type Reminder,
} from "../gen/api";
import { RPCError } from "../transport";
import { NOT_HANDLED } from "./compose";

/** MockEvent is a stored event: the CalendarEvent of its first occurrence
 *  (recurrenceId ""), repeating every day or week `count` times when
 *  `every` is set. */
export interface MockEvent {
  event: CalendarEvent;
  every?: "day" | "week";
  count?: number;
}

/** MockReminder is a fired reminder of one occurrence of an event (its
 *  recurrenceId as calendar.range names it), due at dueAt. */
export interface MockReminder {
  id: string;
  eventId: number;
  recurrenceId: string;
  dueAt: string;
  snoozedUntil?: string;
  dismissed?: boolean;
}

/** MockInvitation links an invitation message to its stored event. */
export interface MockInvitation {
  messageId: number;
  eventId: number;
}

/** MockCalendarData is the events the mock serves; their calendars are
 *  collections in MockPeopleData. */
export interface MockCalendarData {
  events: MockEvent[];
  invitations?: MockInvitation[];
  /** Fired reminders; none when absent. */
  reminders?: MockReminder[];
  /** The clock contact cards' upcoming occurrences count from. */
  now: string;
}

const DAY_MS = 86_400_000;
type Params = Record<string, unknown>;

function validDate(value: unknown): value is string {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const parsed = new Date(value);
  return Number.isFinite(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value;
}

function occurrences(mock: MockEvent, zone: string): Occurrence[] {
  const e = mock.event;
  return Array.from({ length: mock.every ? (mock.count ?? 1) : 1 }, (_, index) => {
    const days = index * (mock.every === "week" ? 7 : 1);
    const startDate = e.allDay ? addDays(e.startDate, days) : "";
    const endDate = e.allDay ? addDays(e.endDate, days) : "";
    const start = e.allDay
      ? dayStart(startDate, zone).toISOString()
      : new Date(Date.parse(e.start) + days * DAY_MS).toISOString();
    const end = e.allDay
      ? dayStart(endDate, zone).toISOString()
      : new Date(Date.parse(e.end) + days * DAY_MS).toISOString();
    return {
      eventId: e.id,
      recurrenceId: mock.every ? (e.allDay ? startDate : start) : "",
      calendarId: e.calendarId,
      accountId: e.accountId,
      summary: e.summary,
      location: e.location,
      allDay: e.allDay,
      start,
      end,
      startDate,
      endDate,
      status: e.status,
      ...(e.answer === undefined ? {} : { answer: e.answer }),
      transparent: e.transparent,
      recurring: mock.every !== undefined,
    };
  });
}

/** MockCalendar answers calendar queries and updates shared collections. */
export class MockCalendar {
  constructor(
    private readonly data: MockCalendarData,
    private readonly collections: Collection[],
    private readonly emit: (event: Event) => void,
  ) {}

  /** dispatch answers supported methods or returns NOT_HANDLED. */
  dispatch(method: string, params: Params): unknown {
    switch (method) {
      case "calendar.invitation":
        return this.invitation(Number(params.messageId));
      case "calendar.respond":
        return this.respond(params);
      case "calendar.range":
        return this.range(params);
      case "calendar.event":
        return this.event(Number(params.id), params.recurrenceId);
      case "calendar.reminders":
        return this.reminders();
      case "calendar.snooze":
      case "calendar.dismiss":
        return this.changeReminders(method, params);
      case "account.setCollection":
        return this.setCollection(params);
      default:
        return NOT_HANDLED;
    }
  }

  private invitationEvent(messageId: number): number {
    const invitation = this.data.invitations?.find((inv) => inv.messageId === messageId);
    if (!invitation) throw new RPCError(ErrorCode.notFound, "message has no invitation");
    return invitation.eventId;
  }

  private invitation(messageId: number): Invitation {
    const eventId = this.invitationEvent(messageId);
    const event = this.event(eventId, "");
    const day = event.allDay ? event.startDate : event.start.slice(0, 10);
    const others = this.data.events
      .filter((mock) => mock.event.id !== eventId)
      .flatMap((mock) => occurrences(mock, "UTC"))
      .filter((o) => !o.allDay && o.start.slice(0, 10) === day);
    const start = Date.parse(event.start),
      end = Date.parse(event.end);
    const before = others
      .filter((o) => Date.parse(o.end) <= start)
      .sort((a, b) => Date.parse(b.end) - Date.parse(a.end))[0];
    const after = others
      .filter((o) => Date.parse(o.start) >= end)
      .sort((a, b) => Date.parse(a.start) - Date.parse(b.start))[0];
    return {
      method: "request",
      event: { ...event, id: 0 },
      eventId,
      ...(event.organizer ? { from: event.organizer } : {}),
      ...(event.answer === undefined ? {} : { answer: event.answer }),
      canRespond: !event.readOnly,
      outdated: false,
      conflicts: others.filter(
        (o) => !o.transparent && o.answer !== "declined" && Date.parse(o.start) < end && Date.parse(o.end) > start,
      ),
      adjacent: [...(before ? [before] : []), ...(after ? [after] : [])],
    };
  }

  private respond(p: Params): CalendarEvent {
    const answer = p.answer;
    if (
      (answer !== "accepted" && answer !== "declined" && answer !== "tentative") ||
      (p.messageId === undefined && p.eventId === undefined)
    ) {
      throw new RPCError(ErrorCode.invalidParams, "answer and an invitation ID are required");
    }
    const id = p.messageId === undefined ? Number(p.eventId) : this.invitationEvent(Number(p.messageId));
    const mock = this.data.events.find((e) => e.event.id === id);
    if (!mock) throw new RPCError(ErrorCode.notFound, "event does not exist");
    mock.event.answer = answer;
    for (const attendee of mock.event.attendees) if (attendee.isUser) attendee.answer = answer;
    this.emit({ event: "calendar.changed", data: { accountId: mock.event.accountId } });
    return this.event(id, "");
  }

  private reminders(): Reminder[] {
    const now = Date.parse(this.data.now);
    return (this.data.reminders ?? [])
      .filter((r) => !r.dismissed && (r.snoozedUntil === undefined || Date.parse(r.snoozedUntil) <= now))
      .flatMap((r) => {
        if (!this.data.events.some((mock) => mock.event.id === r.eventId)) return [];
        const event = this.event(r.eventId, r.recurrenceId);
        return [
          {
            id: r.id,
            eventId: r.eventId,
            recurrenceId: r.recurrenceId,
            calendarId: event.calendarId,
            summary: event.summary,
            location: event.location,
            allDay: event.allDay,
            start: event.start,
            startDate: event.startDate,
            dueAt: r.snoozedUntil ?? r.dueAt,
          },
        ];
      })
      .sort((a, b) => Date.parse(a.dueAt) - Date.parse(b.dueAt));
  }

  private changeReminders(method: string, params: Params): null {
    const ids = params.ids;
    if (!Array.isArray(ids) || ids.length === 0 || ids.some((id) => typeof id !== "string")) {
      throw new RPCError(ErrorCode.invalidParams, "ids must be a non-empty list");
    }
    const until = params.until;
    if (method === "calendar.snooze" && (typeof until !== "string" || !Number.isFinite(Date.parse(until)))) {
      throw new RPCError(ErrorCode.invalidParams, "until must be a time");
    }
    for (const r of this.data.reminders ?? []) {
      if (!ids.includes(r.id)) continue;
      if (method === "calendar.dismiss") r.dismissed = true;
      else if (typeof until === "string") r.snoozedUntil = until;
    }
    this.emit({ event: "calendar.reminders", data: { count: this.reminders().length } });
    return null;
  }

  private visible(): MockEvent[] {
    return this.data.events.filter((e) =>
      this.collections.some((c) => c.id === e.event.calendarId && c.kind === "calendar" && c.enabled),
    );
  }

  private sorted(list: Occurrence[]): Occurrence[] {
    return list.sort(
      (a, b) =>
        Date.parse(a.start) - Date.parse(b.start) || Number(b.allDay) - Number(a.allDay) || a.eventId - b.eventId,
    );
  }

  private range(p: Params): Occurrence[] {
    if (!validDate(p.from) || !validDate(p.to)) {
      throw new RPCError(ErrorCode.invalidParams, "dates must be YYYY-MM-DD");
    }
    const fromDate = p.from;
    const toDate = p.to;
    const days = (Date.parse(toDate) - Date.parse(fromDate)) / DAY_MS;
    if (days < 1 || days > 400) throw new RPCError(ErrorCode.invalidParams, "to must be 1 to 400 days after from");
    const zone = p.timeZone === undefined ? "UTC" : p.timeZone;
    try {
      if (typeof zone !== "string" || zone === "") throw new Error("invalid zone");
      new Intl.DateTimeFormat("en-US", { timeZone: zone });
    } catch {
      throw new RPCError(ErrorCode.invalidParams, "unknown time zone");
    }
    if (typeof zone !== "string") throw new RPCError(ErrorCode.invalidParams, "unknown time zone");
    const from = dayStart(p.from, zone).getTime();
    const to = dayStart(p.to, zone).getTime();
    const calendarIds = p.calendarIds;
    return this.sorted(
      this.visible()
        .filter((e) => !Array.isArray(calendarIds) || calendarIds.includes(e.event.calendarId))
        .flatMap((e) => occurrences(e, zone))
        .filter((o) =>
          o.allDay
            ? o.startDate < toDate && o.endDate > fromDate
            : Date.parse(o.start) < to &&
              (Date.parse(o.end) > from || (o.start === o.end && Date.parse(o.start) >= from)),
        ),
    );
  }

  private event(id: number, recurrenceId: unknown): CalendarEvent {
    const mock = this.data.events.find((e) => e.event.id === id);
    if (!mock) throw new RPCError(ErrorCode.notFound, `event ${id} does not exist`);
    const copy = structuredClone(mock.event);
    if (recurrenceId === undefined || recurrenceId === "") return copy;
    const one = occurrences(mock, "UTC").find((o) => o.recurrenceId === recurrenceId);
    if (!one) throw new RPCError(ErrorCode.notFound, "occurrence does not exist");
    return {
      ...copy,
      recurrenceId: one.recurrenceId,
      start: one.start,
      end: one.end,
      startDate: one.startDate,
      endDate: one.endDate,
    };
  }

  private setCollection(p: Params): Collection {
    const c = this.collections.find((c) => c.id === p.id);
    if (!c) throw new RPCError(ErrorCode.notFound, `collection ${p.id} does not exist`);
    if (p.isDefault === true) {
      for (const other of this.collections) {
        if (other.accountId === c.accountId && other.kind === c.kind) other.isDefault = other.id === c.id;
      }
    }
    if (typeof p.enabled === "boolean" && p.enabled !== c.enabled) {
      c.enabled = p.enabled;
      if (c.kind === "calendar") this.emit({ event: "calendar.changed", data: { accountId: c.accountId } });
      if (c.kind === "addressbook") this.emit({ event: "people.changed", data: { accountId: c.accountId } });
      if (c.kind === "tasklist") this.emit({ event: "tasks.changed", data: { accountId: c.accountId } });
    }
    this.emit({ event: "account.changed", data: { id: c.accountId, deleted: false } });
    return { ...c };
  }

  /** upcoming returns at most five visible occurrences involving this address. */
  upcoming(email: string): Occurrence[] {
    const from = Date.parse(this.data.now);
    const to = from + 30 * DAY_MS;
    const address = email.trim().toLowerCase();
    return this.sorted(
      this.visible()
        .filter(
          ({ event: e }) =>
            e.organizer?.email.toLowerCase() === address || e.attendees.some((a) => a.email.toLowerCase() === address),
        )
        .flatMap((e) => occurrences(e, "UTC"))
        .filter((o) => Date.parse(o.start) >= from && Date.parse(o.start) < to),
    ).slice(0, 5);
  }
}
