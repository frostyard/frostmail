// CONTRACT TEST for task card T-0071 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { MiniMonth, type MiniMonthProps } from "./MiniMonth";

function month(over: Partial<MiniMonthProps> = {}) {
  const props: MiniMonthProps = {
    selected: "2026-10-08",
    today: "2026-10-08",
    weekStart: 0,
    busy: new Set(["2026-10-09"]),
    locale: "en-US",
    onSelect: vi.fn(),
    ...over,
  };
  const view = render(<MiniMonth {...props} />);
  return { props, view };
}

const day = (name: string) => screen.getByRole("button", { name });

describe("MiniMonth", () => {
  it("shows six weeks from the week holding the 1st", () => {
    month();
    const grid = screen.getByRole("grid", { name: "October 2026" });
    const days = within(grid).getAllByRole("button");
    expect(days).toHaveLength(42);
    expect(days[0]?.getAttribute("aria-label")).toBe("Sunday, September 27, 2026");
    expect(days[41]?.getAttribute("aria-label")).toBe("Saturday, November 7, 2026");
    expect(
      within(grid)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["S", "M", "T", "W", "T", "F", "S"]);
    expect(days[0]?.textContent).toBe("27");
  });

  it("starts weeks on the locale's day", () => {
    month({ weekStart: 1 });
    const days = within(screen.getByRole("grid", { name: "October 2026" })).getAllByRole("button");
    expect(days[0]?.getAttribute("aria-label")).toBe("Monday, September 28, 2026");
    expect(screen.getAllByRole("columnheader")[0]?.textContent).toBe("M");
  });

  it("marks today, the selection, other months and busy days", () => {
    month({ selected: "2026-10-12" });
    expect(day("Thursday, October 8, 2026").getAttribute("aria-current")).toBe("date");
    expect(day("Monday, October 12, 2026").getAttribute("aria-current")).toBeNull();
    expect(day("Monday, October 12, 2026").getAttribute("aria-pressed")).toBe("true");
    expect(day("Thursday, October 8, 2026").getAttribute("aria-pressed")).toBe("false");
    expect(day("Monday, October 12, 2026").className).toContain("bg-selection-sidebar");
    expect(day("Sunday, September 27, 2026").className).toContain("text-tertiary");
    expect(day("Friday, October 9, 2026").querySelector("[data-busy]")).not.toBeNull();
    expect(day("Saturday, October 10, 2026").querySelector("[data-busy]")).toBeNull();
  });

  it("fills today in the accent color when it is selected", () => {
    month();
    expect(day("Thursday, October 8, 2026").className).toContain("bg-accent");
  });

  it("selects a day", () => {
    const { props } = month();
    fireEvent.click(day("Wednesday, October 21, 2026"));
    expect(props.onSelect).toHaveBeenCalledWith("2026-10-21");
  });

  it("pages months without selecting, and follows a new selection", () => {
    const { props, view } = month();
    fireEvent.click(screen.getByRole("button", { name: "Next Month" }));
    expect(screen.getByRole("grid", { name: "November 2026" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Previous Month" }));
    fireEvent.click(screen.getByRole("button", { name: "Previous Month" }));
    expect(screen.getByRole("grid", { name: "September 2026" })).toBeTruthy();
    expect(props.onSelect).not.toHaveBeenCalled();
    view.rerender(<MiniMonth {...props} selected="2026-12-03" />);
    expect(screen.getByRole("grid", { name: "December 2026" })).toBeTruthy();
  });
});
