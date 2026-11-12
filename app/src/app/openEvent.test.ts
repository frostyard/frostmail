import { describe, expect, it } from "vitest";

import { instantAt } from "../lib/calendarDates";
import { useFrameWindow } from "../lodge/frameWindow";
import { eventFromRoute, eventRoute, newEventAt, openEvent } from "./openEvent";

describe("openEvent", () => {
  it("reads its target from its route", () => {
    expect(eventFromRoute("/event/302?recurrenceId=2026-10-08T09%3A00%3A00.000Z")).toEqual({
      kind: "edit",
      id: 302,
      recurrenceId: "2026-10-08T09:00:00.000Z",
    });
    expect(eventFromRoute("/event/new?allDay=1&startDate=2026-10-12&endDate=2026-10-13&calendarId=202")).toEqual({
      kind: "new",
      calendarId: 202,
      allDay: true,
      start: "",
      end: "",
      startDate: "2026-10-12",
      endDate: "2026-10-13",
    });
    expect(eventFromRoute("/settings")).toBeNull();
    expect(eventFromRoute("/event/x")).toBeNull();
  });

  it("shows the editor in the frame, at a route that reads back as the target", async () => {
    const target = {
      kind: "new" as const,
      allDay: false,
      start: "2026-10-08T13:00:00.000Z",
      end: "2026-10-08T14:00:00.000Z",
      startDate: "",
      endDate: "",
    };
    await openEvent(target);
    expect(useFrameWindow.getState().open).toEqual({ kind: "event", target });
    expect(eventFromRoute(eventRoute(target))).toEqual(target);
  });

  it("starts a new event at the next whole hour today, else at 9:00 AM", () => {
    const now = new Date("2026-10-08T13:20:00Z"); // 09:20 in New York
    const zone = "America/New_York";
    expect(newEventAt("2026-10-08", "2026-10-08", now, zone)).toMatchObject({
      start: "2026-10-08T14:00:00.000Z",
      end: "2026-10-08T15:00:00.000Z",
      allDay: false,
    });
    expect(newEventAt("2026-11-02", "2026-10-08", now, zone)).toMatchObject({ start: "2026-11-02T14:00:00.000Z" });
    const late = new Date("2026-10-09T03:30:00Z"); // 23:30 on the 8th in New York
    expect(newEventAt("2026-10-08", "2026-10-08", late, zone)).toMatchObject({ start: "2026-10-09T04:00:00.000Z" });
    // A wall time across a DST change.
    expect(instantAt("2026-11-01", 3 * 60, zone).toISOString()).toBe("2026-11-01T08:00:00.000Z");
  });
});
