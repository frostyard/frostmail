// CONTRACT TEST for task card T-0099 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { type CalendarSection, CalendarSidebar, type CalendarSidebarProps } from "./CalendarSidebar";

const sections: CalendarSection[] = [
  {
    accountId: 1,
    title: "ann@icloud.com",
    calendars: [
      { id: 1, name: "Maintenance", color: "#ff9500", enabled: true, readOnly: false, isDefault: true },
      { id: 2, name: "Calendar", color: "#3366cc", enabled: true, readOnly: false, isDefault: false },
      { id: 3, name: "Holidays", color: "#34c759", enabled: true, readOnly: true, isDefault: false },
    ],
  },
];

function sidebar(over: Partial<CalendarSidebarProps> = {}) {
  const props: CalendarSidebarProps = {
    sections,
    onToggle: vi.fn(),
    onMakeDefault: vi.fn(),
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

const row = (name: RegExp) => screen.getByRole("checkbox", { name });
const item = () => screen.getByRole("menuitemcheckbox", { name: "Use as Default Calendar" });

describe("CalendarSidebar default calendar", () => {
  it("makes a calendar the default from its menu", () => {
    const props = sidebar();
    fireEvent.contextMenu(row(/^Calendar/));
    expect(screen.getByRole("menu")).toBeTruthy();
    expect(item().getAttribute("aria-checked")).toBe("false");
    expect(item().getAttribute("aria-disabled")).toBeNull();
    fireEvent.click(item());
    expect(props.onMakeDefault).toHaveBeenCalledWith(2);
    expect(screen.queryByRole("menu")).toBeNull();
    expect(props.onToggle).not.toHaveBeenCalled();
  });

  it("shows the default checked, and offers nothing for it or a read-only calendar", () => {
    const props = sidebar();
    fireEvent.contextMenu(row(/^Maintenance/));
    expect(item().getAttribute("aria-checked")).toBe("true");
    expect(item().getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(item());
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    fireEvent.contextMenu(row(/^Holidays/));
    expect(item().getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(item());
    expect(props.onMakeDefault).not.toHaveBeenCalled();
  });

  it("opens the menu from the keyboard", () => {
    const props = sidebar();
    const calendar = row(/^Calendar/);
    calendar.focus();
    fireEvent.keyDown(calendar, { key: "ContextMenu" });
    expect(item()).toBeTruthy();
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    expect(screen.queryByRole("menu")).toBeNull();
    fireEvent.keyDown(calendar, { key: "F10", shiftKey: true });
    fireEvent.click(item());
    expect(props.onMakeDefault).toHaveBeenCalledWith(2);
  });

  it("has no menu without onMakeDefault", () => {
    sidebar({ onMakeDefault: undefined });
    fireEvent.contextMenu(row(/^Calendar/));
    expect(screen.queryByRole("menu")).toBeNull();
  });
});
