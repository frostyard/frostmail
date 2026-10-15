// Event patches, recurrence scopes and single-use restores (ADR-0030 to ADR-0032).
import { addDays } from "../../lib/calendarDates";
import {
  type Account,
  type Attendee,
  type CalendarEvent,
  type Collection,
  ErrorCode,
  type Event,
  type EventFields,
  type EventScope,
  type EventWrite,
  type Invitee,
  type InviteNotice,
} from "../gen/api";
import { RPCError } from "../transport";
import type { MockCalendarData, MockEvent, MockOverride } from "./calendar";

type Params = Record<string, unknown>;
type Changes = Omit<EventFields, "attendees"> & Partial<Pick<CalendarEvent, "attendees" | "organizer">>;
const DAY_MS = 86_400_000;
const TIME_FIELDS = ["allDay", "start", "end", "startDate", "endDate", "timeZone"] as const;

function invalid(message: string): never {
  throw new RPCError(ErrorCode.invalidParams, message);
}

function step(mock: MockEvent): number {
  return mock.every === "week" ? 7 : 1;
}

/** occurrenceKey names an occurrence by its original start, before overrides. */
export function occurrenceKey(mock: MockEvent, n: number): string {
  return mock.event.allDay
    ? addDays(mock.event.startDate, n * step(mock))
    : new Date(Date.parse(mock.event.start) + n * step(mock) * DAY_MS).toISOString();
}

function shift(event: CalendarEvent, days: number): void {
  event.start = new Date(Date.parse(event.start) + days * DAY_MS).toISOString();
  event.end = new Date(Date.parse(event.end) + days * DAY_MS).toISOString();
  if (event.allDay) {
    event.startDate = addDays(event.startDate, days);
    event.endDate = addDays(event.endDate, days);
  }
}

function date(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^\d{4}-\d{2}-\d{2}$/.test(value) &&
    Number.isFinite(Date.parse(value)) &&
    new Date(value).toISOString().slice(0, 10) === value
  );
}

function times(fields: EventFields): void {
  if (!TIME_FIELDS.some((key) => fields[key] !== undefined)) return;
  if (typeof fields.allDay !== "boolean") invalid("times require allDay");
  if (fields.allDay) {
    if (!date(fields.startDate) || !date(fields.endDate) || fields.startDate >= fields.endDate) {
      invalid("all-day dates must be valid and end after start");
    }
    fields.start = `${fields.startDate}T00:00:00.000Z`;
    fields.end = `${fields.endDate}T00:00:00.000Z`;
    fields.timeZone = "";
  } else {
    if (
      typeof fields.start !== "string" ||
      typeof fields.end !== "string" ||
      !Number.isFinite(Date.parse(fields.start)) ||
      !Number.isFinite(Date.parse(fields.end)) ||
      Date.parse(fields.start) >= Date.parse(fields.end)
    )
      invalid("end must be after start");
    if (typeof fields.timeZone !== "string" || !fields.timeZone) invalid("times require a time zone");
    try {
      new Intl.DateTimeFormat("en-US", { timeZone: fields.timeZone });
    } catch {
      invalid("unknown time zone");
    }
    fields.start = new Date(fields.start).toISOString();
    fields.end = new Date(fields.end).toISOString();
    fields.startDate = "";
    fields.endDate = "";
  }
}

function fields(value: unknown): EventFields {
  if (!value || typeof value !== "object" || Array.isArray(value)) invalid("fields must be an object");
  const strings = ["summary", "location", "description", "start", "end", "startDate", "endDate", "timeZone", "rrule"];
  for (const [key, item] of Object.entries(value)) {
    if (item === undefined) continue;
    if (strings.includes(key)) {
      if (typeof item !== "string") invalid(`${key} must be a string`);
    } else if (key === "allDay" || key === "transparent") {
      if (typeof item !== "boolean") invalid(`${key} must be a boolean`);
    } else if (key === "calendarId") {
      if (!Number.isSafeInteger(item) || Number(item) <= 0) invalid("invalid calendar ID");
    } else if (key === "alarms") {
      if (!Array.isArray(item) || item.some((alarm) => !Number.isSafeInteger(alarm))) invalid("invalid alarms");
    } else if (key === "attendees") {
      if (!Array.isArray(item)) invalid("attendees must be an array");
      for (const invitee of item) validateInvitee(invitee);
    } else invalid(`unknown field ${key}`);
  }
  const result = structuredClone(
    Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)),
  ) as EventFields;
  times(result);
  return result;
}

