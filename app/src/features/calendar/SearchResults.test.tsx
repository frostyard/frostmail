import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Occurrence } from "../../rpc/gen/api";
import { SearchResults, type SearchResultsProps } from "./SearchResults";

const at = (day: number, over: Partial<Occurrence> = {}): Occurrence => ({
  eventId: day,
  recurrenceId: "",
  calendarId: 1,
  accountId: 1,
  summary: "Sync",
  location: "",
  allDay: false,
  start: `2026-10-0${day}T11:00:00.000Z`,
  end: `2026-10-0${day}T11:30:00.000Z`,
  startDate: "",
  endDate: "",
  status: "confirmed",
  transparent: false,
  recurring: false,
  ...over,
});

function results(over: Partial<SearchResultsProps> = {}) {
  const props: SearchResultsProps = {
    occurrences: [at(7), at(8, { location: "Room 4" }), at(9, { summary: " " })],
    failed: false,
    colors: new Map([[1, "#3366cc"]]),
    timeZone: "UTC",
    locale: "en-US",
    now: new Date("2026-10-08T10:30:00Z"),
    selected: null,
    onSelect: vi.fn(),
    onOpen: vi.fn(),
    ...over,
  };
  const view = render(<SearchResults {...props} />);
  return { props, ...view };
}

// Intl puts thin and narrow no-break spaces in ranges and before AM/PM.
const norm = (text: string | null) => (text ?? "").replace(/[\u2009\u202f]/g, " ");

const names = () => screen.getAllByRole("option").map((o) => norm(o.getAttribute("aria-label")));

describe("SearchResults", () => {
  it("lists the rows under Upcoming and Earlier, named by both lines", () => {
    results();
    const list = screen.getByRole("listbox", { name: "Search results" });
    expect([...list.children].map((row) => norm(row.getAttribute("aria-label") ?? row.textContent))).toEqual([
      "Upcoming",
      "Sync, Thu, Oct 8, 2026 · 11:00 – 11:30 AM · Room 4",
      "No Title, Fri, Oct 9, 2026 · 11:00 – 11:30 AM",
      "Earlier",
      "Sync, Wed, Oct 7, 2026 · 11:00 – 11:30 AM",
    ]);
  });

  it("selects a row on a click and marks the selected one", () => {
    const { props } = results({ selected: { eventId: 8, recurrenceId: "" } });
    const selected = screen.getAllByRole("option").filter((o) => o.getAttribute("aria-selected") === "true");
    expect(selected.map((o) => norm(o.getAttribute("aria-label")))).toEqual([names()[0]]);
    fireEvent.click(screen.getByRole("option", { name: /^No Title/ }));
    expect(props.onSelect).toHaveBeenLastCalledWith(expect.objectContaining({ eventId: 9 }));
  });

  it("moves with the arrows in the rows' order, past the heading, and stops at the ends", () => {
    const { props } = results({ selected: { eventId: 9, recurrenceId: "" } });
    const list = screen.getByRole("listbox");
    fireEvent.keyDown(list, { key: "ArrowDown" });
    expect(props.onSelect).toHaveBeenLastCalledWith(expect.objectContaining({ eventId: 7 }));
    fireEvent.keyDown(list, { key: "ArrowUp" });
    expect(props.onSelect).toHaveBeenLastCalledWith(expect.objectContaining({ eventId: 8 }));
    expect(props.onSelect).toHaveBeenCalledTimes(2);
  });

  it("stops at the last row, and starts at the first with nothing selected", () => {
    const last = results({ selected: { eventId: 7, recurrenceId: "" } });
    fireEvent.keyDown(screen.getByRole("listbox"), { key: "ArrowDown" });
    expect(last.props.onSelect).not.toHaveBeenCalled();
    last.unmount();
    const none = results();
    fireEvent.keyDown(screen.getByRole("listbox"), { key: "ArrowDown" });
    expect(none.props.onSelect).toHaveBeenLastCalledWith(expect.objectContaining({ eventId: 8 }));
  });

  it("opens the selected row on Enter and on a double click", () => {
    const { props } = results({ selected: { eventId: 8, recurrenceId: "" } });
    fireEvent.keyDown(screen.getByRole("listbox"), { key: "Enter" });
    expect(props.onOpen).toHaveBeenCalledTimes(1);
    fireEvent.doubleClick(screen.getByRole("option", { name: /Room 4/ }));
    expect(props.onOpen).toHaveBeenCalledTimes(2);
  });

  it("does not open on Enter with nothing selected", () => {
    const { props } = results();
    fireEvent.keyDown(screen.getByRole("listbox"), { key: "Enter" });
    expect(props.onOpen).not.toHaveBeenCalled();
  });

  it("says No Results, nothing while it asks, and when the search failed", () => {
    const none = results({ occurrences: [] });
    expect(screen.getByText("No Results")).toBeTruthy();
    expect(screen.queryByRole("listbox")).toBeNull();
    none.unmount();
    const asking = results({ occurrences: null });
    expect(screen.queryByText("No Results")).toBeNull();
    asking.unmount();
    results({ occurrences: null, failed: true });
    expect(screen.getByText("The search failed")).toBeTruthy();
  });
});
