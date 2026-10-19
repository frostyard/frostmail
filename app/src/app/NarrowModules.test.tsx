// Calendar, People and Tasks in a narrow window (docs/specs/ui.md, Narrow
// windows): one pane at a time, an item opened and Back to where it was.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";
import { calendarPane, NARROW, peoplePane, tasksPane } from "./narrow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  vi.stubGlobal("matchMedia", (media: string) => ({
    matches: media === NARROW,
    media,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 3, now: new Date("2026-10-08T12:00:00Z") }));
  const { container } = render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("listbox", { name: "Messages" });
  const pane = () => container.querySelector("[data-narrow-pane]")?.getAttribute("data-narrow-pane");
  const tab = (name: string) =>
    fireEvent.click(within(screen.getByRole("toolbar", { name: "Modules" })).getByRole("button", { name }));
  return { pane, tab };
}

describe("the narrow panes", () => {
  it("pick each module's pane", () => {
    expect(calendarPane(true, { eventId: 1, recurrenceId: "" })).toBe("calendars");
    expect(calendarPane(false, { eventId: 1, recurrenceId: "" })).toBe("event");
    expect(calendarPane(false, null)).toBe("day");
    expect(peoplePane(true, 4, null)).toBe("books");
    expect(peoplePane(true, null, "new")).toBe("person");
    expect(peoplePane(false, 4, null)).toBe("person");
    expect(peoplePane(false, null, null)).toBe("people");
    expect(tasksPane(true, 4)).toBe("taskLists");
    expect(tasksPane(false, 4)).toBe("task");
    expect(tasksPane(false, null)).toBe("tasks");
  });
});

describe("a narrow window's modules", () => {
  it("Tasks: the tasks, a task, Back, and the lists", async () => {
    const { pane, tab } = await setup();
    tab("Tasks");
    await waitFor(() => expect(pane()).toBe("tasks"));
    const list = await screen.findByRole("listbox", { name: "Today" });
    fireEvent.click(await waitFor(() => within(list).getAllByRole("option")[0] as HTMLElement));
    expect(pane()).toBe("task");
    fireEvent.click(screen.getByRole("button", { name: "Back to Today" }));
    expect(pane()).toBe("tasks");
    fireEvent.click(screen.getByRole("button", { name: "Back to Lists" }));
    expect(pane()).toBe("taskLists");
    expect(screen.queryByRole("button", { name: /^Back to/ })).toBeNull();
    fireEvent.click(screen.getAllByRole("treeitem")[0] as HTMLElement);
    expect(pane()).toBe("tasks");
  });

  it("People: the list, a person, Back, and the address books", async () => {
    const { pane, tab } = await setup();
    tab("People");
    await waitFor(() => expect(pane()).toBe("people"));
    const list = await screen.findByRole("listbox", { name: "Contacts" });
    fireEvent.click(await waitFor(() => within(list).getAllByRole("option")[0] as HTMLElement));
    expect(pane()).toBe("person");
    fireEvent.click(screen.getByRole("button", { name: "Back to All Contacts" }));
    expect(pane()).toBe("people");
    fireEvent.click(within(list).getAllByRole("option")[0] as HTMLElement);
    useUI.getState().openBooks(true);
    await waitFor(() => expect(pane()).toBe("books"));
    fireEvent.click(screen.getAllByRole("treeitem")[0] as HTMLElement);
    // A book picked shows its list, not the person picked before.
    expect(pane()).toBe("people");
    expect(useUI.getState().peopleSelected).toBeNull();
  });

  it("Calendar: the day under its week, an event, Back, and the calendars", async () => {
    const { pane, tab } = await setup();
    tab("Calendar");
    await waitFor(() => expect(pane()).toBe("day"));
    const week = screen.getByRole("group", { name: "Week" });
    expect(within(week).getAllByRole("button", { pressed: true })).toHaveLength(1);
    const date = useUI.getState().calendarDate;
    fireEvent.click(within(week).getByRole("button", { name: "Next Week" }));
    expect(useUI.getState().calendarDate).not.toBe(date);
    useUI.getState().selectOccurrence({ eventId: 302, recurrenceId: "" });
    await waitFor(() => expect(pane()).toBe("event"));
    fireEvent.click(screen.getByRole("button", { name: /^Back to / }));
    expect(pane()).toBe("day");
    fireEvent.click(screen.getByRole("button", { name: "Back to Calendars" }));
    expect(pane()).toBe("calendars");
    useUI.getState().selectOccurrence({ eventId: 302, recurrenceId: "" });
    useUI.getState().openCalendars(true);
    const days = await screen.findAllByRole("grid");
    fireEvent.click(within(days[0] as HTMLElement).getAllByRole("button")[10] as HTMLElement);
    // A date picked shows its day, not the event picked before.
    expect(pane()).toBe("day");
  });
});
