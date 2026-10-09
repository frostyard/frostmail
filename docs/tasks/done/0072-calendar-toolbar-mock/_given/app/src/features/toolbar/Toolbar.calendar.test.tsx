// CONTRACT TEST for task card T-0072 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Toolbar, type ToolbarProps } from "./Toolbar";

function toolbar(over: Partial<ToolbarProps> = {}) {
  const props: ToolbarProps = {
    mode: "calendar",
    calendar: { view: "week", title: "October 2026", paneWidth: 320 },
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
    ...over,
  };
  render(<Toolbar {...props} />);
  return props;
}

const button = (name: string) => screen.getByRole("button", { name });

describe("Toolbar in Calendar", () => {
  it("has the sidebar's, the view's and the event pane's segments", () => {
    toolbar();
    const bar = screen.getByRole("toolbar", { name: "Toolbar" });
    const segments = [...bar.querySelectorAll<HTMLElement>(":scope > [data-segment]")];
    expect(segments.map((s) => s.getAttribute("data-segment"))).toEqual(["sidebar", "calendar", "pane"]);
    expect(segments[0]?.style.width).toBe("221px");
    expect(segments[2]?.style.width).toBe("321px");
    for (const s of segments) expect(s.hasAttribute("data-tauri-drag-region")).toBe(true);
  });

  it("shows Today, paging, the title and the views", () => {
    const props = toolbar();
    expect(screen.getByText("October 2026")).toBeTruthy();
    fireEvent.click(button("Today"));
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "today" });
    fireEvent.click(button("Previous Week"));
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "previousPeriod" });
    fireEvent.click(button("Next Week"));
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "nextPeriod" });
    const views = screen.getByRole("radiogroup", { name: "View" });
    const radios = within(views).getAllByRole("radio");
    expect(radios.map((r) => [r.textContent, r.getAttribute("aria-checked")])).toEqual([
      ["Day", "false"],
      ["Week", "true"],
      ["Month", "false"],
    ]);
    fireEvent.click(within(views).getByRole("radio", { name: "Month" }));
    expect(props.onCommand).toHaveBeenLastCalledWith({ kind: "calendarView", view: "month" });
  });

  it("names paging by the view", () => {
    toolbar({ calendar: { view: "day", title: "Thursday, October 8, 2026", paneWidth: 320 } });
    expect(button("Previous Day")).toBeTruthy();
    expect(button("Next Day")).toBeTruthy();
    expect(screen.getByRole("radio", { name: "Day" }).getAttribute("aria-checked")).toBe("true");
  });

  it("keeps the sidebar toggle, Settings and window controls, and nothing of Mail's or search", () => {
    toolbar({ calendar: { view: "month", title: "October 2026", paneWidth: 320 } });
    for (const name of [
      "Toggle Sidebar",
      "Previous Month",
      "Next Month",
      "Settings",
      "Minimize",
      "Maximize",
      "Close",
    ]) {
      expect(button(name)).toBeTruthy();
    }
    for (const name of ["Get Mail", "New Message", "Delete", "Archive", "Reply", "Flag", "Move"]) {
      expect(screen.queryByRole("button", { name })).toBeNull();
    }
    expect(screen.queryByRole("textbox", { name: "Search" })).toBeNull();
  });
});
