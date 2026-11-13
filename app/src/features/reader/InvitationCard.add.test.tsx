// Add to Calendar on a published event's card (docs/specs/pim-ui.md,
// Invitation card; ADR-0059).
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { CalendarEvent, Invitation } from "../../rpc/gen/api";
import { InvitationCard, type InvitationCardProps } from "./InvitationCard";

const flight: CalendarEvent = {
  id: 0,
  recurrenceId: "",
  calendarId: 0,
  accountId: 1,
  uid: "flight@airline.test",
  summary: "Flight LH 400",
  location: "Gate B22",
  description: "",
  allDay: false,
  start: "2026-10-20T08:30:00Z",
  end: "2026-10-20T10:45:00Z",
  startDate: "",
  endDate: "",
  timeZone: "",
  recurring: false,
  recurrence: "",
  status: "confirmed",
  transparent: false,
  attendees: [],
  alarms: [],
  readOnly: false,
};

const published: Invitation = {
  method: "publish",
  event: flight,
  canRespond: false,
  canAdd: true,
  outdated: false,
  conflicts: [],
  adjacent: [],
};

function card(invitation: Invitation, over: Partial<InvitationCardProps> = {}) {
  const props: InvitationCardProps = {
    invitation,
    timeZone: "UTC",
    locale: "en-US",
    busy: false,
    onAnswer: vi.fn(),
    onAdd: vi.fn(),
    onShowInCalendar: vi.fn(),
    ...over,
  };
  render(<InvitationCard {...props} />);
  return props;
}

describe("InvitationCard: Add to Calendar", () => {
  it("offers Add to Calendar on a published event the calendars do not hold, and no answers", () => {
    const props = card(published);
    expect(screen.getByText("Event")).toBeTruthy();
    expect(screen.queryByRole("group", { name: "Answer" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Add to Calendar" }));
    expect(props.onAdd).toHaveBeenCalledTimes(1);
    expect(props.onAnswer).not.toHaveBeenCalled();
  });

  it("disables the button while the event is on its way", () => {
    const props = card(published, { busy: true });
    const button = screen.getByRole("button", { name: "Add to Calendar" }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    fireEvent.click(button);
    expect(props.onAdd).not.toHaveBeenCalled();
  });

  it("says the calendar has it once it is stored", () => {
    card({ ...published, canAdd: false, eventId: 310 });
    expect(screen.queryByRole("button", { name: "Add to Calendar" })).toBeNull();
    expect(screen.getByText("This event is in your calendar.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Show in Calendar" })).toBeTruthy();
  });

  it("offers nothing when no calendar can take it", () => {
    card({ ...published, canAdd: false });
    expect(screen.queryByRole("button", { name: "Add to Calendar" })).toBeNull();
    expect(screen.queryByText("This event is in your calendar.")).toBeNull();
  });

  it("does not say so for an invitation that is stored", () => {
    card({ ...published, method: "request", canAdd: false, eventId: 310 });
    expect(screen.queryByText("This event is in your calendar.")).toBeNull();
  });
});