function validateInvitee(value: unknown): void {
  if (!value || typeof value !== "object" || Array.isArray(value)) invalid("invalid invitee");
  if (!("email" in value) || typeof value.email !== "string") invalid("invitee requires an email");
  for (const [key, item] of Object.entries(value)) {
    if (key === "email") continue;
    if (key === "name" && (item === undefined || typeof item === "string")) continue;
    if (key === "role" && (item === undefined || item === "required" || item === "optional")) continue;
    invalid("invalid invitee field");
  }
}

function invitees(event: CalendarEvent, given: Invitee[], account: Account | undefined): Attendee[] {
  const seen = new Set([account?.email.trim().toLowerCase()]);
  const result = event.attendees.filter((a) => a.isUser);
  for (const invitee of given) {
    const email = invitee.email.trim().toLowerCase();
    if (!/^[^\s@,;:<>]+@[^\s@,;:<>.]+(?:\.[^\s@,;:<>.]+)+$/.test(email)) invalid("invalid invitee address");
    if (seen.has(email)) continue;
    seen.add(email);
    const previous = event.attendees.find((a) => !a.isUser && a.email.trim().toLowerCase() === email);
    result.push({
      email,
      name: invitee.name ?? previous?.name ?? "",
      role: invitee.role ?? "required",
      answer: previous?.answer ?? "needsaction",
      isUser: false,
    });
  }
  return result;
}

function rule(mock: MockEvent, rrule: string | undefined): void {
  if (rrule === undefined) return;
  if (rrule === "") {
    delete mock.every;
    delete mock.count;
  } else {
    const parts = new Map<string, string>();
    for (const part of rrule.split(";")) {
      const [key, value, extra] = part.split("=");
      if (!key || !value || extra !== undefined || parts.has(key)) invalid("invalid recurrence rule");
      parts.set(key, value);
    }
    const frequency = parts.get("FREQ");
    if (frequency !== "DAILY" && frequency !== "WEEKLY") invalid("unsupported frequency");
    const count = parts.get("COUNT");
    if (count !== undefined && (!/^\d+$/.test(count) || !Number.isSafeInteger(Number(count)) || Number(count) < 1)) {
      invalid("invalid recurrence count");
    }
    mock.every = frequency === "WEEKLY" ? "week" : "day";
    mock.count = count === undefined ? 10 : Number(count);
  }
  mock.event.recurrence = rrule ? `RRULE:${rrule}` : "";
  mock.event.recurring = mock.every !== undefined;
}

function patch(mock: MockEvent, change: Changes): void {
  const { rrule, calendarId: _calendarId, ...values } = change;
  Object.assign(mock.event, values);
  rule(mock, rrule);
}

// Values that still match the original occurrence inherit later master edits.
// Explicitly rescheduled occurrences keep their own complete set of times.
function independentOverride(mock: MockEvent, index: number, override: MockOverride): MockOverride {
  const original = structuredClone(mock.event);
  shift(original, index * step(mock));
  const copy = structuredClone(override);
  if (TIME_FIELDS.every((key) => copy[key] === undefined || copy[key] === original[key])) {
    for (const key of TIME_FIELDS) delete copy[key];
  }
  for (const key of [
    "summary",
    "location",
    "description",
    "transparent",
    "alarms",
    "attendees",
    "organizer",
  ] as const) {
    if (JSON.stringify(copy[key]) === JSON.stringify(original[key])) delete copy[key];
  }
  return copy;
}

// Remap by occurrence index, so edits and exclusions follow their occurrences.
function remap(before: MockEvent, after: MockEvent, offset = 0): void {
  after.overrides = {};
  after.excluded = [];
  if (!after.every) return;
  const length = Math.min((before.count ?? 1) - offset, after.count ?? 1);
  for (let n = 0; n < length; n++) {
    const oldKey = occurrenceKey(before, n + offset);
    const newKey = occurrenceKey(after, n);
    const override = before.overrides?.[oldKey];
    if (override) after.overrides[newKey] = independentOverride(before, n + offset, override);
    if (before.excluded?.includes(oldKey)) after.excluded.push(newKey);
  }
}

function all(mock: MockEvent, change: Changes, index: number): void {
  const before = structuredClone(mock);
  patch(mock, change);
  if (change.allDay !== undefined) shift(mock.event, -index * step(mock));
  remap(before, mock);
}

