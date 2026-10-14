import { afterEach, describe, expect, it, vi } from "vitest";

import { editorTarget, instantAt, newEventAt, openEvent, windowLabel } from "./eventWindow";

afterEach(() => {
  window.location.hash = "";
  delete window.__frostmailEvent;
  vi.restoreAllMocks();
});

describe("eventWindow", () => {
  it("reads its target from the hash a browser tab opens", () => {
    window.location.hash = "#/event/302?recurrenceId=2026-10-08T09%3A00%3A00.000Z";
    expect(editorTarget()).toEqual({ kind: "edit", id: 302, recurrenceId: "2026-10-08T09:00:00.000Z" });
    window.location.hash = "#/event/new?allDay=1&startDate=2026-10-12&endDate=2026-10-13&calendarId=202";
    expect(editorTarget()).toEqual({
      kind: "new",
      calendarId: 202,
      allDay: true,
      start: "",
      end: "",
      startDate: "2026-10-12",
      endDate: "2026-10-13",
    });
    window.location.hash = "#/settings";
    expect(editorTarget()).toBeNull();
  });

  it("prefers the target the window was made with", () => {
    window.__frostmailEvent = { kind: "edit", id: 7, recurrenceId: "" };
    window.location.hash = "#/event/8";
    expect(editorTarget()).toEqual({ kind: "edit", id: 7, recurrenceId: "" });
  });

  it("opens a browser tab whose hash reads back as the target", async () => {
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    const target = {
      kind: "new" as const,
      allDay: false,
      start: "2026-10-08T13:00:00.000Z",
      end: "2026-10-08T14:00:00.000Z",
      startDate: "",
      endDate: "",
    };
    await openEvent(target);
    const url = String(open.mock.calls[0]?.[0]);
    window.location.hash = url.slice(url.indexOf("#"));
    expect(editorTarget()).toEqual(target);
    expect(String(open.mock.calls[0]?.[1])).toMatch(/^event-new-\d+$/);
  });

  it("labels a window by its event and occurrence", () => {
    expect(windowLabel({ kind: "edit", id: 302, recurrenceId: "" })).toBe("event-302");
    expect(windowLabel({ kind: "edit", id: 301, recurrenceId: "2026-10-08T09:00:00.000Z" })).toBe(
      "event-301-2026-10-08T09-00-00-000Z",
    );
    expect(windowLabel({ kind: "edit", id: 305, recurrenceId: "2026-10-12" })).toBe("event-305-2026-10-12");
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
