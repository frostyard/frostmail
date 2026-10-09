// CONTRACT TEST for task card T-0071 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { type CalendarSection, CalendarSidebar, type CalendarSidebarProps } from "./CalendarSidebar";

const sections: CalendarSection[] = [
  {
    accountId: 1,
    title: "user@dav.test",
    calendars: [
      { id: 10, name: "Work", color: "#3366cc", enabled: true, readOnly: false },
      { id: 11, name: "Home", color: "", enabled: false, readOnly: true },
    ],
  },
  { accountId: 2, title: "empty@example.com", calendars: [] },
  {
    accountId: 3,
    title: "me@icloud.example",
    calendars: [{ id: 30, name: "Family", color: "#ff9500", enabled: true, readOnly: false }],
  },
];

function sidebar(over: Partial<CalendarSidebarProps> = {}) {
  const props: CalendarSidebarProps = {
    sections,
    onToggle: vi.fn(),
    selected: "2026-10-08",
    today: "2026-10-08",
    weekStart: 0,
    busy: new Set(),
    locale: "en-US",
    onSelect: vi.fn(),
    ...over,
  };
  render(<CalendarSidebar {...props} />);
  return props;
}

describe("CalendarSidebar", () => {
  it("shows the small month above each account's calendars", () => {
    sidebar();
    expect(screen.getByRole("grid", { name: "October 2026" })).toBeTruthy();
    const first = screen.getByRole("group", { name: "user@dav.test" });
    expect(
      within(first)
        .getAllByRole("checkbox")
        .map((c) => c.textContent),
    ).toEqual(["Work", "Home"]);
    expect(within(screen.getByRole("group", { name: "me@icloud.example" })).getAllByRole("checkbox")).toHaveLength(1);
    expect(screen.queryByRole("group", { name: "empty@example.com" })).toBeNull();
  });

  it("checks the calendars shown and marks read-only ones", () => {
    sidebar();
    const work = screen.getByRole("checkbox", { name: "Work" });
    const home = screen.getByRole("checkbox", { name: /^Home/ });
    expect(work.getAttribute("aria-checked")).toBe("true");
    expect(home.getAttribute("aria-checked")).toBe("false");
    expect(within(home).getByLabelText("Read-only")).toBeTruthy();
    expect(within(work).queryByLabelText("Read-only")).toBeNull();
  });

  it("toggles a calendar", () => {
    const props = sidebar();
    fireEvent.click(screen.getByRole("checkbox", { name: "Work" }));
    expect(props.onToggle).toHaveBeenLastCalledWith(10, false);
    fireEvent.click(screen.getByRole("checkbox", { name: /^Home/ }));
    expect(props.onToggle).toHaveBeenLastCalledWith(11, true);
  });

  it("selects a date in the small month", () => {
    const props = sidebar();
    fireEvent.click(screen.getByRole("button", { name: "Friday, October 9, 2026" }));
    expect(props.onSelect).toHaveBeenCalledWith("2026-10-09");
  });
});
