// Calendar's search in the window (docs/specs/pim-ui.md, Calendar search).
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { frameRoute } from "../testing/frame";
import { MainWindow } from "./MainWindow";

// The fixture's week is October 4–10, 2026 (UTC), which the tests' own
// clock has passed, so every result is under Earlier, the latest first.
// Team sync is a series the user organizes, daily at 10:00 from Tuesday.
async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  const client = new Client(mock);
  await client.calendar.create({
    fields: {
      summary: "Team sync",
      location: "Room 9",
      allDay: false,
      start: "2026-10-06T10:00:00.000Z",
      end: "2026-10-06T10:30:00.000Z",
      timeZone: "UTC",
      rrule: "FREQ=DAILY;COUNT=3",
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
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { calls, client };
}

const field = () => screen.getByRole("searchbox", { name: "Search" }) as HTMLInputElement;
const pane = () => within(screen.getByRole("region", { name: "Event details" }));
// Intl puts thin and narrow no-break spaces in ranges and before AM/PM.
const norm = (text: string | null) => (text ?? "").replace(/[\u2009\u202f]/g, " ");
const names = () => screen.getAllByRole("option").map((o) => norm(o.getAttribute("aria-label")));

async function search(text: string) {
  fireEvent.keyDown(window, { key: "f", ctrlKey: true, altKey: true });
  fireEvent.change(field(), { target: { value: text } });
  fireEvent.keyDown(field(), { key: "Enter" });
  await screen.findByRole("listbox", { name: "Search results" });
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
  useUI.setState({ calendarDate: "2026-10-08" });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("Calendar search", () => {
  it("opens the field with Ctrl+Alt+F and from the toolbar, with the keyboard in it", async () => {
    await setup();
    expect(screen.queryByRole("searchbox")).toBeNull();
    fireEvent.keyDown(window, { key: "f", ctrlKey: true, altKey: true });
    expect(field().placeholder).toBe("Search Events");
    expect(document.activeElement).toBe(field());
    expect(screen.queryByRole("button", { name: "Today" })).toBeNull();

    // Left empty, it closes; Escape closes it too.
    fireEvent.blur(field());
    expect(screen.queryByRole("searchbox")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Search Events" }));
    expect(document.activeElement).toBe(field());
    fireEvent.keyDown(field(), { key: "Escape" });
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(screen.getByRole("button", { name: "Today" })).toBeTruthy();
  });

  it("asks maild in the app's zone and lists the results in the view's place", async () => {
    const { calls } = await setup();
    await search("  team SYNC ");
    expect(calls("calendar.search").at(-1)?.params).toEqual({
      query: "team SYNC",
      timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    });
    expect(names()).toEqual([
      "Team sync, Thu, Oct 8, 2026 · 10:00 – 10:30 AM · Room 9",
      "Team sync, Wed, Oct 7, 2026 · 10:00 – 10:30 AM · Room 9",
      "Team sync, Tue, Oct 6, 2026 · 10:00 – 10:30 AM · Room 9",
    ]);
    expect(screen.getByText("Earlier")).toBeTruthy();
    expect(screen.getByText("Search Results")).toBeTruthy();
    expect(screen.getByText("3 results")).toBeTruthy();
    expect(screen.queryByRole("group", { name: "Thursday, October 8, 2026" })).toBeNull();
    // Blurring a field that holds a query keeps the search.
    fireEvent.blur(field());
    expect(screen.getByRole("listbox")).toBeTruthy();
  });

  it("finds an event by a person and by its notes, and says when nothing matches", async () => {
    await setup();
    await search("carmen");
    expect(new Set(names().map((name) => name?.split(",")[0]))).toEqual(new Set(["Standup", "Design review"]));
    fireEvent.change(field(), { target: { value: "screenshots" } });
    fireEvent.keyDown(field(), { key: "Enter" });
    await waitFor(() => expect(names()).toEqual(["Design review, Thu, Oct 8, 2026 · 2:00 – 3:30 PM · Studio B"]));
    expect(screen.getByText("1 result")).toBeTruthy();
    fireEvent.change(field(), { target: { value: "zeppelin" } });
    fireEvent.keyDown(field(), { key: "Enter" });
    await screen.findByText("No Results");
    expect(screen.getByText("0 results")).toBeTruthy();
  });

  it("selects a result's occurrence and day, moves with the arrows, and opens the editor on Enter", async () => {
    await setup();
    await search("team sync");
    fireEvent.click(screen.getByRole("option", { name: /Wed, Oct 7/ }));
    await pane().findByRole("heading", { name: "Team sync" });
    expect(useUI.getState().calendarDate).toBe("2026-10-07");
    const list = screen.getByRole("listbox");
    fireEvent.keyDown(list, { key: "ArrowDown" });
    expect(useUI.getState().calendarDate).toBe("2026-10-06");
    await waitFor(() => expect(pane().getByText(/October 6, 2026/)).toBeTruthy());
    fireEvent.keyDown(list, { key: "Enter" });
    await waitFor(() => expect(frameRoute()).toMatch(/\/event\/\d+\?recurrenceId=/));
  });

  it("returns to the view on the selected day with Escape, then clears the selection", async () => {
    await setup();
    await search("team sync");
    fireEvent.click(screen.getByRole("option", { name: /Tue, Oct 6/ }));
    await pane().findByRole("heading", { name: "Team sync" });
    fireEvent.keyDown(window, { key: "Escape" });
    await screen.findByRole("group", { name: "Tuesday, October 6, 2026" });
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(useUI.getState().calendarSelected).not.toBeNull();
    expect(useUI.getState().calendarSearch).toBe("");
    fireEvent.keyDown(window, { key: "Escape" });
    expect(useUI.getState().calendarSelected).toBeNull();
  });

  it("asks again when the calendar changes", async () => {
    const { client, calls } = await setup();
    await search("lunch");
    expect(names()).toHaveLength(1);
    const before = calls("calendar.search").length;
    await act(async () => {
      await client.calendar.delete({ id: 303 });
    });
    await screen.findByText("No Results");
    expect(calls("calendar.search").length).toBeGreaterThan(before);
  });
});
