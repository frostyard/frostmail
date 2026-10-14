// CONTRACT TEST for task card T-0152 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { colors, norm, timed, WEEK } from "./fixtures";
import { TimeGrid, type TimeGridProps } from "./TimeGrid";

// The test DOM has no layout: every element is 100 wide and the grid's
// full height, from the page's top-left corner, so a day column is 100
// pixels and the grid's y is the pointer's.
beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    x: 0,
    y: 0,
    left: 0,
    top: 0,
    width: 100,
    height: 1152,
    right: 100,
    bottom: 1152,
    toJSON: () => ({}),
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

// Thursday 14:00–15:30 (UTC), the user's own; and an invitation.
const review = timed("Review", "2026-10-08T14:00:00.000Z", "2026-10-08T15:30:00.000Z", { eventId: 2 });
const invited = timed("Invite", "2026-10-09T17:00:00Z", "2026-10-09T18:00:00Z", { eventId: 5, answer: "needsaction" });

function grid(over: Partial<TimeGridProps> = {}) {
  const props: TimeGridProps = {
    days: WEEK,
    occurrences: [review, invited],
    colors,
    timeZone: "UTC",
    locale: "en-US",
    today: "2026-10-08",
    now: new Date("2026-10-08T12:00:00Z"),
    selected: null,
    onSelect: vi.fn(),
    onShowDay: vi.fn(),
    canMove: (o) => o.answer === undefined,
    onMove: vi.fn(),
    onDraw: vi.fn(),
    ...over,
  };
  render(<TimeGrid {...props} />);
  return props;
}

const block = (name: RegExp) => screen.getByRole("button", { name });
const preview = () => document.querySelector("[data-drag-preview]");
const column = (name: string) => screen.getByRole("group", { name });
const down = (el: Element, x: number, y: number) =>
  fireEvent.pointerDown(el, { button: 0, clientX: x, clientY: y, pointerId: 1 });
const move = (x: number, y: number) => fireEvent.pointerMove(window, { clientX: x, clientY: y, pointerId: 1 });
const up = (x: number, y: number) => fireEvent.pointerUp(window, { clientX: x, clientY: y, pointerId: 1 });

describe("dragging in the time grid", () => {
  it("moves an occurrence in 15-minute steps and across days", () => {
    const props = grid();
    const el = block(/^Review/);
    down(el, 500, 700);
    move(502, 702);
    expect(preview()).toBeNull(); // under the threshold, still a press
    move(600, 748); // a day later, an hour later
    const shown = preview();
    expect(shown && within(column("Friday, October 9, 2026")).getByText("Review")).toBeTruthy();
    expect(norm(shown?.textContent ?? "")).toContain("3:00 – 4:30 PM");
    up(600, 748);
    fireEvent.click(el);
    expect(props.onMove).toHaveBeenCalledWith(review, {
      start: "2026-10-09T15:00:00.000Z",
      end: "2026-10-09T16:30:00.000Z",
    });
    expect(props.onSelect).not.toHaveBeenCalled();
    expect(preview()).toBeNull();
  });

  it("keeps a move within the days shown", () => {
    const props = grid();
    down(block(/^Review/), 500, 700);
    move(1500, 700); // ten columns right, past Saturday
    up(1500, 700);
    expect(props.onMove).toHaveBeenCalledWith(review, {
      start: "2026-10-10T14:00:00.000Z",
      end: "2026-10-10T15:30:00.000Z",
    });
  });

  it("resizes from the bottom edge, to no less than 15 minutes", () => {
    const props = grid();
    const handle = block(/^Review/).querySelector("[data-resize]") as HTMLElement;
    down(handle, 500, 760);
    move(500, 700); // 75 minutes up
    expect(norm(preview()?.textContent ?? "")).toContain("2:00 – 2:15 PM");
    up(500, 700);
    expect(props.onMove).toHaveBeenCalledWith(review, {
      start: "2026-10-08T14:00:00.000Z",
      end: "2026-10-08T14:15:00.000Z",
    });
  });

  it("draws a new event across empty time", () => {
    const props = grid();
    down(column("Wednesday, October 7, 2026"), 350, 9 * 48);
    move(350, 10 * 48 + 10);
    expect(norm(preview()?.textContent ?? "")).toBe("New Event9:00 – 10:15 AM");
    up(350, 10 * 48 + 10);
    expect(props.onDraw).toHaveBeenCalledWith({ start: "2026-10-07T09:00:00.000Z", end: "2026-10-07T10:15:00.000Z" });
    expect(props.onMove).not.toHaveBeenCalled();
  });

  it("puts everything back on Escape", () => {
    const props = grid();
    const outside = vi.fn();
    window.addEventListener("keydown", outside);
    down(block(/^Review/), 500, 700);
    move(500, 800);
    expect(preview()).not.toBeNull();
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(preview()).toBeNull();
    expect(outside).not.toHaveBeenCalled();
    up(500, 800);
    expect(props.onMove).not.toHaveBeenCalled();
    window.removeEventListener("keydown", outside);
  });

  it("treats a press that does not move as a click, and leaves invitations alone", () => {
    const props = grid();
    const el = block(/^Review/);
    down(el, 500, 700);
    up(500, 700);
    fireEvent.click(el);
    expect(props.onSelect).toHaveBeenCalledWith(review);

    const invite = block(/^Invite/);
    expect(invite.querySelector("[data-resize]")).toBeNull();
    down(invite, 600, 820);
    move(600, 900);
    expect(preview()).toBeNull();
    up(600, 900);
    expect(props.onMove).not.toHaveBeenCalled();
  });

  it("treats a drag back to where it began as a click", () => {
    // maild's instants have no milliseconds; the drag's do.
    const plain = timed("Plain", "2026-10-06T14:00:00Z", "2026-10-06T15:00:00Z", { eventId: 7 });
    const props = grid({ occurrences: [plain] });
    const el = block(/^Plain/);
    down(el, 250, 700);
    move(250, 800);
    move(250, 700);
    up(250, 700);
    fireEvent.click(el);
    expect(props.onMove).not.toHaveBeenCalled();
    expect(props.onSelect).toHaveBeenCalledWith(plain);
  });

  it("drags nothing without the handlers", () => {
    grid({ canMove: undefined, onMove: undefined, onDraw: undefined });
    down(block(/^Review/), 500, 700);
    move(600, 800);
    down(column("Wednesday, October 7, 2026"), 350, 400);
    move(350, 500);
    expect(preview()).toBeNull();
  });
});
