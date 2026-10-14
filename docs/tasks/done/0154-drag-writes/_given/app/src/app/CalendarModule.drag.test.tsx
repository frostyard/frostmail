// CONTRACT TEST for task card T-0154 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";
import { usePimUndo } from "./pimUndo";

// The fixture's week is October 4–10, 2026 (UTC): 302 Design review,
// Thursday 14:00–15:30, is the user's; 303 Lunch with Ann, Friday, an
// invitation. Each test adds Team sync, a daily series of the user's at
// 10:00 from Tuesday. The test DOM has no layout: every element is 100
// wide from the page's top-left, so a day column is 100 pixels.
async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  await new Client(mock).calendar.create({
    fields: {
      summary: "Team sync",
      allDay: false,
      start: "2026-10-06T10:00:00.000Z",
      end: "2026-10-06T10:30:00.000Z",
      timeZone: "UTC",
      rrule: "FREQ=DAILY;COUNT=5",
    },
  });
  const open = vi.spyOn(window, "open").mockReturnValue(null);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  fireEvent.keyDown(window, { key: "2", ctrlKey: true });
  await screen.findByRole("group", { name: "Thursday, October 8, 2026" });
  await within(day("Thursday, October 8, 2026")).findByRole("button", { name: /^Design review/ });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { calls, open };
}

const day = (name: string) => screen.getByRole("group", { name });
const block = (dayName: string, name: RegExp) => within(day(dayName)).getByRole("button", { name });
const down = (el: Element, x: number, y: number) =>
  fireEvent.pointerDown(el, { button: 0, clientX: x, clientY: y, pointerId: 1 });
const move = (x: number, y: number) => fireEvent.pointerMove(window, { clientX: x, clientY: y, pointerId: 1 });
const up = (x: number, y: number) => fireEvent.pointerUp(window, { clientX: x, clientY: y, pointerId: 1 });

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

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
  useUI.setState({ calendarDate: "2026-10-08" });
  usePimUndo.setState(usePimUndo.getInitialState());
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

describe("dragging events in Calendar", () => {
  it("writes a moved event in its zone, and keeps the change for undo", async () => {
    const { calls } = await setup();
    down(block("Thursday, October 8, 2026", /^Design review/), 500, 700);
    move(600, 748); // a day and an hour later
    up(600, 748);
    await waitFor(() =>
      expect(calls("calendar.update").at(-1)?.params).toEqual({
        id: 302,
        fields: { allDay: false, start: "2026-10-09T15:00:00.000Z", end: "2026-10-09T16:30:00.000Z", timeZone: "UTC" },
      }),
    );
    await waitFor(() => expect(usePimUndo.getState().calendar).toHaveLength(1));
    await within(day("Friday, October 9, 2026")).findByRole("button", { name: /^Design review/ });
  });

  it("asks the scope for an occurrence of a series", async () => {
    const { calls } = await setup();
    down(block("Wednesday, October 7, 2026", /^Team sync/), 300, 480);
    move(300, 528); // an hour later
    up(300, 528);
    const sheet = within(await screen.findByRole("dialog", { name: "Change Repeating Event" }));
    fireEvent.click(sheet.getByRole("button", { name: "This Event" }));
    await waitFor(() =>
      expect(calls("calendar.update").at(-1)?.params).toEqual({
        id: expect.any(Number),
        recurrenceId: "2026-10-07T10:00:00.000Z",
        scope: "this",
        fields: { allDay: false, start: "2026-10-07T11:00:00.000Z", end: "2026-10-07T11:30:00.000Z", timeZone: "UTC" },
      }),
    );

    // Cancel writes nothing.
    const updates = calls("calendar.update").length;
    down(block("Thursday, October 8, 2026", /^Team sync/), 400, 480);
    move(400, 600);
    up(400, 600);
    fireEvent.click(
      within(await screen.findByRole("dialog", { name: "Change Repeating Event" })).getByRole("button", {
        name: "Cancel",
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(calls("calendar.update")).toHaveLength(updates);
  });

  it("opens the editor on a range drawn across empty time", async () => {
    const { open, calls } = await setup();
    const creates = calls("calendar.create").length; // the setup's series
    down(day("Monday, October 5, 2026"), 150, 9 * 48);
    move(150, 10 * 48 + 10);
    up(150, 10 * 48 + 10);
    await waitFor(() => expect(open).toHaveBeenCalled());
    expect(String(open.mock.calls[0]?.[0])).toMatch(
      /#\/event\/new\?allDay=0&start=2026-10-05T09%3A00%3A00\.000Z&end=2026-10-05T10%3A15%3A00\.000Z$/,
    );
    expect(calls("calendar.create")).toHaveLength(creates);
  });

  it("moves by days in the month view, and leaves invitations alone", async () => {
    const { calls } = await setup();
    fireEvent.keyDown(window, { key: "3", ctrlKey: true, altKey: true });
    const cell = (name: string) => screen.getByRole("gridcell", { name });
    const review = await within(cell("Thursday, October 8, 2026")).findByRole("button", { name: /^Design review/ });
    expect(
      within(cell("Friday, October 9, 2026"))
        .getByRole("button", { name: /^Lunch with Ann/ })
        .getAttribute("draggable"),
    ).toBe("false");
    const dt = transfer();
    fireEvent.dragStart(review, { dataTransfer: dt });
    fireEvent.dragOver(cell("Monday, October 12, 2026"), { dataTransfer: dt });
    fireEvent.drop(cell("Monday, October 12, 2026"), { dataTransfer: dt });
    await waitFor(() =>
      expect(calls("calendar.update").at(-1)?.params).toEqual({
        id: 302,
        fields: { allDay: false, start: "2026-10-12T14:00:00.000Z", end: "2026-10-12T15:30:00.000Z", timeZone: "UTC" },
      }),
    );
  });
});
