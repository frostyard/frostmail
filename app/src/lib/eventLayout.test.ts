// CONTRACT TEST for task card T-0070 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Occurrence } from "../rpc/gen/api";
import { allDayLanes, busyDates, monthItems, timedLayout } from "./eventLayout";

let next = 1;

function timed(summary: string, start: string, end: string, extra: Partial<Occurrence> = {}): Occurrence {
  return {
    eventId: next++,
    recurrenceId: "",
    calendarId: 1,
    accountId: 1,
    summary,
    location: "",
    allDay: false,
    start,
    end,
    startDate: "",
    endDate: "",
    status: "confirmed",
    transparent: false,
    recurring: false,
    ...extra,
  };
}

function allDay(summary: string, startDate: string, endDate: string, extra: Partial<Occurrence> = {}): Occurrence {
  return timed(summary, `${startDate}T00:00:00Z`, `${endDate}T00:00:00Z`, {
    allDay: true,
    startDate,
    endDate,
    ...extra,
  });
}

const at = (day: string, time: string) => `${day}T${time}:00Z`;

describe("timedLayout", () => {
  it("puts overlapping occurrences side by side", () => {
    const occ = [
      timed("A", at("2026-10-08", "09:00"), at("2026-10-08", "10:00")),
      timed("B", at("2026-10-08", "09:30"), at("2026-10-08", "10:30")),
      timed("C", at("2026-10-08", "10:00"), at("2026-10-08", "11:00")),
      timed("D", at("2026-10-08", "11:00"), at("2026-10-08", "12:00")),
    ];
    const blocks = timedLayout(occ, ["2026-10-08"], "UTC");
    expect(blocks.map((b) => [b.occurrence.summary, b.date, b.startMinute, b.endMinute, b.column, b.columns])).toEqual([
      ["A", "2026-10-08", 540, 600, 0, 2],
      ["B", "2026-10-08", 570, 630, 1, 2],
      ["C", "2026-10-08", 600, 660, 0, 2],
      ["D", "2026-10-08", 660, 720, 0, 1],
    ]);
    expect(blocks.every((b) => !b.continuesBefore && !b.continuesAfter)).toBe(true);
  });

  it("puts the longer of two at the same time first", () => {
    const occ = [
      timed("Short", at("2026-10-08", "14:00"), at("2026-10-08", "15:00")),
      timed("Long", at("2026-10-08", "14:00"), at("2026-10-08", "16:00")),
    ];
    expect(timedLayout(occ, ["2026-10-08"], "UTC").map((b) => [b.occurrence.summary, b.column, b.columns])).toEqual([
      ["Long", 0, 2],
      ["Short", 1, 2],
    ]);
  });

  it("gives a short occurrence the room its block takes", () => {
    const occ = [
      timed("Zero", at("2026-10-08", "17:00"), at("2026-10-08", "17:00")),
      timed("After", at("2026-10-08", "17:10"), at("2026-10-08", "17:40")),
      timed("Later", at("2026-10-08", "17:30"), at("2026-10-08", "17:45")),
    ];
    expect(
      timedLayout(occ, ["2026-10-08"], "UTC").map((b) => [
        b.occurrence.summary,
        b.startMinute,
        b.endMinute,
        b.column,
        b.columns,
      ]),
    ).toEqual([
      ["Zero", 1020, 1020, 0, 2],
      ["After", 1030, 1060, 1, 2],
      ["Later", 1050, 1065, 0, 2],
    ]);
  });

  it("splits occurrences at midnight and keeps to the days given", () => {
    const occ = [
      timed("Night", at("2026-10-08", "22:00"), at("2026-10-09", "02:00")),
      timed("Late", at("2026-10-08", "23:00"), at("2026-10-09", "00:00")),
      timed("Elsewhere", at("2026-10-10", "09:00"), at("2026-10-10", "10:00")),
    ];
    const blocks = timedLayout(occ, ["2026-10-08", "2026-10-09"], "UTC");
    expect(
      blocks.map((b) => [
        b.occurrence.summary,
        b.date,
        b.startMinute,
        b.endMinute,
        b.continuesBefore,
        b.continuesAfter,
      ]),
    ).toEqual([
      ["Night", "2026-10-08", 1320, 1440, false, true],
      ["Late", "2026-10-08", 1380, 1440, false, false],
      ["Night", "2026-10-09", 0, 120, true, false],
    ]);
  });

  it("reads times in the zone and leaves out all-day and declined occurrences", () => {
    const occ = [
      timed("Here", "2026-10-08T13:00:00Z", "2026-10-08T14:00:00Z"),
      timed("No", "2026-10-08T15:00:00Z", "2026-10-08T16:00:00Z", { answer: "declined" }),
      allDay("Holiday", "2026-10-08", "2026-10-09"),
    ];
    expect(
      timedLayout(occ, ["2026-10-08"], "America/New_York").map((b) => [b.occurrence.summary, b.startMinute]),
    ).toEqual([["Here", 540]]);
  });
});

