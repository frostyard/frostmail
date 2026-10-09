// CONTRACT TEST for task card T-0072 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { Client } from "../gen/api";
import { CALENDARS, mockData } from "./fixture";
import { MockTransport } from "./mock";

function client() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  return { mock, c: new Client(mock) };
}

const summaries = (list: { summary: string }[]) => list.map((o) => o.summary);

describe("MockTransport calendar", () => {
  it("lists the fixture's calendars beside its address books", async () => {
    const { c } = client();
    const cals = await c.account.collections({ kind: "calendar" });
    expect(cals.map((col) => [col.id, col.name, col.readOnly, col.enabled])).toEqual([
      [CALENDARS.work, "Work", false, true],
      [CALENDARS.home, "Home", false, true],
      [CALENDARS.holidays, "Holidays", true, true],
    ]);
    expect((await c.account.collections({})).length).toBe(5);
  });

  it("answers a range in start order", async () => {
    const { c } = client();
    const day = await c.calendar.range({ from: "2026-10-08", to: "2026-10-09", timeZone: "UTC" });
    expect(summaries(day)).toEqual(["Standup", "Design review", "Weekly sync"]);
    const standup = day[0];
    expect(standup?.recurrenceId).toBe("2026-10-08T09:00:00.000Z");
    expect(standup?.recurring).toBe(true);
    expect(standup?.calendarId).toBe(CALENDARS.work);
    expect(standup?.answer).toBe("accepted");
    expect(Date.parse(standup?.start ?? "")).toBe(Date.parse("2026-10-08T09:00:00Z"));
    expect(Date.parse(standup?.end ?? "")).toBe(Date.parse("2026-10-08T09:15:00Z"));
    expect(day[1]?.recurring).toBe(false);
    expect(day[1]?.recurrenceId).toBe("");
  });

  it("reads days in the range's zone", async () => {
    const { c } = client();
    const tokyo = await c.calendar.range({ from: "2026-10-08", to: "2026-10-09", timeZone: "Asia/Tokyo" });
    expect(summaries(tokyo)).toEqual(["Standup", "Design review"]);
  });

  it("puts all-day occurrences first, with their dates", async () => {
    const { c } = client();
    const monday = await c.calendar.range({ from: "2026-10-12", to: "2026-10-13", timeZone: "UTC" });
    expect(summaries(monday)).toEqual(["Holiday", "Standup"]);
    const holiday = monday[0];
    expect([holiday?.allDay, holiday?.startDate, holiday?.endDate, holiday?.transparent]).toEqual([
      true,
      "2026-10-12",
      "2026-10-13",
      true,
    ]);
    const conference = await c.calendar.range({ from: "2026-10-15", to: "2026-10-16", timeZone: "UTC" });
    expect(conference[0]?.summary).toBe("Conference");
    expect([conference[0]?.startDate, conference[0]?.endDate]).toEqual(["2026-10-14", "2026-10-17"]);
  });

  it("narrows a range to calendars", async () => {
    const { c } = client();
    const home = await c.calendar.range({
      from: "2026-10-09",
      to: "2026-10-11",
      timeZone: "UTC",
      calendarIds: [CALENDARS.home],
    });
    expect(summaries(home)).toEqual(["Lunch with Ann", "Dentist"]);
    expect(home[0]?.answer).toBe("needsaction");
    expect(home[1]?.status).toBe("cancelled");
  });

  it("hides a hidden calendar and announces it", async () => {
    const { mock, c } = client();
    const events: string[] = [];
    mock.onEvent((e) => events.push(e.event));
    const col = await c.account.setCollection({ id: CALENDARS.work, enabled: false });
    expect(col.enabled).toBe(false);
    await Promise.resolve();
    expect(events).toContain("calendar.changed");
    expect(events).toContain("account.changed");
    expect(await c.calendar.range({ from: "2026-10-08", to: "2026-10-09", timeZone: "UTC" })).toEqual([]);
    expect((await c.account.collections({ kind: "calendar" }))[0]?.enabled).toBe(false);
    await expect(c.account.setCollection({ id: 999, enabled: false })).rejects.toThrow();
  });

  it("answers an event, or one occurrence of a series", async () => {
    const { c } = client();
    const [standup] = await c.calendar.range({ from: "2026-10-08", to: "2026-10-09", timeZone: "UTC" });
    const id = standup?.eventId ?? 0;
    const series = await c.calendar.event({ id });
    expect(series.recurrenceId).toBe("");
    expect(series.recurring).toBe(true);
    expect(series.recurrence).toBe("RRULE:FREQ=DAILY;COUNT=30");
    expect(Date.parse(series.start)).toBe(Date.parse("2026-10-01T09:00:00Z"));
    expect(series.organizer?.email).toBe("bob.okafor@acme.test");
    expect(series.attendees.find((a) => a.isUser)?.email).toBe("test1@mailtest.test");
    expect(series.alarms).toEqual([10]);
    const one = await c.calendar.event({ id, recurrenceId: "2026-10-08T09:00:00.000Z" });
    expect(one.recurrenceId).toBe("2026-10-08T09:00:00.000Z");
    expect(Date.parse(one.start)).toBe(Date.parse("2026-10-08T09:00:00Z"));
    expect(Date.parse(one.end)).toBe(Date.parse("2026-10-08T09:15:00Z"));
    await expect(c.calendar.event({ id, recurrenceId: "2026-12-25T09:00:00.000Z" })).rejects.toThrow();
    await expect(c.calendar.event({ id: 999 })).rejects.toThrow();
  });

  it("rejects bad ranges", async () => {
    const { c } = client();
    await expect(c.calendar.range({ from: "2026-10-09", to: "2026-10-08" })).rejects.toThrow();
    await expect(c.calendar.range({ from: "2026-01-01", to: "2027-02-06" })).rejects.toThrow();
    await expect(
      c.calendar.range({ from: "2026-10-08", to: "2026-10-09", timeZone: "Mars/Olympus" }),
    ).rejects.toThrow();
  });

  it("puts a person's coming occurrences on their contact card", async () => {
    const { c } = client();
    const ann = await c.people.card({ email: "ann.smith@northwind.test" });
    expect(summaries(ann.upcoming)).toEqual(["Lunch with Ann"]);
    const bob = await c.people.card({ email: "bob.okafor@acme.test" });
    expect(summaries(bob.upcoming)).toEqual(["Standup", "Standup", "Standup", "Standup", "Standup"]);
    expect(Date.parse(bob.upcoming[0]?.start ?? "")).toBe(Date.parse("2026-10-09T09:00:00Z"));
    expect((await c.people.card({ email: "nobody@example.test" })).upcoming).toEqual([]);
  });
});
