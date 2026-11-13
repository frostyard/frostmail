import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Toolbar, type ToolbarCalendar, type ToolbarProps } from "./Toolbar";

function toolbar(calendar: Partial<ToolbarCalendar> = {}) {
  const props: ToolbarProps = {
    mode: "calendar",
    calendar: { view: "week", title: "October 2026", paneWidth: 320, ...calendar },
    sidebarWidth: 220,
    listWidth: 360,
    title: "",
    subtitle: "",
    syncing: false,
    selection: { count: 0, seen: true, flagColor: 0 },
    canArchive: false,
    moveTargets: [],
    maximized: false,
    search: <input aria-label="Search" />,
    onCommand: vi.fn(),
  };
  render(<Toolbar {...props} />);
  return props;
}

describe("Toolbar in Calendar: search", () => {
  it("ends the view's segment with Search Events, and shows no field until asked", () => {
    const props = toolbar();
    const button = screen.getByRole("button", { name: "Search Events" });
    expect(button.getAttribute("title")).toBe("Search Events (Ctrl+Alt+F)");
    expect(button.parentElement?.lastElementChild).toBe(button);
    expect(screen.queryByRole("textbox", { name: "Search" })).toBeNull();
    fireEvent.click(button);
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "calendarSearch" });
  });

  it("puts the field in place of Today, the paging buttons and the views", () => {
    toolbar({ search: { results: null } });
    const field = screen.getByRole("textbox", { name: "Search" });
    expect(field.closest("[data-segment]")?.getAttribute("data-segment")).toBe("calendar");
    expect(screen.getByRole("button", { name: "New Event" })).toBeTruthy();
    expect(screen.getByText("October 2026")).toBeTruthy();
    for (const name of ["Today", "Previous Week", "Next Week", "Search Events"])
      expect(screen.queryByRole("button", { name })).toBeNull();
    expect(screen.queryByRole("radiogroup", { name: "View" })).toBeNull();
  });

  it("titles the results and counts them", () => {
    toolbar({ search: { results: "5 results" } });
    expect(screen.getByText("Search Results")).toBeTruthy();
    expect(screen.getByText("5 results")).toBeTruthy();
    expect(screen.queryByText("October 2026")).toBeNull();
  });
});
