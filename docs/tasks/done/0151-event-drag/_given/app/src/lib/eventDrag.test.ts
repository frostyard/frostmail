// CONTRACT TEST for task card T-0151 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Occurrence } from "../rpc/gen/api";
import { canMove, daysOf, drawn, dropFields, minuteAt, moved, resized, shiftedDays, stepsOf } from "./eventDrag";

const NY = "America/New_York";

const occurrence = (o: Partial<Occurrence>): Occurrence => ({
  eventId: 302,
  recurrenceId: "",
  calendarId: 201,
  accountId: 1,
  summary: "Design review",
  location: "",
  allDay: false,
  start: "2026-10-08T14:00:00.000Z",
  end: "2026-10-08T15:30:00.000Z",
  startDate: "",
  endDate: "",
  status: "confirmed",
  transparent: false,
  recurring: false,
  ...o,
});

describe("eventDrag", () => {
  it("turns pointer moves into steps and days", () => {
    expect(stepsOf(0)).toBe(0);
    expect(stepsOf(5)).toBe(0); // 6 minutes
    expect(stepsOf(7)).toBe(15); // 8.75 minutes
    expect(stepsOf(48)).toBe(60);
    expect(stepsOf(-30)).toBe(-30); // -37.5 minutes, to the nearest step
    expect(daysOf(130, 100)).toBe(1);
    expect(daysOf(-260, 100)).toBe(-3);
    expect(daysOf(40, 100)).toBe(0);
    expect(daysOf(500, 0)).toBe(0);
  });

  it("finds the step under a pointer, within the day", () => {
    expect(minuteAt(0)).toBe(0);
    expect(minuteAt(11)).toBe(0); // 13.75 minutes
    expect(minuteAt(12)).toBe(15);
    expect(minuteAt(9 * 48 + 20)).toBe(9 * 60 + 15);
    expect(minuteAt(-10)).toBe(0);
    expect(minuteAt(2000)).toBe(23 * 60 + 45);
  });

  it("moves a timed range by wall time, keeping its length", () => {
    const range = { start: "2026-10-08T14:00:00.000Z", end: "2026-10-08T15:30:00.000Z" }; // 10:00 EDT
    expect(moved(range, 0, 15, NY)).toEqual({ start: "2026-10-08T14:15:00.000Z", end: "2026-10-08T15:45:00.000Z" });
    expect(moved(range, 1, -60, NY)).toEqual({ start: "2026-10-09T13:00:00.000Z", end: "2026-10-09T14:30:00.000Z" });
    // Across New York's change to standard time, 10:00 stays 10:00 on the wall.
    expect(moved(range, 28, 0, NY)).toEqual({ start: "2026-11-05T15:00:00.000Z", end: "2026-11-05T16:30:00.000Z" });
    // Past midnight, into the next day.
    expect(moved(range, 0, 14 * 60, NY).start).toBe("2026-10-09T04:00:00.000Z");
  });

  it("moves an end, never to less than a step after the start", () => {
    const range = { start: "2026-10-08T14:00:00.000Z", end: "2026-10-08T15:00:00.000Z" };
    expect(resized(range, 30, NY)).toEqual({ start: range.start, end: "2026-10-08T15:30:00.000Z" });
    expect(resized(range, -45, NY).end).toBe("2026-10-08T14:15:00.000Z");
    expect(resized(range, -120, NY).end).toBe("2026-10-08T14:15:00.000Z");
  });

  it("draws a range across the steps it covers", () => {
    expect(drawn("2026-10-08", 9 * 60, 9 * 60 + 45, NY)).toEqual({
      start: "2026-10-08T13:00:00.000Z",
      end: "2026-10-08T14:00:00.000Z",
    });
    expect(drawn("2026-10-08", 11 * 60, 10 * 60, NY)).toEqual({
      start: "2026-10-08T14:00:00.000Z",
      end: "2026-10-08T15:15:00.000Z",
    });
    expect(drawn("2026-10-08", 600, 600, "UTC")).toEqual({
      start: "2026-10-08T10:00:00.000Z",
      end: "2026-10-08T10:15:00.000Z",
    });
    expect(drawn("2026-10-08", 23 * 60 + 45, 23 * 60 + 45, "UTC").end).toBe("2026-10-09T00:00:00.000Z");
  });

  it("lets only the user's own writable events move", () => {
    const writable = new Set([201, 202]);
    expect(canMove(occurrence({}), writable)).toBe(true);
    expect(canMove(occurrence({ calendarId: 203 }), writable)).toBe(false);
    expect(canMove(occurrence({ answer: "accepted" }), writable)).toBe(false);
  });

  it("writes what a drop changes", () => {
    expect(dropFields({ start: "2026-10-08T14:15:00.000Z", end: "2026-10-08T15:45:00.000Z" }, "Europe/Berlin")).toEqual(
      {
        allDay: false,
        start: "2026-10-08T14:15:00.000Z",
        end: "2026-10-08T15:45:00.000Z",
        timeZone: "Europe/Berlin",
      },
    );
    expect(shiftedDays(occurrence({}), 2, NY, "UTC")).toEqual({
      allDay: false,
      start: "2026-10-10T14:00:00.000Z",
      end: "2026-10-10T15:30:00.000Z",
      timeZone: "UTC",
    });
    expect(
      shiftedDays(occurrence({ allDay: true, startDate: "2026-10-14", endDate: "2026-10-17" }), -3, NY, NY),
    ).toEqual({ allDay: true, startDate: "2026-10-11", endDate: "2026-10-14" });
  });
});
