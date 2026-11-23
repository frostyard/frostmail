// lodge-ui ui/v0.7.0 moved --text-secondary to #5b5b60 (light) and #aaaaaf
// (dark): on --accent, the default event colour, it reaches only 1.03:1. A
// selected occurrence is filled with its event colour and accent-contrast
// text, so its start time takes that colour, not secondary.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { monthGrid } from "../../lib/calendarDates";
import { colors, named, timed } from "./fixtures";
import { MonthGrid } from "./MonthGrid";

const standup = timed("Standup", "2026-10-08T09:00:00Z", "2026-10-08T09:15:00Z", { eventId: 1 });

function time(selected: boolean) {
  render(
    <MonthGrid
      days={monthGrid("2026-10-08", 0)}
      selectedDate="2026-10-08"
      occurrences={[standup]}
      colors={colors}
      timeZone="UTC"
      locale="en-US"
      today="2026-10-08"
      selected={selected ? { eventId: 1, recurrenceId: "" } : null}
      lines={4}
      onSelect={vi.fn()}
      onSelectDate={vi.fn()}
      onShowDay={vi.fn()}
    />,
  );
  const button = screen.getByRole("button", { name: named("Standup, 9:00 AM") });
  expect(button.getAttribute("aria-pressed")).toBe(String(selected));
  const start = button.querySelector<HTMLElement>(".tabular-nums");
  if (!start) throw new Error("no start time");
  return { button, className: start.className };
}

describe("MonthGrid's start time on a selected occurrence", () => {
  it("is not secondary on the event colour's fill", () => {
    const { button, className } = time(true);
    expect(button.className).toContain("text-accent-contrast");
    expect(className).not.toContain("text-secondary");
  });

  it("stays secondary on an occurrence that is not selected", () => {
    expect(time(false).className).toContain("text-secondary");
  });
});
