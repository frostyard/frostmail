// CONTRACT TEST for task card T-0089 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { dateText, timeRange } from "../../lib/eventText";
import type { Attendee, CalendarEvent, Invitation, Occurrence, Part } from "../../rpc/gen/api";
import { InvitationCard, type InvitationCardProps, invitationPart } from "./InvitationCard";

const norm = (s: string | null | undefined) => (s ?? "").replace(/[   ]/g, " ");

const ann: Attendee = {
  email: "ann.smith@northwind.test",
  name: "Ann Smith",
  role: "chair",
  answer: "accepted",
  isUser: false,
};

const lunch: CalendarEvent = {
  id: 0,
  recurrenceId: "",
  calendarId: 0,
  accountId: 1,
  uid: "lunch@northwind.test",
  summary: "Lunch with Ann",
  location: "Cafe Nord",
  description: "",
  allDay: false,
  start: "2026-10-09T12:00:00Z",
  end: "2026-10-09T13:00:00Z",
  startDate: "",
  endDate: "",
  timeZone: "",
  recurring: false,
  recurrence: "",
  status: "confirmed",
  transparent: false,
  organizer: ann,
  attendees: [{ email: "test1@mailtest.test", name: "", role: "required", answer: "needsaction", isUser: true }],
  answer: "needsaction",
  alarms: [],
  readOnly: false,
};

const occ = (summary: string): Occurrence => ({
  eventId: 1,
  recurrenceId: "",
  calendarId: 1,
  accountId: 1,
  summary,
  location: "",
  allDay: false,
  start: "2026-10-09T09:00:00Z",
  end: "2026-10-09T09:15:00Z",
  startDate: "",
  endDate: "",
  status: "confirmed",
  transparent: false,
  recurring: false,
});

const request: Invitation = {
  method: "request",
  event: lunch,
  eventId: 303,
  from: ann,
  answer: "needsaction",
  canRespond: true,
  outdated: false,
  conflicts: [occ("Design review")],
  adjacent: [occ("Standup"), occ("Weekly sync")],
};

function card(over: Partial<InvitationCardProps> = {}) {
  const props: InvitationCardProps = {
    invitation: request,
    timeZone: "UTC",
    locale: "en-US",
    busy: false,
    onAnswer: vi.fn(),
    onShowInCalendar: vi.fn(),
    ...over,
  };
  render(<InvitationCard {...props} />);
  return props;
}

const region = () => screen.getByRole("region", { name: "Invitation" });
const answer = (name: string) => within(screen.getByRole("group", { name: "Answer" })).getByRole("button", { name });

