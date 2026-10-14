// CONTRACT TEST for task card T-0150 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

// The fixture's week is October 4–10, 2026 (UTC), a week the tests' own
// clock has passed: a new event on its days starts at 9:00 AM. 302 Design
// review (Thursday) is the user's; Team sync, added here, is a series the
// user organizes, daily at 10:00 from Tuesday.
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
  return { open, calls };
}

const day = (name: string) => screen.getByRole("group", { name });
const pane = () => within(screen.getByRole("region", { name: "Event details" }));
const view = () => screen.getByRole("region", { name: "Calendar view" });

async function select(dayName: string, summary: RegExp) {
  fireEvent.click(within(day(dayName)).getByRole("button", { name: summary }));
  await pane().findByRole("heading", { name: summary });
}

const newEvent = "#/event/new?allDay=0&start=2026-10-08T09%3A00%3A00.000Z&end=2026-10-08T10%3A00%3A00.000Z";

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
  useUI.setState({ calendarDate: "2026-10-08" });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("Calendar's New Event and keys", () => {
  it("starts a new event on the selected date from the toolbar and with Ctrl+N", async () => {
    const { open } = await setup();
    const button = screen.getByRole("button", { name: "New Event" });
    expect(button.getAttribute("title")).toBe("New Event (Ctrl+N)");
    fireEvent.click(button);
    await waitFor(() => expect(open).toHaveBeenCalledTimes(1));
    expect(String(open.mock.calls[0]?.[0])).toMatch(new RegExp(`${newEvent.replace(/[?.]/g, "\\$&")}$`));
    expect(String(open.mock.calls[0]?.[1])).toMatch(/^event-new-\d+$/);

    useUI.getState().setCalendarDate("2026-10-09");
    fireEvent.keyDown(window, { key: "n", ctrlKey: true });
    await waitFor(() => expect(open).toHaveBeenCalledTimes(2));
    expect(String(open.mock.calls[1]?.[0])).toContain("start=2026-10-09T09%3A00%3A00.000Z");
  });

  it("opens the selected occurrence with Enter, and shows the day without one", async () => {
    const { open } = await setup();
    await select("Thursday, October 8, 2026", /^Design review/);
    view().focus();
    fireEvent.keyDown(window, { key: "Enter" });
    await waitFor(() => expect(open).toHaveBeenCalled());
    expect(String(open.mock.calls[0]?.[0])).toMatch(/#\/event\/302$/);

    fireEvent.keyDown(window, { key: "Escape" });
    expect(useUI.getState().calendarSelected).toBeNull();
    view().focus();
    fireEvent.keyDown(window, { key: "Enter" });
    expect(useUI.getState().calendarView).toBe("day");
    expect(open).toHaveBeenCalledTimes(1);
  });

  it("deletes the selected occurrence with Delete, through the scope sheet for a series", async () => {
    const { calls } = await setup();
    await select("Thursday, October 8, 2026", /^Design review/);
    fireEvent.keyDown(window, { key: "Delete" });
    await waitFor(() => expect(calls("calendar.delete").at(-1)?.params).toEqual({ id: 302 }));

    await select("Friday, October 9, 2026", /^Team sync/);
    fireEvent.keyDown(window, { key: "Backspace" });
    const sheet = within(await screen.findByRole("dialog", { name: "Delete Repeating Event" }));
    fireEvent.click(sheet.getByRole("button", { name: "This Event" }));
    await waitFor(() =>
      expect(calls("calendar.delete").at(-1)?.params).toMatchObject({
        recurrenceId: "2026-10-09T10:00:00.000Z",
        scope: "this",
      }),
    );
  });

  it("deletes nothing without a selection", async () => {
    const { calls } = await setup();
    fireEvent.keyDown(window, { key: "Delete" });
    await Promise.resolve();
    expect(calls("calendar.delete")).toHaveLength(0);
  });
});