function truncate(mock: MockEvent, count: number): void {
  const before = structuredClone(mock);
  mock.count = count;
  const value = mock.event.recurrence
    .replace(/^RRULE:/, "")
    .split(";")
    .filter((part) => !part.startsWith("COUNT="));
  mock.event.recurrence = `RRULE:${value.join(";")};COUNT=${count}`;
  remap(before, mock);
}

function eventCopies(mock: MockEvent): CalendarEvent[] {
  return Array.from({ length: mock.every ? (mock.count ?? 1) : 1 }, (_, index) => {
    const key = occurrenceKey(mock, index);
    if (mock.excluded?.includes(key)) return [];
    const event = structuredClone(mock.event);
    shift(event, index * step(mock));
    return [{ ...event, ...mock.overrides?.[key] }];
  }).flat();
}

function recipients(events: MockEvent[]): Set<string> {
  return new Set(
    events
      .flatMap(eventCopies)
      .flatMap((event) =>
        event.organizer && !event.organizer.isUser
          ? []
          : event.attendees.filter((a) => !a.isUser).map((a) => a.email.trim().toLowerCase()),
      ),
  );
}

function visibleFields(event: CalendarEvent): unknown[] {
  return (["summary", "location", "description", ...TIME_FIELDS, "recurrence", "status"] as const).map(
    (key) => event[key],
  );
}

// Compare effective occurrences, so an alerts-only override is still quiet.
function visibleEvents(events: MockEvent[]): string {
  return JSON.stringify(
    [...events]
      .sort((a, b) => a.event.id - b.event.id)
      .map((mock) => [
        mock.event.id,
        visibleFields(mock.event),
        mock.every,
        mock.count,
        [...(mock.excluded ?? [])].sort(),
        eventCopies(mock).map(visibleFields),
      ]),
  );
}

function notice(before: MockEvent[], after: MockEvent[], deleting: boolean): InviteNotice {
  const old = recipients(before);
  const next = recipients(after);
  if (deleting || after.length === 0) return { kind: "cancellations", count: old.size };
  if (before.length === 0) return { kind: "invitations", count: next.size };
  if (visibleEvents(before) !== visibleEvents(after)) {
    return { kind: "updates", count: new Set([...old, ...next]).size };
  }
  const added = [...next].filter((email) => !old.has(email));
  const removed = [...old].filter((email) => !next.has(email));
  return {
    kind: added.length && removed.length ? "updates" : removed.length ? "cancellations" : "invitations",
    count: added.length + removed.length,
  };
}

interface Restore {
  before?: MockEvent;
  position: number;
  created: number[];
  accounts: number[];
}

/** MockEventEdits applies event writes and keeps the objects each write replaced. */
export class MockEventEdits {
  private nextRestoreId = 1;
  private nextEventId = 301;
  private readonly restores = new Map<string, Restore>();

  constructor(
    private readonly data: MockCalendarData,
    private readonly collections: Collection[],
    private readonly emit: (event: Event) => void,
    private readonly read: (id: number, recurrenceId?: string) => CalendarEvent,
    private readonly accounts: () => Account[] = () => [],
  ) {}

  /** create adds an event to a named calendar or the default writable calendar. */
  create(p: Params): EventWrite {
    const change = fields(p.fields);
    if (change.allDay === undefined) invalid("new events require times");
    const id = p.calendarId ?? change.calendarId;
    const writable = this.collections.filter((c) => c.kind === "calendar" && c.enabled && !c.readOnly);
    const calendar = id === undefined ? (writable.find((c) => c.isDefault) ?? writable[0]) : this.calendar(Number(id));
    if (!calendar) throw new RPCError(ErrorCode.notFound, "no writable calendar");
    const mock: MockEvent = {
      event: {
        id: this.nextId(),
        uid: "",
        recurrenceId: "",
        calendarId: calendar.id,
        accountId: calendar.accountId,
        summary: "",
        location: "",
        description: "",
        allDay: false,
        start: "",
        end: "",
        startDate: "",
        endDate: "",
        timeZone: "",
        recurring: false,
        recurrence: "",
        status: "confirmed",
        transparent: false,
        attendees: [],
        alarms: [],
        readOnly: false,
      },
    };
    mock.event.uid = `mock-${mock.event.id}`;
    patch(mock, this.changes(mock.event, change, calendar));
    this.confirm(p, [], [mock]);
    const restoreId = this.keep(undefined, [mock]);
    this.data.events.push(mock);
    return this.written(restoreId, mock.event.id);
  }

