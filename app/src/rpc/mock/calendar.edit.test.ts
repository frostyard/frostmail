// CONTRACT TEST for task card T-0144 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { Client, ErrorCode } from "../gen/api";
import { CALENDARS, mockData } from "./fixture";
import { MockTransport } from "./mock";

// The fixture's day is 2026-10-08 (UTC). Events: 302 Design review, which
// the user organizes, today 14:00–15:30; 303 Lunch with Ann, an
// invitation, tomorrow; 304 Holiday in the read-only Holidays calendar.
function client() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  const events: string[] = [];
  mock.onEvent((e) => events.push(e.event));
  return { mock, c: new Client(mock), events };
}

async function code(p: Promise<unknown>): Promise<number | undefined> {
  try {
    await p;
    return undefined;
  } catch (err) {
    return (err as { code?: number }).code;
  }
}

const hour = (iso: string) => new Date(iso).getUTCHours();

async function day(c: Client, from: string, to: string, summary?: string) {
  const all = await c.calendar.range({ from, to, timeZone: "UTC" });
  return summary === undefined ? all : all.filter((o) => o.summary.startsWith(summary));
}

// A weekly series the user made: Tuesdays 10:00–10:30 UTC from October 13,
// four times. Its recurrence IDs are the occurrences' original starts.
async function series(c: Client) {
  const w = await c.calendar.create({
    fields: {
      summary: "Team sync",
      allDay: false,
      start: "2026-10-13T10:00:00.000Z",
      end: "2026-10-13T10:30:00.000Z",
      timeZone: "UTC",
      rrule: "FREQ=WEEKLY;COUNT=4",
    },
  });
  const id = w.event?.id ?? 0;
  return { id, keys: ["13", "20", "27"].map((d) => `2026-10-${d}T10:00:00.000Z`).concat("2026-11-03T10:00:00.000Z") };
}

const SPAN = ["2026-10-12", "2026-11-10"] as const;

