// CONTRACT TEST for task card T-0091 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Occurrence } from "../rpc/gen/api";
import { nextOccurrences } from "./eventText";

const occ = (summary: string, over: Partial<Occurrence> = {}): Occurrence => ({
  eventId: 1,
  recurrenceId: "",
  calendarId: 1,
  accountId: 1,
  summary,
  location: "",
  allDay: false,
  start: "2026-10-09T09:00:00Z",
  end: "2026-10-09T10:00:00Z",
  startDate: "",
  endDate: "",
  status: "confirmed",
  transparent: false,
  recurring: false,
  ...over,
});

const now = new Date("2026-10-09T09:30:00Z");

describe("nextOccurrences", () => {
  it("keeps what has not ended and is not cancelled, in order, up to the limit", () => {
    const list = [
      occ("Ended", { start: "2026-10-09T08:00:00Z", end: "2026-10-09T09:00:00Z" }),
      occ("Now"),
      occ("Holiday", { allDay: true, startDate: "2026-10-09", endDate: "2026-10-10" }),
      occ("Dentist", { start: "2026-10-09T16:00:00Z", end: "2026-10-09T17:00:00Z", status: "cancelled" }),
      occ("Later", { start: "2026-10-09T16:00:00Z", end: "2026-10-09T17:00:00Z" }),
      occ("Tomorrow", { start: "2026-10-10T09:00:00Z", end: "2026-10-10T10:00:00Z" }),
    ];
    expect(nextOccurrences(list, now, "UTC", 5).map((o) => o.summary)).toEqual(["Now", "Holiday", "Later", "Tomorrow"]);
    expect(nextOccurrences(list, now, "UTC", 2).map((o) => o.summary)).toEqual(["Now", "Holiday"]);
  });

  it("ends an all-day occurrence at midnight in the zone", () => {
    const yesterday = occ("Yesterday", { allDay: true, startDate: "2026-10-08", endDate: "2026-10-09" });
    expect(nextOccurrences([yesterday], now, "UTC", 5)).toEqual([]);
    expect(nextOccurrences([yesterday], now, "Pacific/Honolulu", 5).map((o) => o.summary)).toEqual(["Yesterday"]);
  });
});
