// CONTRACT TEST for task card T-0070 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import {
  addDays,
  dayStart,
  localeWeekStart,
  monthGrid,
  startOfWeek,
  step,
  today,
  viewRange,
  viewTitle,
  weekday,
  zoned,
} from "./calendarDates";

describe("date arithmetic", () => {
  it("adds days across months, years and leap days", () => {
    expect(addDays("2026-10-08", 1)).toBe("2026-10-09");
    expect(addDays("2026-10-31", 1)).toBe("2026-11-01");
    expect(addDays("2026-12-31", 1)).toBe("2027-01-01");
    expect(addDays("2028-02-28", 1)).toBe("2028-02-29");
    expect(addDays("2026-03-01", -1)).toBe("2026-02-28");
    expect(addDays("2026-10-08", -365)).toBe("2025-10-08");
    expect(addDays("2026-10-08", 0)).toBe("2026-10-08");
  });

  it("knows weekdays", () => {
    expect(weekday("2026-10-08")).toBe(4);
    expect(weekday("2026-10-11")).toBe(0);
    expect(weekday("2028-02-29")).toBe(2);
  });

  it("starts weeks on the given day", () => {
    expect(startOfWeek("2026-10-08", 0)).toBe("2026-10-04");
    expect(startOfWeek("2026-10-08", 1)).toBe("2026-10-05");
    expect(startOfWeek("2026-10-04", 1)).toBe("2026-09-28");
    expect(startOfWeek("2026-10-05", 1)).toBe("2026-10-05");
    expect(startOfWeek("2026-10-10", 6)).toBe("2026-10-10");
    expect(startOfWeek("2026-10-09", 6)).toBe("2026-10-03");
  });
});

describe("monthGrid", () => {
  it("is six weeks from the week holding the 1st", () => {
    const days = monthGrid("2026-10-08", 0);
    expect(days).toHaveLength(42);
    expect(days[0]).toBe("2026-09-27");
    expect(days[4]).toBe("2026-10-01");
    expect(days[41]).toBe("2026-11-07");
    expect(monthGrid("2026-10-31", 1)[0]).toBe("2026-09-28");
    expect(monthGrid("2026-02-14", 0)[0]).toBe("2026-02-01");
    expect(monthGrid("2026-02-14", 0)[41]).toBe("2026-03-14");
  });
});

describe("views", () => {
  it("covers a view's days", () => {
    expect(viewRange("day", "2026-10-08", 0)).toEqual({ from: "2026-10-08", to: "2026-10-09" });
    expect(viewRange("week", "2026-10-08", 0)).toEqual({ from: "2026-10-04", to: "2026-10-11" });
    expect(viewRange("week", "2026-10-08", 1)).toEqual({ from: "2026-10-05", to: "2026-10-12" });
    expect(viewRange("month", "2026-10-08", 0)).toEqual({ from: "2026-09-27", to: "2026-11-08" });
  });

  it("steps by the view's period", () => {
    expect(step("day", "2026-10-08", 1)).toBe("2026-10-09");
    expect(step("day", "2026-10-01", -1)).toBe("2026-09-30");
    expect(step("week", "2026-10-08", 1)).toBe("2026-10-15");
    expect(step("week", "2026-10-08", -1)).toBe("2026-10-01");
    expect(step("month", "2026-10-08", 1)).toBe("2026-11-08");
    expect(step("month", "2026-12-15", 1)).toBe("2027-01-15");
    expect(step("month", "2026-01-15", -1)).toBe("2025-12-15");
    expect(step("month", "2026-01-31", 1)).toBe("2026-02-28");
    expect(step("month", "2028-01-31", 1)).toBe("2028-02-29");
    expect(step("month", "2026-03-31", -1)).toBe("2026-02-28");
  });

  it("titles a view", () => {
    expect(viewTitle("day", "2026-10-08", 0, "en-US")).toBe("Thursday, October 8, 2026");
    expect(viewTitle("week", "2026-10-08", 0, "en-US")).toBe("October 2026");
    expect(viewTitle("week", "2026-09-30", 0, "en-US")).toBe("Sep – Oct 2026");
    expect(viewTitle("week", "2026-12-30", 0, "en-US")).toBe("Dec 2026 – Jan 2027");
    expect(viewTitle("week", "2026-10-04", 1, "en-US")).toBe("Sep – Oct 2026");
    expect(viewTitle("month", "2026-10-08", 0, "en-US")).toBe("October 2026");
  });
});

describe("time zones", () => {
  it("names today in a zone", () => {
    const now = new Date("2026-10-09T02:00:00Z");
    expect(today("America/New_York", now)).toBe("2026-10-08");
    expect(today("Asia/Tokyo", now)).toBe("2026-10-09");
    expect(today("UTC", now)).toBe("2026-10-09");
  });

  it("reads an instant's day and minutes in a zone", () => {
    expect(zoned("2026-10-08T13:15:00Z", "America/New_York")).toEqual({ date: "2026-10-08", minutes: 9 * 60 + 15 });
    expect(zoned("2026-10-09T02:00:00Z", "America/New_York")).toEqual({ date: "2026-10-08", minutes: 22 * 60 });
    expect(zoned(new Date("2026-10-08T00:00:00Z"), "UTC")).toEqual({ date: "2026-10-08", minutes: 0 });
    expect(zoned("2026-10-08T18:30:00Z", "Asia/Kolkata")).toEqual({ date: "2026-10-09", minutes: 0 });
  });

  it("finds a day's first instant", () => {
    expect(dayStart("2026-10-08", "UTC").toISOString()).toBe("2026-10-08T00:00:00.000Z");
    expect(dayStart("2026-10-08", "America/New_York").toISOString()).toBe("2026-10-08T04:00:00.000Z");
    expect(dayStart("2026-11-01", "America/New_York").toISOString()).toBe("2026-11-01T04:00:00.000Z");
    expect(dayStart("2026-11-02", "America/New_York").toISOString()).toBe("2026-11-02T05:00:00.000Z");
    expect(dayStart("2026-03-08", "America/New_York").toISOString()).toBe("2026-03-08T05:00:00.000Z");
    expect(dayStart("2026-03-09", "America/New_York").toISOString()).toBe("2026-03-09T04:00:00.000Z");
    expect(dayStart("2026-10-09", "Asia/Kolkata").toISOString()).toBe("2026-10-08T18:30:00.000Z");
  });

  it("knows the locale's first day of the week", () => {
    expect(localeWeekStart("en-US")).toBe(0);
    expect(localeWeekStart("de-DE")).toBe(1);
  });
});
