// CONTRACT TEST for task card T-0074 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { allDay, norm, timed } from "../features/calendar/fixtures";
import { upcomingWhen } from "./eventText";

const now = new Date("2026-10-08T12:00:00Z"); // a Thursday

describe("upcomingWhen", () => {
  it("gives today's time, a weekday within six days, else the date", () => {
    const at = (start: string) => norm(upcomingWhen(timed("x", start, start), "UTC", "en-US", now));
    expect(at("2026-10-08T15:00:00Z")).toBe("3:00 PM");
    expect(at("2026-10-09T09:00:00Z")).toBe("Fri");
    expect(at("2026-10-14T09:00:00Z")).toBe("Wed");
    expect(at("2026-10-15T09:00:00Z")).toBe("Oct 15");
  });

  it("says Today for an all-day occurrence today", () => {
    const on = (date: string, end: string) => upcomingWhen(allDay("x", date, end), "UTC", "en-US", now);
    expect(on("2026-10-08", "2026-10-09")).toBe("Today");
    expect(on("2026-10-07", "2026-10-10")).toBe("Today");
    expect(on("2026-10-09", "2026-10-10")).toBe("Fri");
    expect(on("2026-10-20", "2026-10-21")).toBe("Oct 20");
  });

  it("reads days in the zone", () => {
    const late = timed("x", "2026-10-09T02:00:00Z", "2026-10-09T03:00:00Z");
    expect(norm(upcomingWhen(late, "America/New_York", "en-US", now))).toBe("10:00 PM");
    expect(upcomingWhen(late, "UTC", "en-US", now)).toBe("Fri");
  });
});