  /** update patches a single event, an occurrence, the whole series or its future. */
  update(p: Params): EventWrite {
    const original = this.writable(Number(p.id));
    const given = fields(p.fields);
    const invited = original.event.organizer && !original.event.organizer.isUser;
    if (invited && (p.scope === "future" || Object.keys(given).some((key) => key !== "alarms"))) {
      invalid("invitations take only alarms and cannot split");
    }
    const { index, scope } = this.scope(original, p);
    const calendar = this.calendar(original.event.calendarId);
    const source = scope === "all" ? original.event : this.read(original.event.id, occurrenceKey(original, index));
    const change = this.changes(source, given, calendar);
    if (
      calendar.invites &&
      change.calendarId !== undefined &&
      change.calendarId !== calendar.id &&
      (recipients([original]).size || change.attendees?.some((a) => !a.isUser))
    ) {
      invalid("events with invitees cannot move calendars");
    }
    if (original.every && change.calendarId !== undefined && scope !== "all") invalid("move requires all events");
    const destination = change.calendarId === undefined ? undefined : this.calendar(change.calendarId);
    const mock = structuredClone(original);
    let result = mock;
    let recurrenceId: string | undefined;
    if (!mock.every) patch(mock, change);
    else if (scope === "this") {
      if (change.rrule !== undefined) invalid("one occurrence cannot change the rule");
      recurrenceId = occurrenceKey(mock, index);
      const { rrule: _rrule, calendarId: _calendarId, ...values } = change;
      mock.overrides ??= {};
      mock.overrides[recurrenceId] = { ...mock.overrides[recurrenceId], ...values };
    } else if (scope === "future") {
      result = this.split(mock, index);
      all(result, change, 0);
    } else all(mock, change, index);
    if (destination) {
      result.event.calendarId = destination.id;
      result.event.accountId = destination.accountId;
    }
    const after = result === mock ? [mock] : [mock, result];
    this.confirm(p, [original], after);
    const restoreId = this.keep(original, after);
    this.data.events[this.data.events.indexOf(original)] = mock;
    if (result !== mock) this.data.events.push(result);
    return this.written(restoreId, result.event.id, recurrenceId);
  }

  /** delete removes an event, excludes an occurrence or truncates the series. */
  delete(p: Params): EventWrite {
    const original = this.writable(Number(p.id));
    if (original.every && original.event.organizer && !original.event.organizer.isUser && p.scope !== "all") {
      invalid("an invitation series can only be deleted in full");
    }
    const { index, scope } = this.scope(original, p);
    const mock = structuredClone(original);
    const remove = !mock.every || scope === "all";
    if (!remove && scope === "this") {
      const key = occurrenceKey(mock, index);
      mock.excluded = [...(mock.excluded ?? []), key];
      if (mock.overrides) delete mock.overrides[key];
    } else if (!remove) truncate(mock, index);
    this.confirm(p, [original], remove ? [] : [mock], true);
    const restoreId = this.keep(original, remove ? [] : [mock]);
    this.data.events.splice(this.data.events.indexOf(original), 1, ...(remove ? [] : [mock]));
    return this.written(restoreId);
  }

  /** restore puts back the saved event at its old position and removes creations. */
  restore(p: Params): null {
    const restoreId = String(p.restoreId);
    const saved = this.restores.get(restoreId);
    if (!saved) throw new RPCError(ErrorCode.notFound, "restore does not exist");
    if (saved.before) this.calendar(saved.before.event.calendarId);
    const ids = [...saved.created, ...(saved.before ? [saved.before.event.id] : [])];
    const current = this.data.events.filter((mock) => ids.includes(mock.event.id));
    for (const mock of current) this.writable(mock.event.id);
    this.confirm(p, current, saved.before ? [saved.before] : []);
    this.data.events = this.data.events.filter((mock) => !ids.includes(mock.event.id));
    if (saved.before) this.data.events.splice(saved.position, 0, structuredClone(saved.before));
    this.restores.delete(restoreId);
    this.announce([...saved.accounts, ...current.map((mock) => mock.event.accountId)]);
    return null;
  }

