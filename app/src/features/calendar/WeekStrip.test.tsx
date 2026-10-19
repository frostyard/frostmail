// A narrow window's date picker (docs/specs/ui.md, Narrow windows).
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { WeekStrip } from "./WeekStrip";

describe("WeekStrip", () => {
  it("shows the selected date's week, and moves a day or a week", () => {
    const onSelect = vi.fn();
    render(
      <WeekStrip
        selected="2026-10-21"
        today="2026-10-19"
        weekStart={0}
        busy={new Set(["2026-10-22"])}
        locale="en-US"
        onSelect={onSelect}
      />,
    );
    const week = screen.getByRole("group", { name: "Week" });
    const days = within(week)
      .getAllByRole("button")
      .filter((b) => b.hasAttribute("aria-pressed"));
    expect(days.map((b) => b.textContent?.replace(/\D/g, ""))).toEqual(["18", "19", "20", "21", "22", "23", "24"]);
    expect(days.map((b) => b.getAttribute("aria-pressed"))).toEqual([
      "false",
      "false",
      "false",
      "true",
      "false",
      "false",
      "false",
    ]);
    expect(days[1]?.getAttribute("aria-current")).toBe("date");
    expect(days[4]?.querySelector("[data-busy]")).not.toBeNull();
    for (const b of days) expect(b.className).toContain("h-14");
    fireEvent.click(days[4] as HTMLElement);
    expect(onSelect).toHaveBeenLastCalledWith("2026-10-22");
    fireEvent.click(within(week).getByRole("button", { name: "Previous Week" }));
    expect(onSelect).toHaveBeenLastCalledWith("2026-10-14");
    fireEvent.click(within(week).getByRole("button", { name: "Next Week" }));
    expect(onSelect).toHaveBeenLastCalledWith("2026-10-28");
  });
});
