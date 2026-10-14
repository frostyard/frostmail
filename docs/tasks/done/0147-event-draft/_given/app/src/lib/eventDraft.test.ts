// CONTRACT TEST for task card T-0147 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { CalendarEvent } from "../rpc/gen/api";
import { changedFields, draftOf, type EventDraft, instants, moveStart, newDraft, ruleOf, validate } from "./eventDraft";

// A weekly review in New York, as calendar.event returns its October 15
// occurrence.
const review: CalendarEvent = {
  id: 302,
  recurrenceId: "2026-10-15T13:00:00.000Z",
  calendarId: 201,
  accountId: 1,
  uid: "review@example.com",
  summary: "Design review",
  location: "Studio B",
  description: "Bring screenshots.",
  allDay: false,
  start: "2026-10-15T13:00:00.000Z",
  end: "2026-10-15T14:30:00.000Z",
  startDate: "",
  endDate: "",
  timeZone: "America/New_York",
  recurring: true,
  recurrence: "EXDATE;TZID=America/New_York:20261022T090000\nRRULE:FREQ=WEEKLY;BYDAY=TH;COUNT=8",
  status: "confirmed",
  transparent: false,
  attendees: [],
  alarms: [15, 60],
  readOnly: false,
};

const conference: CalendarEvent = {
  ...review,
  id: 305,
  recurrenceId: "",
  summary: "Conference",
  allDay: true,
  start: "2026-10-14T00:00:00.000Z",
  end: "2026-10-17T00:00:00.000Z",
  startDate: "2026-10-14",
  endDate: "2026-10-17",
  timeZone: "",
  recurring: false,
  recurrence: "",
  alarms: [],
};