describe("allDayLanes", () => {
  const week = ["2026-10-04", "2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09", "2026-10-10"];

  it("stacks bars in the first lane free across their days", () => {
    const occ = [
      allDay("H1", "2026-10-05", "2026-10-08"),
      allDay("H2", "2026-10-07", "2026-10-08"),
      allDay("H3", "2026-10-01", "2026-10-06"),
      allDay("H4", "2026-10-10", "2026-10-13"),
      allDay("H5", "2026-10-11", "2026-10-12"),
      allDay("Declined", "2026-10-06", "2026-10-07", { answer: "declined" }),
      timed("Timed", at("2026-10-06", "09:00"), at("2026-10-06", "10:00")),
    ];
    expect(
      allDayLanes(occ, week).map((b) => [
        b.occurrence.summary,
        b.lane,
        b.first,
        b.last,
        b.continuesBefore,
        b.continuesAfter,
      ]),
    ).toEqual([
      ["H3", 0, 0, 1, true, false],
      ["H1", 1, 1, 3, false, false],
      ["H2", 0, 3, 3, false, false],
      ["H4", 0, 6, 6, false, true],
    ]);
  });

  it("is empty without all-day occurrences", () => {
    expect(allDayLanes([], week)).toEqual([]);
  });
});

describe("monthItems", () => {
  it("lists a day's all-day occurrences, then its timed ones by start", () => {
    const occ = [
      timed("C", at("2026-10-08", "09:00"), at("2026-10-08", "09:30")),
      allDay("B", "2026-10-08", "2026-10-09"),
      timed("D", at("2026-10-08", "08:00"), at("2026-10-08", "08:30")),
      allDay("A", "2026-10-07", "2026-10-09"),
      timed("E", at("2026-10-07", "23:00"), at("2026-10-08", "01:00")),
      timed("F", at("2026-10-08", "00:00"), at("2026-10-08", "00:00")),
      timed("G", at("2026-10-08", "23:00"), at("2026-10-09", "00:00")),
      timed("H", at("2026-10-09", "00:00"), at("2026-10-09", "00:00")),
      timed("Before", at("2026-10-07", "22:00"), at("2026-10-08", "00:00")),
      allDay("Gone", "2026-10-06", "2026-10-08"),
      timed("No", at("2026-10-08", "12:00"), at("2026-10-08", "13:00"), { answer: "declined" }),
    ];
    expect(monthItems(occ, "2026-10-08", "UTC").map((o) => o.summary)).toEqual(["A", "B", "E", "F", "D", "C", "G"]);
  });

  it("reads days in the zone", () => {
    const occ = [timed("Evening", "2026-10-09T01:00:00Z", "2026-10-09T02:00:00Z")];
    expect(monthItems(occ, "2026-10-08", "America/New_York").map((o) => o.summary)).toEqual(["Evening"]);
    expect(monthItems(occ, "2026-10-09", "America/New_York")).toEqual([]);
  });
});

describe("busyDates", () => {
  it("names the days with something to show", () => {
    const occ = [
      timed("Night", at("2026-10-08", "22:00"), at("2026-10-09", "02:00")),
      allDay("Trip", "2026-10-11", "2026-10-13"),
      timed("No", at("2026-10-14", "12:00"), at("2026-10-14", "13:00"), { answer: "declined" }),
    ];
    const days = ["2026-10-08", "2026-10-09", "2026-10-10", "2026-10-11", "2026-10-12", "2026-10-13", "2026-10-14"];
    expect([...busyDates(occ, days, "UTC")].sort()).toEqual(["2026-10-08", "2026-10-09", "2026-10-11", "2026-10-12"]);
  });
});
