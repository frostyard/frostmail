import { describe, expect, it } from "vitest";

import type { Occurrence } from "../rpc/gen/api";
import { searchRows, searchSubtitle, searchWhen } from "./calendarSearch";

// Intl puts thin and narrow no-break spaces in ranges and before AM/PM.
const norm = (text: string | null) => (text ?? "").replace(/[\u2009\u202f]/g, " ");

const occurrence = (over: Partial<Occurrence>): Occurrence => ({
  eventId: 1,
  recurrenceId: "",
  calendarId: 1,
  accountId: 1,
  summary: "Sync",
  location: "",
  allDay: false,
  start: "2026-10-08T11:00:00.000Z",
  end: "2026-10-08T11:30:00.000Z",
  startDate: "",
  endDate: "",
  status: "confirmed",
  transparent: false,
  recurring: false,
  ...over,
});

const at = (day: number) =>
  occurrence({ eventId: day, start: `2026-10-0${day}T11:00:00.000Z`, end: `2026-10-0${day}T11:30:00.000Z` });

describe("searchRows", () => {
  const ids = (now: string) =>
    searchRows([at(5), at(6), at(7), at(8), at(9)], new Date(now)).map((row) =>
      "heading" in row ? row.heading : row.occurrence.eventId,
    );

  it("lists what has not ended in start order, then the rest, the latest first", () => {
    expect(ids("2026-10-08T10:30:00Z")).toEqual(["Upcoming", 8, 9, "Earlier", 7, 6, 5]);
  });

  it("counts an occurrence under way as upcoming, and one just ended as earlier", () => {
    expect(ids("2026-10-08T11:29:00Z")).toEqual(["Upcoming", 8, 9, "Earlier", 7, 6, 5]);
    expect(ids("2026-10-08T11:30:00Z")).toEqual(["Upcoming", 9, "Earlier", 8, 7, 6, 5]);
  });

  // A conference from the 5th to the 9th is under way on the 8th, and
  // starts before the meetings of the 6th and 7th, which are over.
  it("judges each occurrence by its own end, not by where the first unfinished one starts", () => {
    const conference = occurrence({ eventId: 50, start: "2026-10-05T08:00:00.000Z", end: "2026-10-09T17:00:00.000Z" });
    const rows = searchRows([conference, at(6), at(7), at(8), at(9)], new Date("2026-10-08T10:30:00Z"));
    expect(rows.map((row) => ("heading" in row ? row.heading : row.occurrence.eventId))).toEqual([
      "Upcoming",
      50,
      8,
      9,
      "Earlier",
      7,
      6,
    ]);
  });

  it("leaves out a heading with no rows", () => {
    expect(ids("2026-10-01T00:00:00Z")).toEqual(["Upcoming", 5, 6, 7, 8, 9]);
    expect(ids("2026-11-01T00:00:00Z")).toEqual(["Earlier", 9, 8, 7, 6, 5]);
    expect(searchRows([], new Date())).toEqual([]);
  });
});

describe("searchWhen", () => {
  it("writes the day, the time, the place and a cancellation", () => {
    expect(norm(searchWhen(occurrence({}), "UTC", "en-US"))).toBe("Thu, Oct 8, 2026 · 11:00 – 11:30 AM");
    expect(norm(searchWhen(occurrence({ location: "Room 4", status: "cancelled" }), "UTC", "en-US"))).toBe(
      "Thu, Oct 8, 2026 · 11:00 – 11:30 AM · Room 4 · Cancelled",
    );
  });

  it("reads a timed occurrence's day in the zone", () => {
    const late = occurrence({ start: "2026-10-08T23:00:00.000Z", end: "2026-10-08T23:30:00.000Z" });
    expect(norm(searchWhen(late, "Europe/Berlin", "en-US"))).toBe("Fri, Oct 9, 2026 · 1:00 – 1:30 AM");
  });

  it("writes an all-day occurrence by its date, whatever the zone", () => {
    const holiday = occurrence({
      allDay: true,
      startDate: "2026-10-09",
      endDate: "2026-10-10",
      start: "2026-10-09T10:00:00.000Z",
      end: "2026-10-10T10:00:00.000Z",
    });
    expect(norm(searchWhen(holiday, "Pacific/Honolulu", "en-US"))).toBe("Fri, Oct 9, 2026 · All day");
  });
});

describe("searchSubtitle", () => {
  it("counts, waits and fails", () => {
    expect(searchSubtitle(null, false)).toBe("Searching…");
    expect(searchSubtitle(null, true)).toBe("The search failed");
    expect(searchSubtitle([], false)).toBe("0 results");
    expect(searchSubtitle([at(5)], false)).toBe("1 result");
    expect(searchSubtitle([at(5), at(6)], false)).toBe("2 results");
  });
});
