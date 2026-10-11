// CONTRACT TEST for task card T-0115 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { atLocal, remindChoices, sendChoices, whenText } from "./later";

const NY = "America/New_York";
const at = (iso: string) => new Date(iso);
const labels = (choices: { label: string; at: Date }[]) => choices.map((c) => [c.label, c.at.toISOString()]);

describe("later's times", () => {
  it("finds a wall-clock time in the zone, across a DST change", () => {
    expect(atLocal("2026-10-12", 8, 0, NY).toISOString()).toBe("2026-10-12T12:00:00.000Z");
    expect(atLocal("2026-11-01", 8, 0, NY).toISOString()).toBe("2026-11-01T13:00:00.000Z"); // EST from 2 AM
    expect(atLocal("2026-03-08", 21, 0, NY).toISOString()).toBe("2026-03-09T01:00:00.000Z"); // EDT from 2 AM
    expect(atLocal("2026-10-12", 8, 30, "UTC").toISOString()).toBe("2026-10-12T08:30:00.000Z");
  });

  it("says a time as Today, Tomorrow or the date", () => {
    const now = at("2026-10-11T14:00:00Z"); // 10:00 AM in New York
    expect(whenText(at("2026-10-12T01:00:00Z"), now, NY, "en-US")).toBe("Today at 9:00 PM");
    expect(whenText(at("2026-10-12T12:00:00Z"), now, NY, "en-US")).toBe("Tomorrow at 8:00 AM");
    expect(whenText(at("2026-10-14T12:05:00Z"), now, NY, "en-US")).toBe("Wed, Oct 14 at 8:05 AM");
    expect(whenText(at("2027-01-04T13:00:00Z"), now, NY, "en-US")).toBe("Mon, Jan 4, 2027 at 8:00 AM");
  });

  it("offers Send Later's times, tonight only before 9 PM", () => {
    const morning = at("2026-10-11T14:00:00Z"); // 10:00 AM in New York
    expect(labels(sendChoices(morning, NY, "en-US"))).toEqual([
      ["Send 9:00 PM Tonight", "2026-10-12T01:00:00.000Z"],
      ["Send 8:00 AM Tomorrow", "2026-10-12T12:00:00.000Z"],
    ]);
    const late = at("2026-10-12T01:30:00Z"); // 9:30 PM in New York
    expect(labels(sendChoices(late, NY, "en-US"))).toEqual([["Send 8:00 AM Tomorrow", "2026-10-12T12:00:00.000Z"]]);
  });

  it("offers Remind Me's times", () => {
    const morning = at("2026-10-11T14:00:00Z");
    expect(labels(remindChoices(morning, NY))).toEqual([
      ["Remind Me in 1 Hour", "2026-10-11T15:00:00.000Z"],
      ["Remind Me Tonight", "2026-10-12T01:00:00.000Z"],
      ["Remind Me Tomorrow", "2026-10-12T12:00:00.000Z"],
    ]);
    const late = at("2026-10-12T01:30:00Z");
    expect(labels(remindChoices(late, NY)).map(([label]) => label)).toEqual([
      "Remind Me in 1 Hour",
      "Remind Me Tomorrow",
    ]);
  });
});
