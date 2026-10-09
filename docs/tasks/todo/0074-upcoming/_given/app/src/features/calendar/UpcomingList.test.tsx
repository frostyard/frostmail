// CONTRACT TEST for task card T-0074 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { allDay, colors, timed } from "./fixtures";
import { UpcomingList, type UpcomingListProps } from "./UpcomingList";

const lunch = timed("Lunch with Ann", "2026-10-09T12:00:00Z", "2026-10-09T13:00:00Z", { eventId: 303 });
const holiday = allDay("Holiday", "2026-10-12", "2026-10-13", { eventId: 304, calendarId: 2 });

function list(over: Partial<UpcomingListProps> = {}) {
  const props: UpcomingListProps = {
    occurrences: [lunch, holiday],
    colors,
    timeZone: "UTC",
    locale: "en-US",
    now: new Date("2026-10-08T12:00:00Z"),
    onOpen: vi.fn(),
    ...over,
  };
  const view = render(<UpcomingList {...props} />);
  return { props, view };
}

describe("UpcomingList", () => {
  it("lists occurrences under Upcoming, named by title and when", () => {
    list();
    const region = screen.getByRole("region", { name: "Upcoming" });
    const rows = within(region).getAllByRole("button");
    expect(rows.map((r) => r.getAttribute("aria-label"))).toEqual(["Lunch with Ann, Fri", "Holiday, Mon"]);
    expect(rows[0]?.textContent).toContain("Lunch with Ann");
    expect(rows[0]?.textContent).toContain("Fri");
    expect(rows[0]?.style.getPropertyValue("--event-color")).toBe("#3366cc");
    expect(rows[1]?.style.getPropertyValue("--event-color")).toBe("var(--accent)");
  });

  it("opens an occurrence", () => {
    const { props } = list();
    fireEvent.click(screen.getByRole("button", { name: "Holiday, Mon" }));
    expect(props.onOpen).toHaveBeenCalledWith(holiday);
  });

  it("is nothing without occurrences", () => {
    const { view } = list({ occurrences: [] });
    expect(view.container.innerHTML).toBe("");
  });
});
