// CONTRACT TEST for task card T-0076 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { Client } from "../gen/api";
import { mockData } from "./fixture";
import { MockTransport } from "./mock";

function client() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z"), reminders: true }));
  return { mock, c: new Client(mock) };
}

describe("MockTransport reminders", () => {
  it("lists the fixture's reminders, oldest due first", async () => {
    const { c } = client();
    const list = await c.calendar.reminders({});
    expect(list.map((r) => [r.summary, r.eventId, r.recurrenceId, r.location])).toEqual([
      ["Standup", 301, "2026-10-08T09:00:00.000Z", "Room 4"],
      ["Design review", 302, "", "Studio B"],
    ]);
    expect(Date.parse(list[0]?.start ?? "")).toBe(Date.parse("2026-10-08T09:00:00Z"));
    expect(new Set(list.map((r) => r.id)).size).toBe(2);
  });

  it("snoozes and dismisses, announcing the count", async () => {
    const { mock, c } = client();
    const counts: number[] = [];
    mock.onEvent((e) => {
      if (e.event === "calendar.reminders") counts.push(e.data.count);
    });
    const [standup, review] = await c.calendar.reminders({});
    await c.calendar.snooze({ ids: [standup?.id ?? ""], until: "2026-10-08T12:10:00Z" });
    expect((await c.calendar.reminders({})).map((r) => r.summary)).toEqual(["Design review"]);
    await c.calendar.dismiss({ ids: [review?.id ?? ""] });
    expect(await c.calendar.reminders({})).toEqual([]);
    await Promise.resolve();
    expect(counts).toEqual([1, 0]);
    await expect(c.calendar.dismiss({ ids: [] })).rejects.toThrow();
  });
});