describe("InvitationCard", () => {
  it("shows the event, its day, place, organizer and neighbors", () => {
    card();
    const r = within(region());
    expect(r.getByText("Lunch with Ann")).toBeTruthy();
    expect(r.getByText("Invitation")).toBeTruthy();
    expect(r.getByText("OCT")).toBeTruthy();
    expect(r.getByText("9")).toBeTruthy();
    const when = `${dateText(lunch, "UTC", "en-US")} · ${timeRange(lunch.start, lunch.end, "UTC", "en-US")}`;
    expect(r.getByText((_, el) => norm(el?.textContent) === norm(when) && el?.children.length === 0)).toBeTruthy();
    expect(r.getByText("Cafe Nord · Ann Smith (organizer)")).toBeTruthy();
    expect(r.getByText("Conflicts with Design review")).toBeTruthy();
    expect(r.getByText("Before: Standup · After: Weekly sync")).toBeTruthy();
  });

  it("answers with Accept, Maybe and Decline", () => {
    const props = card();
    for (const name of ["Accept", "Maybe", "Decline"]) expect(answer(name).getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(answer("Accept"));
    expect(props.onAnswer).toHaveBeenLastCalledWith("accepted");
    fireEvent.click(answer("Maybe"));
    expect(props.onAnswer).toHaveBeenLastCalledWith("tentative");
    fireEvent.click(answer("Decline"));
    expect(props.onAnswer).toHaveBeenLastCalledWith("declined");
  });

  it("shows the current answer pressed, and waits while one is sent", () => {
    card({ invitation: { ...request, answer: "tentative" }, busy: true });
    expect(answer("Maybe").getAttribute("aria-pressed")).toBe("true");
    expect(answer("Maybe").className).toContain("bg-accent");
    expect(answer("Accept").getAttribute("aria-pressed")).toBe("false");
    for (const name of ["Accept", "Maybe", "Decline"]) expect((answer(name) as HTMLButtonElement).disabled).toBe(true);
  });

  it("says why it cannot be answered", () => {
    const { unmount } = render(
      <InvitationCard
        invitation={{ ...request, canRespond: false, outdated: true }}
        timeZone="UTC"
        locale="en-US"
        busy={false}
        onAnswer={vi.fn()}
        onShowInCalendar={vi.fn()}
      />,
    );
    expect(screen.getByText("This invitation is out of date.")).toBeTruthy();
    expect(screen.queryByRole("group", { name: "Answer" })).toBeNull();
    unmount();
    card({ invitation: { ...request, canRespond: false, answer: "declined" } });
    expect(screen.getByText("You declined")).toBeTruthy();
  });

  it("says nothing more for a read-only invitation without an answer", () => {
    const { answer: _, ...unanswered } = request;
    card({ invitation: { ...unanswered, canRespond: false } });
    expect(screen.queryByRole("group", { name: "Answer" })).toBeNull();
    expect(screen.queryByText(/^You /)).toBeNull();
  });

  it("marks a cancellation", () => {
    card({ invitation: { ...request, method: "cancel", canRespond: false, event: { ...lunch, status: "cancelled" } } });
    expect(within(region()).getByText("Cancelled").className).toContain("text-flag-1");
    expect(screen.queryByRole("group", { name: "Answer" })).toBeNull();
  });

  it("shows who answered a reply", () => {
    const { answer: _, ...unanswered } = request;
    card({
      invitation: {
        ...unanswered,
        method: "reply",
        canRespond: false,
        from: { email: "bob@example.com", name: "Bob", role: "required", answer: "declined", isUser: false },
        conflicts: [],
        adjacent: [],
      },
    });
    expect(within(region()).getByText("Reply")).toBeTruthy();
    expect(screen.getByText("Bob declined")).toBeTruthy();
    expect(screen.queryByText(/^Conflicts/)).toBeNull();
    expect(screen.queryByText(/^Before/)).toBeNull();
  });

  it("shows an all-day event's own date and an untitled one", () => {
    card({
      invitation: {
        ...request,
        event: { ...lunch, summary: "", allDay: true, startDate: "2026-10-12", endDate: "2026-10-13", location: "" },
      },
    });
    const r = within(region());
    expect(r.getByText("No Title")).toBeTruthy();
    expect(r.getByText("12")).toBeTruthy();
    expect(r.getByText("Monday, October 12, 2026 · All day")).toBeTruthy();
    expect(r.getByText("Ann Smith (organizer)")).toBeTruthy();
  });

  it("opens the event in Calendar", () => {
    const props = card();
    fireEvent.click(screen.getByRole("button", { name: "Show in Calendar" }));
    expect(props.onShowInCalendar).toHaveBeenCalled();
  });
});

describe("invitationPart", () => {
  const part = (contentType: string, filename = ""): Part => ({
    path: "2",
    contentType,
    filename,
    disposition: "attachment",
    contentId: "",
    size: 900,
  });

  it("finds a calendar part", () => {
    expect(invitationPart([part("text/plain"), part("text/calendar")])?.contentType).toBe("text/calendar");
    expect(invitationPart([part("application/ics", "invite.ics")])?.filename).toBe("invite.ics");
    expect(invitationPart([part("application/octet-stream", "Meeting.ICS")])?.filename).toBe("Meeting.ICS");
    expect(invitationPart([part("application/pdf", "agenda.pdf")])).toBeUndefined();
  });
});
