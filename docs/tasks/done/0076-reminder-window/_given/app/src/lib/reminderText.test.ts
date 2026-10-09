// CONTRACT TEST for task card T-0076 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Reminder } from "../rpc/gen/api";
import { reminderWhen, snoozeChoices } from "./reminderText";

const now = new Date("2026-10-08T12:00:00Z"); // a Thursday
const norm = (s: string) => s.replace(/[   ]/g, " ");

function reminder(start: string, extra: Partial<Reminder> = {}): Reminder {
  return {
    id: "r",
    eventId: 1,
    recurrenceId: "",
    calendarId: 1,
    summary: "S",
    location: "",
    allDay: false,
    start,
    startDate: "",
    dueAt: start,
    ...extra,
  };
}

describe("reminderWhen", () => {
  it.each([
    ["2026-10-08T12:10:00Z", "In 10 minutes"],
    ["2026-10-08T12:00:30Z", "In 1 minute"],
    ["2026-10-08T12:59:00Z", "In 59 minutes"],
    ["2026-10-08T12:00:00Z", "Now"],
    ["2026-10-08T11:56:00Z", "Now"],
    ["2026-10-08T11:54:00Z", "6 minutes ago"],
    ["2026-10-08T11:01:00Z", "59 minutes ago"],
    ["2026-10-08T10:30:00Z", "Today, 10:30 AM"],
    ["2026-10-08T14:00:00Z", "Today, 2:00 PM"],
    ["2026-10-09T09:30:00Z", "Tomorrow, 9:30 AM"],
    ["2026-10-07T09:30:00Z", "Yesterday, 9:30 AM"],
    ["2026-10-12T09:30:00Z", "Mon, Oct 12, 9:30 AM"],
  ])("timed at %s", (start, want) => {
    expect(norm(reminderWhen(reminder(start), now, "UTC", "en-US"))).toBe(want);
  });

  it("names all-day dates", () => {
    const on = (date: string) =>
      reminderWhen(reminder(`${date}T00:00:00Z`, { allDay: true, startDate: date }), now, "UTC", "en-US");
    expect([on("2026-10-08"), on("2026-10-09"), on("2026-10-07"), on("2026-10-12")]).toEqual([
      "Today",
      "Tomorrow",
      "Yesterday",
      "Mon, Oct 12",
    ]);
  });

  it("reads days in the zone", () => {
    expect(norm(reminderWhen(reminder("2026-10-09T01:00:00Z"), now, "America/New_York", "en-US"))).toBe(
      "Today, 9:00 PM",
    );
  });
});

describe("snoozeChoices", () => {
  it("offers minutes, an hour and tomorrow morning", () => {
    const choices = snoozeChoices(now, "UTC");
    expect(choices.map((c) => [c.label, c.until.toISOString()])).toEqual([
      ["5 minutes", "2026-10-08T12:05:00.000Z"],
      ["10 minutes", "2026-10-08T12:10:00.000Z"],
      ["15 minutes", "2026-10-08T12:15:00.000Z"],
      ["1 hour", "2026-10-08T13:00:00.000Z"],
      ["Tomorrow", "2026-10-09T09:00:00.000Z"],
    ]);
    expect(snoozeChoices(now, "America/New_York")[4]?.until.toISOString()).toBe("2026-10-09T13:00:00.000Z");
  });
});
