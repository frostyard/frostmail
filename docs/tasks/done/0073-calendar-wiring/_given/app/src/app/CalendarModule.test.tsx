// CONTRACT TEST for task card T-0073 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { named } from "../features/calendar/fixtures";
import { today } from "../lib/calendarDates";
import { CALENDARS, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { mock, calls };
}

const key = (k: string, mods: { ctrlKey?: boolean; altKey?: boolean } = {}) =>
  fireEvent.keyDown(window, { key: k, ...mods });
const ctrl = (k: string) => key(k, { ctrlKey: true });
const day = (name: string) => screen.getByRole("group", { name });
const title = () => document.querySelector("[data-segment='calendar']")?.textContent ?? "";

async function showCalendar() {
  ctrl("2");
  await screen.findByRole("group", { name: "Thursday, October 8, 2026" });
  await within(day("Thursday, October 8, 2026")).findByRole("button", { name: /^Standup/ });
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
  useUI.setState({ calendarDate: "2026-10-08" });
});

describe("Calendar module", () => {
  it("opens with Ctrl+2 on the selected date's week, and leaves with Ctrl+1", async () => {
    const { calls } = await setup();
    const mailBar = screen.getByRole("toolbar", { name: "Modules" });
    expect(within(mailBar).getByRole("button", { name: "Calendar" })).toBeTruthy();
    await showCalendar();
    expect(useUI.getState().module).toBe("calendar");
    expect(screen.queryByRole("tree", { name: "Mailboxes" })).toBeNull();
    const bar = screen.getByRole("toolbar", { name: "Modules" });
    expect(within(bar).getByRole("button", { name: "Calendar" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("radio", { name: "Week" }).getAttribute("aria-checked")).toBe("true");
    expect(title()).toContain("October 2026");
    for (const name of ["Sunday, October 4, 2026", "Saturday, October 10, 2026"]) expect(day(name)).toBeTruthy();
    const thursday = within(day("Thursday, October 8, 2026"));
    expect(thursday.getByRole("button", { name: named("Standup, 9:00 – 9:15 AM, Room 4") })).toBeTruthy();
    expect(thursday.getByRole("button", { name: named("Design review, 2:00 – 3:30 PM, Studio B") })).toBeTruthy();
    expect(
      calls("calendar.range").some((c) =>
        expect.objectContaining({ from: "2026-10-04", to: "2026-10-11", timeZone: "UTC" }).asymmetricMatch(c.params),
      ),
    ).toBe(true);
    ctrl("1");
    await screen.findByRole("tree", { name: "Mailboxes" });
    expect(useUI.getState().module).toBe("mail");
  });

  it("lists the calendars in the sidebar and hides one", async () => {
    const { calls } = await setup();
    await showCalendar();
    const section = screen.getByRole("group", { name: "test1@mailtest.test" });
    expect(
      within(section)
        .getAllByRole("checkbox")
        .map((c) => c.textContent),
    ).toEqual(["Work", "Home", "Holidays"]);
    fireEvent.click(within(section).getByRole("checkbox", { name: "Work" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: /^Standup/ })).toBeNull());
    expect(calls("account.setCollection").at(-1)?.params).toEqual({ id: CALENDARS.work, enabled: false });
    await waitFor(() =>
      expect(screen.getByRole("checkbox", { name: "Work" }).getAttribute("aria-checked")).toBe("false"),
    );
  });

  it("switches views from the toolbar and with Ctrl+Alt+1 to 3", async () => {
    await setup();
    await showCalendar();
    fireEvent.click(screen.getByRole("radio", { name: "Month" }));
    await waitFor(() => expect(screen.getAllByRole("gridcell")).toHaveLength(42));
    const cell = screen.getByRole("gridcell", { name: "Thursday, October 8, 2026" });
    expect(within(cell).getByRole("button", { name: named("Standup, 9:00 AM") })).toBeTruthy();
    expect(useUI.getState().calendarView).toBe("month");
    key("1", { ctrlKey: true, altKey: true });
    await waitFor(() => expect(screen.queryByRole("group", { name: "Friday, October 9, 2026" })).toBeNull());
    expect(day("Thursday, October 8, 2026")).toBeTruthy();
    expect(title()).toContain("Thursday, October 8, 2026");
    key("2", { ctrlKey: true, altKey: true });
    await screen.findByRole("group", { name: "Friday, October 9, 2026" });
    key("3", { ctrlKey: true, altKey: true });
    await waitFor(() => expect(screen.getAllByRole("gridcell")).toHaveLength(42));
  });

  it("pages, moves by day, and returns to today", async () => {
    await setup();
    await showCalendar();
    fireEvent.click(screen.getByRole("button", { name: "Next Week" }));
    await screen.findByRole("group", { name: "Thursday, October 15, 2026" });
    ctrl("ArrowLeft");
    ctrl("ArrowLeft");
    await screen.findByRole("group", { name: "Thursday, October 1, 2026" });
    expect(title()).toContain("Sep – Oct 2026");
    key("1", { ctrlKey: true, altKey: true });
    await waitFor(() => expect(title()).toContain("Thursday, October 1, 2026"));
    key("ArrowRight");
    await screen.findByRole("group", { name: "Friday, October 2, 2026" });
    ctrl("t");
    expect(useUI.getState().calendarDate).toBe(today("UTC", new Date()));
  });

  it("selects a date in the small month, and opens a day from the month view", async () => {
    await setup();
    await showCalendar();
    fireEvent.click(screen.getByRole("button", { name: "Thursday, October 15, 2026" }));
    await screen.findByRole("group", { name: "Saturday, October 17, 2026" });
    fireEvent.click(screen.getByRole("radio", { name: "Month" }));
    fireEvent.doubleClick(await screen.findByRole("gridcell", { name: "Tuesday, October 13, 2026" }));
    await screen.findByRole("group", { name: "Tuesday, October 13, 2026" });
    expect(useUI.getState().calendarView).toBe("day");
    expect(screen.getByRole("radio", { name: "Day" }).getAttribute("aria-checked")).toBe("true");
  });

  it("shows the selected occurrence in the event pane, and Escape clears it", async () => {
    const { calls } = await setup();
    await showCalendar();
    expect(screen.getByText("No Event Selected")).toBeTruthy();
    fireEvent.click(within(day("Thursday, October 8, 2026")).getByRole("button", { name: /^Standup/ }));
    expect(await screen.findByRole("heading", { name: "Standup" })).toBeTruthy();
    expect(calls("calendar.event").at(-1)?.params).toEqual({ id: 301, recurrenceId: "2026-10-08T09:00:00.000Z" });
    expect(screen.getByText("Every day, 30 times")).toBeTruthy();
    expect(useUI.getState().calendarSelected).toEqual({ eventId: 301, recurrenceId: "2026-10-08T09:00:00.000Z" });
    key("Escape");
    await screen.findByText("No Event Selected");
    expect(useUI.getState().calendarSelected).toBeNull();
  });
});
