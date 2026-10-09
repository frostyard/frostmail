// CONTRACT TEST for task card T-0071 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { allDay, colors, named, norm, timed, WEEK } from "./fixtures";
import { TimeGrid, type TimeGridProps } from "./TimeGrid";

const standup = timed("Standup", "2026-10-08T09:00:00Z", "2026-10-08T09:15:00Z", {
  eventId: 1,
  recurrenceId: "2026-10-08T09:00:00.000Z",
  location: "Room 4",
  recurring: true,
});
const review = timed("Review", "2026-10-08T09:00:00Z", "2026-10-08T10:00:00Z", { eventId: 2, calendarId: 2 });
const lunch = timed("Lunch", "2026-10-06T12:00:00Z", "2026-10-06T13:00:00Z", { eventId: 3 });
const cancelled = timed("Gone", "2026-10-09T15:00:00Z", "2026-10-09T16:00:00Z", { eventId: 4, status: "cancelled" });
const invited = timed("Invite", "2026-10-09T17:00:00Z", "2026-10-09T18:00:00Z", { eventId: 5, answer: "needsaction" });
const holiday = allDay("Holiday", "2026-10-05", "2026-10-07", { eventId: 6 });

function grid(over: Partial<TimeGridProps> = {}) {
  const props: TimeGridProps = {
    days: WEEK,
    occurrences: [standup, review, lunch, cancelled, invited, holiday],
    colors,
    timeZone: "UTC",
    locale: "en-US",
    today: "2026-10-08",
    now: new Date("2026-10-08T14:30:00Z"),
    selected: null,
    onSelect: vi.fn(),
    onShowDay: vi.fn(),
    ...over,
  };
  const view = render(<TimeGrid {...props} />);
  return { props, view };
}

const column = (date: string) => screen.getByRole("group", { name: date });