describe("event drafts", () => {
  it("read an event's times in its own zone", () => {
    const d = draftOf(review, "Europe/Berlin");
    expect(d).toEqual({
      summary: "Design review",
      location: "Studio B",
      allDay: false,
      startDate: "2026-10-15",
      startTime: "09:00",
      endDate: "2026-10-15",
      endTime: "10:30",
      timeZone: "America/New_York",
      rrule: "FREQ=WEEKLY;BYDAY=TH;COUNT=8",
      calendarId: 201,
      alarms: [15, 60],
      transparent: false,
      description: "Bring screenshots.",
    } satisfies EventDraft);
    expect(d.alarms).not.toBe(review.alarms);
    expect(draftOf({ ...review, timeZone: "" }, "Europe/Berlin")).toMatchObject({
      startTime: "15:00",
      timeZone: "Europe/Berlin",
    });
  });

  it("show an all-day event's last day, with times ready for a change to timed", () => {
    expect(draftOf(conference, "Europe/Berlin")).toMatchObject({
      allDay: true,
      startDate: "2026-10-14",
      endDate: "2026-10-16",
      startTime: "09:00",
      endTime: "10:00",
      timeZone: "Europe/Berlin",
    });
  });

  it("start a new event from its first times", () => {
    const timed = newDraft(
      { allDay: false, start: "2026-10-08T14:00:00.000Z", end: "2026-10-08T15:00:00.000Z", startDate: "", endDate: "" },
      201,
      "America/New_York",
    );
    expect(timed).toEqual({
      summary: "",
      location: "",
      allDay: false,
      startDate: "2026-10-08",
      startTime: "10:00",
      endDate: "2026-10-08",
      endTime: "11:00",
      timeZone: "America/New_York",
      rrule: "",
      calendarId: 201,
      alarms: [],
      transparent: false,
      description: "",
    } satisfies EventDraft);
    expect(
      newDraft({ allDay: true, start: "", end: "", startDate: "2026-10-12", endDate: "2026-10-13" }, 202, "UTC"),
    ).toMatchObject({ allDay: true, startDate: "2026-10-12", endDate: "2026-10-12", calendarId: 202 });
  });

  it("find an event's rule among its recurrence lines", () => {
    expect(ruleOf(review.recurrence)).toBe("FREQ=WEEKLY;BYDAY=TH;COUNT=8");
    expect(ruleOf("RDATE:20261020T090000Z")).toBe("");
    expect(ruleOf("")).toBe("");
  });

  it("send every field of a new event, and only what changed of an existing one", () => {
    const fresh = {
      ...newDraft({ allDay: true, start: "", end: "", startDate: "2026-10-12", endDate: "2026-10-13" }, 202, "UTC"),
      summary: "Holiday",
    };
    expect(changedFields(null, fresh)).toEqual({
      summary: "Holiday",
      allDay: true,
      startDate: "2026-10-12",
      endDate: "2026-10-13",
    });
    const before = draftOf(review, "UTC");
    expect(changedFields(before, before)).toEqual({});
    expect(changedFields(before, { ...before, summary: "Review", alarms: [15] })).toEqual({
      summary: "Review",
      alarms: [15],
    });
    expect(changedFields(before, { ...before, endTime: "11:00" })).toEqual({
      allDay: false,
      start: "2026-10-15T13:00:00.000Z",
      end: "2026-10-15T15:00:00.000Z",
      timeZone: "America/New_York",
    });
    expect(changedFields(before, { ...before, allDay: true })).toEqual({
      allDay: true,
      startDate: "2026-10-15",
      endDate: "2026-10-16",
    });
    expect(
      changedFields(before, { ...before, rrule: "", transparent: true, calendarId: 202, description: "" }),
    ).toEqual({
      rrule: "",
      transparent: true,
      calendarId: 202,
      description: "",
    });
    // An all-day event's hidden times and zone are not changes.
    const day = draftOf(conference, "UTC");
    expect(changedFields(day, { ...day, startTime: "07:00", timeZone: "Asia/Tokyo" })).toEqual({});
  });

  it("move the start and keep the length", () => {
    const d = draftOf(review, "UTC");
    expect(moveStart(d, "2026-10-16", "17:30")).toMatchObject({
      startDate: "2026-10-16",
      startTime: "17:30",
      endDate: "2026-10-16",
      endTime: "19:00",
      rrule: "FREQ=WEEKLY;BYDAY=FR;COUNT=8",
    });
    expect(moveStart(d, "2026-10-15", "23:00")).toMatchObject({ endDate: "2026-10-16", endTime: "00:30" });
    expect(moveStart(draftOf(conference, "UTC"), "2026-10-20")).toMatchObject({
      startDate: "2026-10-20",
      endDate: "2026-10-22",
    });
    // Across New York's change to standard time: the length stays 90 minutes.
    expect(moveStart(d, "2026-11-01", "01:00")).toMatchObject({ endDate: "2026-11-01", endTime: "01:30" });
  });

  it("refuse an end before its start, and what is missing", () => {
    const d = draftOf(review, "UTC");
    expect(validate(d)).toEqual({});
    expect(validate({ ...d, endTime: "08:00" })).toEqual({ end: "The end is before the start" });
    expect(validate({ ...d, endTime: "09:00" })).toEqual({ end: "The end is before the start" });
    expect(validate({ ...d, startTime: "" })).toEqual({ start: "Enter a start" });
    expect(validate({ ...d, timeZone: "" })).toMatchObject({ timeZone: "Choose a time zone" });
    const day = draftOf(conference, "UTC");
    expect(validate({ ...day, endDate: "2026-10-14" })).toEqual({});
    expect(validate({ ...day, endDate: "2026-10-13" })).toEqual({ end: "The end is before the start" });
    expect(validate({ ...day, endDate: "" })).toEqual({ end: "Enter an end" });
  });

  it("read a timed draft's instants in its zone", () => {
    const { start, end } = instants(draftOf(review, "UTC"));
    expect([start.toISOString(), end.toISOString()]).toEqual(["2026-10-15T13:00:00.000Z", "2026-10-15T14:30:00.000Z"]);
  });
});
