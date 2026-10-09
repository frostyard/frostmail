// CONTRACT TEST for task card T-0071 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { monthGrid } from "../../lib/calendarDates";
import { allDay, colors, named, timed } from "./fixtures";
import { MonthGrid, type MonthGridProps } from "./MonthGrid";

const standup = timed("Standup", "2026-10-08T09:00:00Z", "2026-10-08T09:15:00Z", { eventId: 1 });
const holiday = allDay("Holiday", "2026-10-08", "2026-10-10", { eventId: 2, calendarId: 2 });
const busy = ["One", "Two", "Three", "Four", "Five"].map((s, i) =>
  timed(
    s,
    `2026-10-09T${String(9 + i).padStart(2, "0")}:00:00Z`,
    `2026-10-09T${String(9 + i).padStart(2, "0")}:30:00Z`,
  ),
);

function month(over: Partial<MonthGridProps> = {}) {
  const props: MonthGridProps = {
    days: monthGrid("2026-10-08", 0),
    selectedDate: "2026-10-08",
    occurrences: [standup, holiday],
    colors,
    timeZone: "UTC",
    locale: "en-US",
    today: "2026-10-08",
    selected: null,
    lines: 4,
    onSelect: vi.fn(),
    onSelectDate: vi.fn(),
    onShowDay: vi.fn(),
    ...over,
  };
  const view = render(<MonthGrid {...props} />);
  return { props, view };
}

const cell = (name: string) => screen.getByRole("gridcell", { name });

describe("MonthGrid", () => {
  it("shows six weeks under the weekday names", () => {
    month();
    const grid = screen.getByRole("grid", { name: "October 2026" });
    expect(
      within(grid)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]);
    expect(within(grid).getAllByRole("row").length).toBeGreaterThanOrEqual(6);
    const cells = within(grid).getAllByRole("gridcell");
    expect(cells).toHaveLength(42);
    expect(cells[0]?.getAttribute("aria-label")).toBe("Sunday, September 27, 2026");
  });

  it("numbers days, naming the month on each 1st", () => {
    month();
    expect(within(cell("Thursday, October 8, 2026")).getByText("8")).toBeTruthy();
    expect(within(cell("Thursday, October 1, 2026")).getByText("Oct 1")).toBeTruthy();
    expect(within(cell("Sunday, November 1, 2026")).getByText("Nov 1")).toBeTruthy();
    expect(within(cell("Sunday, September 27, 2026")).getByText("27").className).toContain("text-tertiary");
    expect(within(cell("Thursday, October 8, 2026")).getByText("8").className).toContain("bg-accent");
    expect(cell("Thursday, October 8, 2026").getAttribute("aria-current")).toBe("date");
    expect(cell("Friday, October 9, 2026").getAttribute("aria-current")).toBeNull();
  });

  it("lists each day's all-day occurrences, then its timed ones", () => {
    month();
    const thursday = within(cell("Thursday, October 8, 2026"));
    expect(thursday.getAllByRole("button")).toEqual([
      thursday.getByRole("button", { name: "Holiday, all day" }),
      thursday.getByRole("button", { name: named("Standup, 9:00 AM") }),
    ]);
    expect(within(cell("Friday, October 9, 2026")).getByRole("button", { name: "Holiday, all day" })).toBeTruthy();
    expect(within(cell("Saturday, October 10, 2026")).queryAllByRole("button")).toHaveLength(0);
    expect(
      thursday.getByRole("button", { name: named("Standup, 9:00 AM") }).style.getPropertyValue("--event-color"),
    ).toBe("#3366cc");
    expect(thursday.getByRole("button", { name: "Holiday, all day" }).style.getPropertyValue("--event-color")).toBe(
      "var(--accent)",
    );
  });

  it("shows N more when a day has more than fit", () => {
    const { props } = month({ occurrences: busy, lines: 3 });
    const friday = within(cell("Friday, October 9, 2026"));
    const more = friday.getByRole("button", { name: "Show 3 more on Friday, October 9, 2026" });
    expect(friday.getAllByRole("button")).toEqual([
      friday.getByRole("button", { name: named("One, 9:00 AM") }),
      friday.getByRole("button", { name: named("Two, 10:00 AM") }),
      more,
    ]);
    expect(more.textContent).toBe("3 more");
    fireEvent.click(friday.getByRole("button", { name: "Show 3 more on Friday, October 9, 2026" }));
    expect(props.onShowDay).toHaveBeenCalledWith("2026-10-09");
    expect(props.onSelectDate).not.toHaveBeenCalled();
  });

  it("shows every item when they fit", () => {
    month({ occurrences: busy.slice(0, 3), lines: 3 });
    expect(within(cell("Friday, October 9, 2026")).getAllByRole("button")).toHaveLength(3);
  });

  it("selects occurrences and dates, and opens a day on double-click", () => {
    const { props, view } = month();
    fireEvent.click(screen.getByRole("button", { name: named("Standup, 9:00 AM") }));
    expect(props.onSelect).toHaveBeenCalledWith(standup);
    expect(props.onSelectDate).not.toHaveBeenCalled();
    fireEvent.click(cell("Tuesday, October 13, 2026"));
    expect(props.onSelectDate).toHaveBeenCalledWith("2026-10-13");
    fireEvent.doubleClick(cell("Tuesday, October 13, 2026"));
    expect(props.onShowDay).toHaveBeenCalledWith("2026-10-13");
    expect(cell("Thursday, October 8, 2026").getAttribute("aria-selected")).toBe("true");
    expect(cell("Tuesday, October 13, 2026").getAttribute("aria-selected")).toBe("false");
    view.rerender(<MonthGrid {...props} selected={{ eventId: 1, recurrenceId: "" }} />);
    expect(screen.getByRole("button", { name: named("Standup, 9:00 AM") }).getAttribute("aria-pressed")).toBe("true");
  });
});
