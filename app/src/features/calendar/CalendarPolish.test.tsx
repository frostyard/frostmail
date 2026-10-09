// CONTRACT TEST for task card T-0077 (docs/tasks). Do not edit.
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { monthGrid } from "../../lib/calendarDates";
import { colors, named, norm, timed, WEEK } from "./fixtures";
import { MonthGrid } from "./MonthGrid";
import { TimeGrid } from "./TimeGrid";

const standup = timed("Standup", "2026-10-08T09:00:00Z", "2026-10-08T09:15:00Z", { eventId: 1 });

describe("calendar polish", () => {
  it("puts a month line's title before its time, which narrow cells hide", () => {
    render(
      <MonthGrid
        days={monthGrid("2026-10-08", 0)}
        selectedDate="2026-10-08"
        occurrences={[standup]}
        colors={colors}
        timeZone="UTC"
        locale="en-US"
        today="2026-10-08"
        selected={null}
        lines={4}
        onSelect={vi.fn()}
        onSelectDate={vi.fn()}
        onShowDay={vi.fn()}
      />,
    );
    const cell = screen.getByRole("gridcell", { name: "Thursday, October 8, 2026" });
    expect(cell.className).toContain("@container");
    const line = within(cell).getByRole("button", { name: named("Standup, 9:00 AM") });
    expect(norm(line.textContent ?? "")).toBe("Standup9:00 AM");
    const time = within(line).getByText((text) => norm(text) === "9:00 AM");
    expect(time.className).toContain("hidden");
    expect(time.className).toContain("@[9rem]:inline");
  });

  it("opens the time grid with 7 AM's label in full", () => {
    const { container } = render(
      <TimeGrid
        days={WEEK}
        occurrences={[]}
        colors={colors}
        timeZone="UTC"
        locale="en-US"
        today="2026-10-08"
        now={new Date("2026-10-08T12:00:00Z")}
        selected={null}
        onSelect={vi.fn()}
        onShowDay={vi.fn()}
      />,
    );
    const scroller = container.querySelector<HTMLElement>(".overflow-y-auto");
    expect(scroller?.scrollTop).toBe(7 * 48 - 12);
  });
});