describe("MockTransport event writes", () => {
  it("creates events in the default calendar or the one named, and announces them", async () => {
    const { c, events } = client();
    const w = await c.calendar.create({
      fields: {
        summary: "Planning",
        allDay: false,
        start: "2026-10-09T15:00:00Z",
        end: "2026-10-09T16:00:00Z",
        timeZone: "Europe/Berlin",
        alarms: [10],
      },
    });
    expect(w.event).toMatchObject({ calendarId: CALENDARS.work, summary: "Planning", timeZone: "Europe/Berlin" });
    expect(w.event?.alarms).toEqual([10]);
    expect(typeof w.restoreId).toBe("string");
    expect((await day(c, "2026-10-09", "2026-10-10", "Planning")).map((o) => o.start)).toEqual([
      "2026-10-09T15:00:00.000Z",
    ]);
    await Promise.resolve();
    expect(events).toContain("calendar.changed");

    const trip = await c.calendar.create({
      calendarId: CALENDARS.home,
      fields: { summary: "Trip", allDay: true, startDate: "2026-10-12", endDate: "2026-10-14" },
    });
    expect(trip.event).toMatchObject({ calendarId: CALENDARS.home, allDay: true, startDate: "2026-10-12" });
    const [o] = await day(c, "2026-10-13", "2026-10-14", "Trip");
    expect(o).toMatchObject({ allDay: true, startDate: "2026-10-12", endDate: "2026-10-14" });
  });

  it("refuses events it cannot write", async () => {
    const { c } = client();
    const timed = { allDay: false, start: "2026-10-09T15:00:00Z", end: "2026-10-09T16:00:00Z", timeZone: "UTC" };
    for (const fields of [
      { summary: "no times" },
      { ...timed, end: "2026-10-09T14:00:00Z" },
      { ...timed, timeZone: "Mars/Olympus" },
      { allDay: true, startDate: "2026-10-12" },
      { ...timed, rrule: "FREQ=MONTHLY" },
    ]) {
      expect(await code(c.calendar.create({ fields }))).toBe(ErrorCode.invalidParams);
    }
    expect(await code(c.calendar.create({ calendarId: CALENDARS.holidays, fields: timed }))).toBe(ErrorCode.conflict);
    expect(await code(c.calendar.create({ calendarId: 999, fields: timed }))).toBe(ErrorCode.notFound);
    expect(await code(c.calendar.update({ id: 304, fields: { summary: "x" } }))).toBe(ErrorCode.conflict);
    expect(await code(c.calendar.update({ id: 999, fields: { summary: "x" } }))).toBe(ErrorCode.notFound);
  });

  it("changes an event, and restores it once", async () => {
    const { c } = client();
    const before = await c.calendar.event({ id: 302 });
    const w = await c.calendar.update({
      id: 302,
      fields: {
        summary: "Design review (v2)",
        allDay: false,
        start: "2026-10-08T16:00:00Z",
        end: "2026-10-08T17:00:00Z",
        timeZone: "UTC",
        transparent: true,
      },
    });
    expect(w.event).toMatchObject({ id: 302, summary: "Design review (v2)", transparent: true });
    expect((await day(c, "2026-10-08", "2026-10-09", "Design review")).map((o) => hour(o.start))).toEqual([16]);
    await c.calendar.restore({ restoreId: w.restoreId });
    expect(await c.calendar.event({ id: 302 })).toEqual(before);
    expect(await code(c.calendar.restore({ restoreId: w.restoreId }))).toBe(ErrorCode.notFound);
  });

  it("takes only alerts for an invitation", async () => {
    const { c } = client();
    expect(await code(c.calendar.update({ id: 303, fields: { summary: "Lunch" } }))).toBe(ErrorCode.invalidParams);
    const w = await c.calendar.update({ id: 303, fields: { alarms: [30] } });
    expect(w.event?.alarms).toEqual([30]);
  });

  it("changes one occurrence of a series", async () => {
    const { c } = client();
    const { id, keys } = await series(c);
    expect((await day(c, ...SPAN, "Team sync")).map((o) => o.recurrenceId)).toEqual(keys);
    expect(await code(c.calendar.update({ id, fields: { summary: "x" } }))).toBe(ErrorCode.invalidParams);
    expect(
      await code(c.calendar.update({ id, recurrenceId: "2026-10-14T10:00:00.000Z", scope: "this", fields: {} })),
    ).toBe(ErrorCode.notFound);
    expect(
      await code(c.calendar.update({ id, recurrenceId: keys[1], scope: "this", fields: { rrule: "FREQ=DAILY" } })),
    ).toBe(ErrorCode.invalidParams);

    const w = await c.calendar.update({
      id,
      recurrenceId: keys[1],
      scope: "this",
      fields: {
        summary: "Team sync (late)",
        allDay: false,
        start: "2026-10-20T11:00:00.000Z",
        end: "2026-10-20T11:30:00.000Z",
        timeZone: "UTC",
      },
    });
    expect(w.event).toMatchObject({ id, recurrenceId: keys[1], summary: "Team sync (late)" });
    const occs = await day(c, ...SPAN, "Team sync");
    expect(occs.map((o) => [o.recurrenceId, o.summary, hour(o.start)])).toEqual([
      [keys[0], "Team sync", 10],
      [keys[1], "Team sync (late)", 11],
      [keys[2], "Team sync", 10],
      [keys[3], "Team sync", 10],
    ]);
    expect((await c.calendar.event({ id, recurrenceId: keys[1] })).summary).toBe("Team sync (late)");
  });

  it("moves a whole series from one of its occurrences", async () => {
    const { c } = client();
    const { id, keys } = await series(c);
    await c.calendar.update({ id, recurrenceId: keys[1], scope: "this", fields: { location: "Room 9" } });
    await c.calendar.update({
      id,
      recurrenceId: keys[2],
      scope: "all",
      fields: {
        summary: "Sync",
        allDay: false,
        start: "2026-10-27T12:00:00.000Z",
        end: "2026-10-27T13:00:00.000Z",
        timeZone: "UTC",
      },
    });
    const occs = await day(c, ...SPAN, "Sync");
    expect(occs.map((o) => [o.recurrenceId.slice(0, 13), hour(o.start), hour(o.end), o.location])).toEqual([
      ["2026-10-13T12", 12, 13, ""],
      ["2026-10-20T12", 12, 13, "Room 9"],
      ["2026-10-27T12", 12, 13, ""],
      ["2026-11-03T12", 12, 13, ""],
    ]);

    // A new rule drops what it no longer has.
    await c.calendar.update({
      id,
      recurrenceId: occs[0]?.recurrenceId,
      scope: "all",
      fields: { rrule: "FREQ=WEEKLY;COUNT=1" },
    });
    expect(await day(c, ...SPAN, "Sync")).toHaveLength(1);
  });

  it("splits a series at an occurrence, and restores both halves", async () => {
    const { c } = client();
    const { id, keys } = await series(c);
    const w = await c.calendar.update({
      id,
      recurrenceId: keys[2],
      scope: "future",
      fields: { summary: "Team sync v2" },
    });
    const next = w.event?.id ?? 0;
    expect(next).not.toBe(id);
    expect((await day(c, ...SPAN, "Team sync")).map((o) => [o.eventId, o.summary])).toEqual([
      [id, "Team sync"],
      [id, "Team sync"],
      [next, "Team sync v2"],
      [next, "Team sync v2"],
    ]);
    await c.calendar.restore({ restoreId: w.restoreId });
    expect((await day(c, ...SPAN, "Team sync")).map((o) => [o.eventId, o.summary])).toEqual(
      keys.map(() => [id, "Team sync"]),
    );
    expect(await code(c.calendar.event({ id: next }))).toBe(ErrorCode.notFound);

    // From the first occurrence, All Future Events is All Events.
    const all = await c.calendar.update({ id, recurrenceId: keys[0], scope: "future", fields: { summary: "Sync" } });
    expect(all.event?.id).toBe(id);
    expect(await day(c, ...SPAN, "Sync")).toHaveLength(4);
  });

  it("deletes events and occurrences, and restores them", async () => {
    const { c } = client();
    const { id, keys } = await series(c);
    const one = await c.calendar.delete({ id, recurrenceId: keys[1], scope: "this" });
    expect(one.event).toBeUndefined();
    expect((await day(c, ...SPAN, "Team sync")).map((o) => o.recurrenceId)).toEqual([keys[0], keys[2], keys[3]]);
    await c.calendar.delete({ id, recurrenceId: keys[2], scope: "future" });
    expect((await day(c, ...SPAN, "Team sync")).map((o) => o.recurrenceId)).toEqual([keys[0]]);
    // A restore writes back the series as that write found it.
    await c.calendar.restore({ restoreId: one.restoreId });
    expect((await day(c, ...SPAN, "Team sync")).map((o) => o.recurrenceId)).toEqual(keys);
    await c.calendar.delete({ id, recurrenceId: keys[0], scope: "future" });
    expect(await day(c, ...SPAN, "Team sync")).toEqual([]);

    const gone = await c.calendar.delete({ id: 302 });
    expect(await day(c, "2026-10-08", "2026-10-09", "Design review")).toEqual([]);
    await c.calendar.restore({ restoreId: gone.restoreId });
    expect(await day(c, "2026-10-08", "2026-10-09", "Design review")).toHaveLength(1);
  });

  it("moves an event to another calendar", async () => {
    const { c } = client();
    const w = await c.calendar.update({ id: 302, fields: { calendarId: CALENDARS.home } });
    expect(w.event?.calendarId).toBe(CALENDARS.home);
    expect(await code(c.calendar.update({ id: 302, fields: { calendarId: CALENDARS.holidays } }))).toBe(
      ErrorCode.conflict,
    );
    const { id, keys } = await series(c);
    expect(
      await code(
        c.calendar.update({ id, recurrenceId: keys[1], scope: "this", fields: { calendarId: CALENDARS.home } }),
      ),
    ).toBe(ErrorCode.invalidParams);
    await c.calendar.restore({ restoreId: w.restoreId });
    expect((await c.calendar.event({ id: 302 })).calendarId).toBe(CALENDARS.work);
  });
});