  private changes(event: CalendarEvent, given: EventFields, calendar: Collection): Changes {
    const { attendees, ...change } = given;
    if (attendees === undefined) return change;
    const account = this.accounts().find((a) => a.id === calendar.accountId);
    const result: Changes = { ...change, attendees: invitees(event, attendees, account) };
    if (result.attendees?.some((a) => !a.isUser)) {
      if (!calendar.invites) invalid("calendar does not support invitees");
      if (!event.organizer) {
        if (!account) invalid("calendar account is missing");
        result.organizer = {
          email: account.email.trim().toLowerCase(),
          name: account.displayName,
          role: "chair",
          answer: "accepted",
          isUser: true,
        };
      }
    }
    return result;
  }

  private confirm(p: Params, before: MockEvent[], after: MockEvent[], deleting = false): void {
    const scheduling = (events: MockEvent[]) =>
      events.filter(
        (mock) =>
          (!mock.event.organizer || mock.event.organizer.isUser) &&
          this.collections.some((c) => c.id === mock.event.calendarId && c.invites),
      );
    const data = notice(scheduling(before), scheduling(after), deleting);
    if (!data.count || p.notify === true) return;
    if (p.notify !== undefined) invalid("notifying invitees requires notify: true");
    throw new RPCError(ErrorCode.notifyRequired, "this change tells invitees", data);
  }

  private calendar(id: number): Collection {
    const calendar = this.collections.find((c) => c.id === id && c.kind === "calendar");
    if (!calendar) throw new RPCError(ErrorCode.notFound, "calendar does not exist");
    if (calendar.readOnly || !calendar.enabled) throw new RPCError(ErrorCode.conflict, "calendar is not writable");
    return calendar;
  }

  private writable(id: number): MockEvent {
    const mock = this.data.events.find((m) => m.event.id === id);
    if (!mock) throw new RPCError(ErrorCode.notFound, "event does not exist");
    if (mock.event.readOnly) throw new RPCError(ErrorCode.conflict, "event is read-only");
    this.calendar(mock.event.calendarId);
    return mock;
  }

  private scope(mock: MockEvent, p: Params): { index: number; scope: EventScope } {
    if (p.scope !== undefined && p.scope !== "this" && p.scope !== "all" && p.scope !== "future")
      invalid("invalid scope");
    if (!mock.every) {
      if (p.recurrenceId) invalid("single event has no recurrence ID");
      return { index: 0, scope: "all" };
    }
    if (typeof p.recurrenceId !== "string" || !p.recurrenceId || p.scope === undefined) {
      invalid("series requires a recurrence ID and scope");
    }
    for (let index = 0; index < (mock.count ?? 1); index++) {
      if (occurrenceKey(mock, index) === p.recurrenceId && !mock.excluded?.includes(p.recurrenceId)) {
        return { index, scope: p.scope === "future" && index === 0 ? "all" : p.scope };
      }
    }
    throw new RPCError(ErrorCode.notFound, "occurrence does not exist");
  }

  private split(mock: MockEvent, index: number): MockEvent {
    const next = structuredClone(mock);
    next.event.id = this.nextId();
    next.event.uid = `mock-${next.event.id}`;
    shift(next.event, index * step(mock));
    next.count = (mock.count ?? 1) - index;
    remap(mock, next, index);
    truncate(next, next.count);
    truncate(mock, index);
    return next;
  }

  private nextId(): number {
    return Math.max(this.nextEventId, ...this.data.events.map((m) => m.event.id + 1));
  }

  private keep(before: MockEvent | undefined, after: MockEvent[]): string {
    this.nextEventId = Math.max(this.nextEventId, ...after.map((mock) => mock.event.id + 1));
    const restoreId = `event-restore-${this.nextRestoreId++}`;
    this.restores.set(
      restoreId,
      structuredClone({
        before,
        position: before ? this.data.events.indexOf(before) : this.data.events.length,
        created: after.filter((mock) => mock.event.id !== before?.event.id).map((mock) => mock.event.id),
        accounts: [...(before ? [before.event.accountId] : []), ...after.map((mock) => mock.event.accountId)],
      }),
    );
    return restoreId;
  }

  private announce(accounts: number[]): void {
    for (const accountId of new Set(accounts)) this.emit({ event: "calendar.changed", data: { accountId } });
  }

  private written(restoreId: string, id?: number, recurrenceId?: string): EventWrite {
    this.announce(this.restores.get(restoreId)?.accounts ?? []);
    return { restoreId, ...(id === undefined ? {} : { event: this.read(id, recurrenceId) }) };
  }
}
