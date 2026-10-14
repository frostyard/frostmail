// CONTRACT TEST for task card T-0153 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { monthGrid } from "../../lib/calendarDates";
import { allDay, colors, timed, WEEK } from "./fixtures";
import { MonthGrid, type MonthGridProps } from "./MonthGrid";
import { TimeGrid, type TimeGridProps } from "./TimeGrid";

/** transfer is a DataTransfer that keeps what dragstart puts in it. */
function transfer() {
  const data = new Map<string, string>();
  return {
    get types() {
      return [...data.keys()];
    },
    setData: vi.fn((type: string, value: string) => void data.set(type, value)),
    getData: (type: string) => data.get(type) ?? "",
    dropEffect: "none",
    effectAllowed: "all",
  };
}

const standup = timed("Standup", "2026-10-08T09:00:00Z", "2026-10-08T09:15:00Z", { eventId: 1 });
const holiday = allDay("Holiday", "2026-10-08", "2026-10-10", { eventId: 2 });
const invited = timed("Invite", "2026-10-08T17:00:00Z", "2026-10-08T18:00:00Z", { eventId: 5, answer: "accepted" });

function month(over: Partial<MonthGridProps> = {}) {
  const props: MonthGridProps = {
    days: monthGrid("2026-10-08", 0),
    selectedDate: "2026-10-08",
    occurrences: [standup, holiday, invited],
    colors,
    timeZone: "UTC",
    locale: "en-US",
    today: "2026-10-08",
    selected: null,
    lines: 5,
    onSelect: vi.fn(),
    onSelectDate: vi.fn(),
    onShowDay: vi.fn(),
    canMove: (o) => o.answer === undefined,
    onMoveDays: vi.fn(),
    ...over,
  };
  render(<MonthGrid {...props} />);
  return props;
}

const cell = (name: string) => screen.getByRole("gridcell", { name });
const chip = (day: string, name: RegExp) => within(cell(day)).getByRole("button", { name });

afterEach(() => {
  vi.restoreAllMocks();
});

describe("dragging in the month view", () => {
  it("moves an occurrence by the days between the cells", () => {
    const props = month();
    const dt = transfer();
    const el = chip("Thursday, October 8, 2026", /^Standup/);
    expect(el.getAttribute("draggable")).toBe("true");
    fireEvent.dragStart(el, { dataTransfer: dt });
    expect(dt.types).toContain("application/x-frostmail-occurrence");
    const target = cell("Tuesday, October 13, 2026");
    // Over a cell, the drag may drop there.
    expect(fireEvent.dragOver(target, { dataTransfer: dt })).toBe(false);
    expect(target.hasAttribute("data-drop-target")).toBe(true);
    fireEvent.drop(target, { dataTransfer: dt });
    expect(props.onMoveDays).toHaveBeenCalledWith(standup, 5);
  });

  it("counts a multi-day occurrence's days from the cell it was taken from", () => {
    const props = month();
    const dt = transfer();
    fireEvent.dragStart(chip("Friday, October 9, 2026", /^Holiday/), { dataTransfer: dt });
    fireEvent.dragOver(cell("Wednesday, October 7, 2026"), { dataTransfer: dt });
    fireEvent.drop(cell("Wednesday, October 7, 2026"), { dataTransfer: dt });
    expect(props.onMoveDays).toHaveBeenCalledWith(holiday, -2);
  });

  it("writes nothing for a drop where it began, or after the drag ends elsewhere", () => {
    const props = month();
    const dt = transfer();
    const el = chip("Thursday, October 8, 2026", /^Standup/);
    fireEvent.dragStart(el, { dataTransfer: dt });
    fireEvent.drop(cell("Thursday, October 8, 2026"), { dataTransfer: dt });
    fireEvent.dragStart(el, { dataTransfer: dt });
    fireEvent.dragOver(cell("Friday, October 16, 2026"), { dataTransfer: dt });
    fireEvent.dragEnd(el, { dataTransfer: dt }); // Escape, or a drop outside the grid
    expect(cell("Friday, October 16, 2026").hasAttribute("data-drop-target")).toBe(false);
    fireEvent.drop(cell("Friday, October 16, 2026"), { dataTransfer: dt });
    expect(props.onMoveDays).not.toHaveBeenCalled();
  });

  it("leaves invitations, and other drags, alone", () => {
    const props = month();
    expect(chip("Thursday, October 8, 2026", /^Invite/).getAttribute("draggable")).toBe("false");
    // Something else dragged over a cell (a message from Mail) is not taken.
    const files = transfer();
    files.setData("text/plain", "hello");
    expect(fireEvent.dragOver(cell("Tuesday, October 13, 2026"), { dataTransfer: files })).toBe(true);
    fireEvent.drop(cell("Tuesday, October 13, 2026"), { dataTransfer: files });
    expect(props.onMoveDays).not.toHaveBeenCalled();
  });

  it("drags nothing without the handlers", () => {
    month({ canMove: undefined, onMoveDays: undefined });
    expect(chip("Thursday, October 8, 2026", /^Standup/).getAttribute("draggable")).toBe("false");
  });
});

describe("dragging in the all-day strip", () => {
  function strip(over: Partial<TimeGridProps> = {}) {
    // Seven columns of 100 pixels from the page's left edge.
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      width: 700,
      height: 30,
      right: 700,
      bottom: 30,
      toJSON: () => ({}),
    });
    const props: TimeGridProps = {
      days: WEEK,
      occurrences: [holiday, standup],
      colors,
      timeZone: "UTC",
      locale: "en-US",
      today: "2026-10-08",
      now: new Date("2026-10-08T12:00:00Z"),
      selected: null,
      onSelect: vi.fn(),
      onShowDay: vi.fn(),
      canMove: (o) => o.answer === undefined,
      onMoveDays: vi.fn(),
      ...over,
    };
    render(<TimeGrid {...props} />);
    return props;
  }

  it("moves an all-day occurrence by the columns between taking and dropping it", () => {
    const props = strip();
    const dt = transfer();
    const bar = screen.getByRole("button", { name: "Holiday, all day" });
    expect(bar.getAttribute("draggable")).toBe("true");
    // The press before the drag says where the bar was taken: Thursday's
    // column; the drag then shows a drop cell for each day.
    fireEvent.pointerDown(bar, { button: 0, clientX: 450, pointerId: 1 });
    fireEvent.dragStart(bar, { dataTransfer: dt });
    const saturday = document.querySelector('[data-drop-day="2026-10-10"]') as HTMLElement;
    expect(fireEvent.dragOver(saturday, { dataTransfer: dt })).toBe(false);
    fireEvent.drop(saturday, { dataTransfer: dt });
    expect(props.onMoveDays).toHaveBeenCalledWith(holiday, 2);
    expect(document.querySelector("[data-drop-day]")).toBeNull();
  });

  it("writes nothing for a drop in the same column", () => {
    const props = strip();
    const dt = transfer();
    const bar = screen.getByRole("button", { name: "Holiday, all day" });
    fireEvent.pointerDown(bar, { button: 0, clientX: 450, pointerId: 1 });
    fireEvent.dragStart(bar, { dataTransfer: dt });
    fireEvent.drop(document.querySelector('[data-drop-day="2026-10-08"]') as HTMLElement, { dataTransfer: dt });
    expect(props.onMoveDays).not.toHaveBeenCalled();
  });
});
