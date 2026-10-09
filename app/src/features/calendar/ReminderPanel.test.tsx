// CONTRACT TEST for task card T-0076 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Reminder } from "../../rpc/gen/api";
import { colors, norm } from "./fixtures";
import { ReminderPanel, type ReminderPanelProps } from "./ReminderPanel";

const standup: Reminder = {
  id: "r1",
  eventId: 301,
  recurrenceId: "2026-10-08T09:00:00.000Z",
  calendarId: 1,
  summary: "Standup",
  location: "Room 4",
  allDay: false,
  start: "2026-10-08T09:00:00Z",
  startDate: "",
  dueAt: "2026-10-08T08:50:00Z",
};
const review: Reminder = {
  ...standup,
  id: "r2",
  eventId: 302,
  recurrenceId: "",
  calendarId: 2,
  summary: "Design review",
  location: "",
  start: "2026-10-08T14:00:00Z",
};

function panel(over: Partial<ReminderPanelProps> = {}) {
  const props: ReminderPanelProps = {
    reminders: [standup, review],
    colors,
    timeZone: "UTC",
    locale: "en-US",
    now: new Date("2026-10-08T12:00:00Z"),
    onSnooze: vi.fn(),
    onDismiss: vi.fn(),
    onOpen: vi.fn(),
    onClose: vi.fn(),
    ...over,
  };
  const view = render(<ReminderPanel {...props} />);
  return { props, view };
}

const rows = () => within(screen.getByRole("list", { name: "Reminders" })).getAllByRole("listitem");

describe("ReminderPanel", () => {
  it("lists the reminders with when and where", () => {
    panel();
    expect(screen.getByText("Reminders", { selector: "h1" })).toBeTruthy();
    const [first, second] = rows();
    expect(first?.textContent).toContain("Standup");
    expect(norm(first?.textContent ?? "")).toContain("Today, 9:00 AM · Room 4");
    expect(norm(second?.textContent ?? "")).toContain("Today, 2:00 PM");
    expect(second?.textContent).not.toContain("·");
    expect(first?.style.getPropertyValue("--event-color")).toBe("#3366cc");
    expect(second?.style.getPropertyValue("--event-color")).toBe("var(--accent)");
  });

  it("dismisses one or all", () => {
    const { props } = panel();
    fireEvent.click(within(rows()[0] as HTMLElement).getByRole("button", { name: "Dismiss" }));
    expect(props.onDismiss).toHaveBeenLastCalledWith(["r1"]);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss All" }));
    expect(props.onDismiss).toHaveBeenLastCalledWith(["r1", "r2"]);
  });

  it("has no Dismiss All for one", () => {
    panel({ reminders: [standup] });
    expect(screen.queryByRole("button", { name: "Dismiss All" })).toBeNull();
  });

  it("snoozes for a chosen while", () => {
    const { props } = panel();
    fireEvent.click(within(rows()[1] as HTMLElement).getByRole("button", { name: "Snooze" }));
    const menu = screen.getByRole("menu");
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((i) => i.textContent),
    ).toEqual(["5 minutes", "10 minutes", "15 minutes", "1 hour", "Tomorrow"]);
    fireEvent.click(within(menu).getByRole("menuitem", { name: "1 hour" }));
    expect(props.onSnooze).toHaveBeenCalledWith("r2", new Date("2026-10-08T13:00:00Z"));
  });

  it("opens an occurrence from its title, and closes", () => {
    const { props, view } = panel();
    fireEvent.click(screen.getByRole("button", { name: "Standup" }));
    expect(props.onOpen).toHaveBeenCalledWith(standup);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    fireEvent.keyDown(view.container.firstElementChild as HTMLElement, { key: "Escape" });
    expect(props.onClose).toHaveBeenCalledTimes(2);
  });
});
