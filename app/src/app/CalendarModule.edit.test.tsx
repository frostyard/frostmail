// CONTRACT TEST for task card T-0146 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

// The fixture's week is October 4–10, 2026 (UTC); the user organizes 302
// Design review (Thursday), is invited to 303 Lunch with Ann (Friday), and
// cannot change 304 Holiday (a read-only calendar). Each test adds a series
// the user organizes: Team sync, daily at 10:00 from Tuesday, five times.
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
  return { mock, calls };
}

const day = (name: string) => screen.getByRole("group", { name });
const pane = () => within(screen.getByRole("region", { name: "Event details" }));

async function select(dayName: string, summary: RegExp) {
  fireEvent.click(within(day(dayName)).getByRole("button", { name: summary }));
  await pane().findByRole("heading", { name: summary });
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
  useUI.setState({ calendarDate: "2026-10-08" });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("Calendar edits from the event pane", () => {
  it("edits and deletes an event the user organizes", async () => {
    const { calls } = await setup();
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    await select("Thursday, October 8, 2026", /^Design review/);
    fireEvent.click(pane().getByRole("button", { name: "Edit Event" }));
    await waitFor(() => expect(open).toHaveBeenCalled());
    expect(String(open.mock.calls[0]?.[0])).toMatch(/#\/event\/302$/);
    expect(open.mock.calls[0]?.[1]).toBe("event-302");

    fireEvent.click(pane().getByRole("button", { name: "Delete Event" }));
    await waitFor(() => expect(calls("calendar.delete").at(-1)?.params).toEqual({ id: 302 }));
    await waitFor(() =>
      expect(within(day("Thursday, October 8, 2026")).queryByRole("button", { name: /^Design review/ })).toBeNull(),
    );
    expect(await pane().findByText("No Event Selected")).toBeTruthy();
  });

  it("offers only Edit for an invitation, and neither in a read-only calendar", async () => {
    await setup();
    await select("Friday, October 9, 2026", /^Lunch with Ann/);
    expect(pane().getByRole("button", { name: "Edit Event" })).toBeTruthy();
    expect(pane().queryByRole("button", { name: "Delete Event" })).toBeNull();

    useUI.getState().selectOccurrence({ eventId: 304, recurrenceId: "" }, "2026-10-12");
    await pane().findByRole("heading", { name: "Holiday" });
    expect(pane().queryByRole("button", { name: "Edit Event" })).toBeNull();
    expect(pane().queryByRole("button", { name: "Delete Event" })).toBeNull();
  });

  it("deletes one occurrence of a series through the scope sheet", async () => {
    const { calls } = await setup();
    await select("Thursday, October 8, 2026", /^Team sync/);
    const series = useUI.getState().calendarSelected?.eventId;
    expect(series).toBe(308); // the mock's next ID
    fireEvent.click(pane().getByRole("button", { name: "Delete Event" }));
    const sheet = within(await screen.findByRole("dialog", { name: "Delete Repeating Event" }));
    expect(sheet.getAllByRole("button").map((b) => b.textContent)).toEqual([
      "This Event",
      "All Future Events",
      "All Events",
      "Cancel",
    ]);
    fireEvent.click(sheet.getByRole("button", { name: "This Event" }));
    await waitFor(() =>
      expect(calls("calendar.delete").at(-1)?.params).toEqual({
        id: series,
        recurrenceId: "2026-10-08T10:00:00.000Z",
        scope: "this",
      }),
    );
    await waitFor(() =>
      expect(within(day("Thursday, October 8, 2026")).queryByRole("button", { name: /^Team sync/ })).toBeNull(),
    );
    expect(within(day("Friday, October 9, 2026")).getByRole("button", { name: /^Team sync/ })).toBeTruthy();

    // From the first occurrence the sheet has no All Future Events; Cancel writes nothing.
    await select("Tuesday, October 6, 2026", /^Team sync/);
    fireEvent.click(pane().getByRole("button", { name: "Delete Event" }));
    const first = within(await screen.findByRole("dialog", { name: "Delete Repeating Event" }));
    expect(first.queryByRole("button", { name: "All Future Events" })).toBeNull();
    const deletes = calls("calendar.delete").length;
    fireEvent.click(first.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Delete Repeating Event" })).toBeNull());
    expect(calls("calendar.delete")).toHaveLength(deletes);
    expect(within(day("Tuesday, October 6, 2026")).getByRole("button", { name: /^Team sync/ })).toBeTruthy();
  });
});