describe("TimeGrid", () => {
  it("heads each day and opens it in the day view", () => {
    const { props } = grid();
    const headers = [
      "Sunday, October 4, 2026",
      "Monday, October 5, 2026",
      "Tuesday, October 6, 2026",
      "Wednesday, October 7, 2026",
      "Thursday, October 8, 2026",
      "Friday, October 9, 2026",
      "Saturday, October 10, 2026",
    ].map((name) => screen.getByRole("button", { name }));
    expect(headers[4]?.getAttribute("aria-current")).toBe("date");
    expect(headers[3]?.getAttribute("aria-current")).toBeNull();
    expect(headers[4]?.textContent).toContain("8");
    fireEvent.click(headers[2] as HTMLElement);
    expect(props.onShowDay).toHaveBeenCalledWith("2026-10-06");
  });

  it("places timed occurrences in their day's column by time", () => {
    grid();
    const thursday = column("Thursday, October 8, 2026");
    const block = within(thursday).getByRole("button", { name: named("Standup, 9:00 – 9:15 AM, Room 4") });
    expect(block.style.top).toBe("432px");
    expect(block.style.height).toBe("18px");
    const other = within(thursday).getByRole("button", { name: named("Review, 9:00 – 10:00 AM") });
    expect(other.style.top).toBe("432px");
    expect(other.style.height).toBe("48px");
    expect([other.style.left, block.style.left]).toEqual(["0%", "50%"]);
    expect(
      within(column("Tuesday, October 6, 2026")).getByRole("button", { name: named("Lunch, 12:00 – 1:00 PM") }),
    ).toBeTruthy();
    expect(within(column("Wednesday, October 7, 2026")).queryAllByRole("button")).toHaveLength(0);
  });

  it("colors occurrences by calendar", () => {
    grid();
    expect(screen.getByRole("button", { name: /^Standup/ }).style.getPropertyValue("--event-color")).toBe("#3366cc");
    expect(screen.getByRole("button", { name: /^Review/ }).style.getPropertyValue("--event-color")).toBe(
      "var(--accent)",
    );
  });

  it("strikes cancelled occurrences and dashes unanswered ones", () => {
    grid();
    const gone = screen.getByRole("button", { name: /^Gone/ });
    expect(gone.className).toContain("opacity-60");
    expect(within(gone).getByText("Gone").className).toContain("line-through");
    expect(screen.getByRole("button", { name: /^Invite/ }).className).toContain("border-dashed");
    expect(screen.getByRole("button", { name: /^Standup/ }).className).not.toContain("border-dashed");
  });

  it("selects an occurrence", () => {
    const { props, view } = grid();
    fireEvent.click(screen.getByRole("button", { name: /^Standup/ }));
    expect(props.onSelect).toHaveBeenCalledWith(standup);
    expect(screen.getByRole("button", { name: /^Standup/ }).getAttribute("aria-pressed")).toBe("false");
    view.rerender(<TimeGrid {...props} selected={{ eventId: 1, recurrenceId: "2026-10-08T09:00:00.000Z" }} />);
    expect(screen.getByRole("button", { name: /^Standup/ }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button", { name: /^Review/ }).getAttribute("aria-pressed")).toBe("false");
  });

  it("labels the hours", () => {
    grid();
    for (const hour of ["1 AM", "9 AM", "12 PM", "11 PM"]) {
      expect(screen.getAllByText((text) => norm(text) === hour).length).toBeGreaterThan(0);
    }
  });

  it("draws the current time in today's column only", () => {
    const { view } = grid();
    const line = column("Thursday, October 8, 2026").querySelector<HTMLElement>("[data-now]");
    expect(line?.style.top).toBe(`${(14 * 60 + 30) * 0.8}px`);
    expect(view.container.querySelectorAll("[data-now]")).toHaveLength(1);
  });

  it("has no now line when today is not shown", () => {
    const { view } = grid({ today: "2026-10-20" });
    expect(view.container.querySelector("[data-now]")).toBeNull();
  });

  it("shows all-day occurrences as bars in the strip", () => {
    const { props } = grid();
    const bar = screen.getByRole("button", { name: "Holiday, all day" });
    fireEvent.click(bar);
    expect(props.onSelect).toHaveBeenCalledWith(holiday);
    expect(within(column("Monday, October 5, 2026")).queryByRole("button", { name: /Holiday/ })).toBeNull();
  });

  it("keeps the week's strip to three lanes", () => {
    const many = ["A", "B", "C", "D"].map((s) => allDay(s, "2026-10-08", "2026-10-09"));
    const { props } = grid({ occurrences: [...many, allDay("Other", "2026-10-06", "2026-10-07")] });
    for (const s of ["A", "B", "Other"]) expect(screen.getByRole("button", { name: `${s}, all day` })).toBeTruthy();
    for (const s of ["C", "D"]) expect(screen.queryByRole("button", { name: `${s}, all day` })).toBeNull();
    const more = screen.getByRole("button", { name: "Show 2 more on Thursday, October 8, 2026" });
    expect(more.textContent).toBe("2 more");
    fireEvent.click(more);
    expect(props.onShowDay).toHaveBeenCalledWith("2026-10-08");
  });

  it("shows every lane in the day view", () => {
    const many = ["A", "B", "C", "D"].map((s) => allDay(s, "2026-10-08", "2026-10-09"));
    grid({ days: ["2026-10-08"], occurrences: many });
    for (const s of ["A", "B", "C", "D"]) expect(screen.getByRole("button", { name: `${s}, all day` })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /more on/ })).toBeNull();
    expect(screen.getAllByRole("group")).toHaveLength(1);
  });

  it("reads times in the zone", () => {
    grid({
      timeZone: "America/New_York",
      occurrences: [timed("Early", "2026-10-08T13:00:00Z", "2026-10-08T14:00:00Z")],
    });
    const block = within(column("Thursday, October 8, 2026")).getByRole("button", {
      name: named("Early, 9:00 – 10:00 AM"),
    });
    expect(block.style.top).toBe("432px");
  });
});
