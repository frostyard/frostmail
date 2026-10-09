// CONTRACT TEST for task card T-0071 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { CalendarEvent } from "../../rpc/gen/api";
import { EventPane, type EventPaneProps } from "./EventPane";
import { norm } from "./fixtures";

const standup: CalendarEvent = {
  id: 1,
  recurrenceId: "2026-10-08T13:00:00.000Z",
  calendarId: 1,
  accountId: 1,
  uid: "standup",
  summary: "Standup",
  location: "Room 4",
  description: "Line one\nLine two",
  allDay: false,
  start: "2026-10-08T13:00:00Z",
  end: "2026-10-08T13:15:00Z",
  startDate: "",
  endDate: "",
  timeZone: "Europe/Berlin",
  recurring: true,
  recurrence: "RRULE:FREQ=DAILY;COUNT=7",
  status: "confirmed",
  transparent: false,
  organizer: { email: "maria@example.com", name: "Maria", role: "chair", answer: "accepted", isUser: false },
  attendees: [
    { email: "user@dav.test", name: "", role: "required", answer: "accepted", isUser: true },
    { email: "bob@example.com", name: "Bob", role: "optional", answer: "declined", isUser: false },
    { email: "tina@example.com", name: "Tina", role: "required", answer: "tentative", isUser: false },
    { email: "ned@example.com", name: "", role: "required", answer: "needsaction", isUser: false },
  ],
  answer: "accepted",
  alarms: [10, 0],
  readOnly: false,
};

function pane(over: Partial<EventPaneProps> = {}) {
  const props: EventPaneProps = {
    event: standup,
    calendar: { name: "Work", color: "#3366cc", account: "user@dav.test" },
    timeZone: "America/New_York",
    locale: "en-US",
    onPerson: vi.fn(),
    ...over,
  };
  const view = render(<EventPane {...props} />);
  return { props, view };
}

const value = (label: string) => screen.getByText(label, { selector: "dt" }).nextElementSibling?.textContent ?? "";
const has = (text: string) => screen.getAllByText((t) => norm(t) === text).length > 0;

describe("EventPane", () => {
  it("says when nothing is selected", () => {
    pane({ event: null, calendar: null });
    expect(screen.getByText("No Event Selected")).toBeTruthy();
  });

  it("shows the title, place and time", () => {
    pane();
    expect(screen.getByRole("heading", { name: "Standup" })).toBeTruthy();
    expect(screen.getByText("Room 4")).toBeTruthy();
    expect(has("Thursday, October 8, 2026")).toBe(true);
    expect(has("9:00 – 9:15 AM")).toBe(true);
    expect(has("Europe/Berlin: 3:00 – 3:15 PM")).toBe(true);
    expect(screen.queryByText("Cancelled")).toBeNull();
  });

  it("leaves out the event's zone when it is the app's", () => {
    pane({ event: { ...standup, timeZone: "America/New_York" } });
    expect(screen.queryByText(/America\/New_York/)).toBeNull();
  });

  it("lists repeats, calendar, alerts and the user's answer", () => {
    pane();
    expect(value("Repeats")).toBe("Every day, 7 times");
    expect(value("Calendar")).toContain("Work");
    expect(value("Calendar")).toContain("user@dav.test");
    expect(value("Alerts")).toContain("10 minutes before");
    expect(value("Alerts")).toContain("At time of event");
    expect(value("Your answer")).toBe("Accepted");
  });

  it("leaves out rows without a value", () => {
    pane({ event: { ...standup, recurrence: "", recurring: false, alarms: [], answer: undefined, description: "" } });
    for (const label of ["Repeats", "Alerts", "Your answer"]) {
      expect(screen.queryByText(label, { selector: "dt" })).toBeNull();
    }
    expect(value("Calendar")).toContain("Work");
  });

  it("lists the organizer and invitees with their answers", () => {
    const { props } = pane();
    const person = (li: HTMLElement | undefined) => within(li as HTMLElement).getByRole("button").textContent;
    const organizer = within(screen.getByRole("list", { name: "Organizer" })).getAllByRole("listitem");
    expect(organizer.map(person)).toEqual(["Maria"]);
    expect(within(organizer[0] as HTMLElement).getByLabelText("Accepted")).toBeTruthy();
    const invitees = within(screen.getByRole("list", { name: "Invitees" })).getAllByRole("listitem");
    expect(invitees.map(person)).toEqual(["user@dav.test", "Bob", "Tina", "ned@example.com"]);
    expect(invitees[0]?.textContent).toContain("(you)");
    expect(invitees[1]?.textContent).toContain("(optional)");
    expect(invitees[2]?.textContent).not.toMatch(/\(you\)|\(optional\)/);
    expect(within(invitees[0] as HTMLElement).getByLabelText("Accepted")).toBeTruthy();
    expect(within(invitees[1] as HTMLElement).getByLabelText("Declined")).toBeTruthy();
    expect(within(invitees[2] as HTMLElement).getByLabelText("Maybe")).toBeTruthy();
    expect(within(invitees[3] as HTMLElement).queryByLabelText(/Accepted|Declined|Maybe/)).toBeNull();
    const bob = screen.getByRole("button", { name: "Bob" });
    fireEvent.click(bob);
    expect(props.onPerson).toHaveBeenCalledWith("bob@example.com", "Bob", bob);
  });

  it("has no people lists without people", () => {
    pane({ event: { ...standup, organizer: undefined, attendees: [] } });
    expect(screen.queryByRole("list", { name: "Organizer" })).toBeNull();
    expect(screen.queryByRole("list", { name: "Invitees" })).toBeNull();
  });

  it("keeps the notes' line breaks", () => {
    pane();
    const notes = screen.getByText((_, el) => el?.textContent === "Line one\nLine two" && el.children.length === 0);
    expect(notes.className).toContain("whitespace-pre-wrap");
  });

  it("marks cancelled events and untitled ones", () => {
    pane({ event: { ...standup, status: "cancelled", summary: "" } });
    expect(screen.getByText("Cancelled")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "No Title" })).toBeTruthy();
  });

  it("shows all-day events by their days", () => {
    pane({
      event: { ...standup, allDay: true, startDate: "2026-10-12", endDate: "2026-10-15", timeZone: "", recurrence: "" },
    });
    expect(has("October 12 – 14, 2026")).toBe(true);
    expect(screen.getByText("All day")).toBeTruthy();
  });
});
