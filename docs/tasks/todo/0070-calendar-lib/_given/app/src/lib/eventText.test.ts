// CONTRACT TEST for task card T-0070 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { alarmText, answerText, calendarColor, dateText, describeRecurrence, timeRange } from "./eventText";

// Intl puts thin and narrow no-break spaces in ranges and before AM/PM.
const norm = (s: string) => s.replace(/[   ]/g, " ");

describe("describeRecurrence", () => {
  it.each([
    ["RRULE:FREQ=DAILY", "Every day"],
    ["RRULE:FREQ=DAILY;INTERVAL=2", "Every 2 days"],
    ["RRULE:FREQ=DAILY;COUNT=7", "Every day, 7 times"],
    ["RRULE:FREQ=DAILY;COUNT=1", "Every day, once"],
    ["RRULE:FREQ=WEEKLY", "Every week"],
    ["RRULE:FREQ=WEEKLY;BYDAY=TU", "Every week on Tuesday"],
    ["RRULE:FREQ=WEEKLY;BYDAY=SA,SU;WKST=SU", "Every week on Saturday and Sunday"],
    ["RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR", "Every week on Monday, Wednesday and Friday"],
    ["RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", "Every weekday"],
    [
      "RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=TH;UNTIL=20261231T235959Z",
      "Every 2 weeks on Thursday, until December 31, 2026",
    ],
    ["RRULE:FREQ=MONTHLY", "Every month"],
    ["RRULE:FREQ=MONTHLY;BYMONTHDAY=15", "Every month on day 15"],
    ["RRULE:FREQ=MONTHLY;BYDAY=2TU", "Every month on the second Tuesday"],
    ["RRULE:FREQ=MONTHLY;BYDAY=-1FR;INTERVAL=3", "Every 3 months on the last Friday"],
    ["RRULE:FREQ=YEARLY", "Every year"],
    ["RRULE:FREQ=YEARLY;UNTIL=20300101", "Every year, until January 1, 2030"],
    ["EXDATE:20261010T090000Z\nRRULE:FREQ=DAILY", "Every day"],
    ["RRULE:FREQ=MONTHLY;BYDAY=MO,TU", "Custom"],
    ["RRULE:FREQ=HOURLY", "Custom"],
    ["RRULE:FREQ=DAILY;BYHOUR=9,17", "Custom"],
    ["RDATE:20261010T090000Z", "Custom"],
    ["", ""],
  ])("%# %s", (recurrence, want) => {
    expect(describeRecurrence(recurrence, "en-US")).toBe(want);
  });
});

describe("alarmText", () => {
  it.each([
    [0, "At time of event"],
    [1, "1 minute before"],
    [10, "10 minutes before"],
    [60, "1 hour before"],
    [90, "90 minutes before"],
    [120, "2 hours before"],
    [1440, "1 day before"],
    [2880, "2 days before"],
    [10080, "7 days before"],
    [-5, "5 minutes after"],
    [-60, "1 hour after"],
  ])("%i", (minutes, want) => {
    expect(alarmText(minutes)).toBe(want);
  });
});

describe("times and dates", () => {
  it("formats a time range in the zone", () => {
    expect(norm(timeRange("2026-10-08T13:00:00Z", "2026-10-08T13:15:00Z", "America/New_York", "en-US"))).toBe(
      "9:00 – 9:15 AM",
    );
    expect(norm(timeRange("2026-10-08T15:30:00Z", "2026-10-08T17:00:00Z", "America/New_York", "en-US"))).toBe(
      "11:30 AM – 1:00 PM",
    );
  });

  it("formats an event's dates", () => {
    const at = {
      allDay: false,
      start: "2026-10-09T02:00:00Z",
      end: "2026-10-09T03:00:00Z",
      startDate: "",
      endDate: "",
    };
    expect(dateText(at, "America/New_York", "en-US")).toBe("Thursday, October 8, 2026");
    expect(dateText(at, "UTC", "en-US")).toBe("Friday, October 9, 2026");
    const day = { allDay: true, start: "", end: "", startDate: "2026-10-12", endDate: "2026-10-13" };
    expect(dateText(day, "America/New_York", "en-US")).toBe("Monday, October 12, 2026");
    const days = { ...day, endDate: "2026-10-15" };
    expect(norm(dateText(days, "America/New_York", "en-US"))).toBe("October 12 – 14, 2026");
  });

  it("names answers", () => {
    expect(answerText("accepted")).toBe("Accepted");
    expect(answerText("declined")).toBe("Declined");
    expect(answerText("tentative")).toBe("Maybe");
    expect(answerText("needsaction")).toBe("Not answered");
    expect(answerText("delegated")).toBe("Delegated");
  });

  it("falls back to the accent color", () => {
    expect(calendarColor("#3366cc")).toBe("#3366cc");
    expect(calendarColor("")).toBe("var(--accent)");
  });
});
