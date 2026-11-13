// Adding a published event from its card (docs/specs/pim-ui.md,
// Invitation card; ADR-0059).
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { Client } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

// The fixture's invitation message, turned into a mail that publishes a
// flight: an organizer, the buyer as an attendee, and an alarm.
async function setup() {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z"), invitation: true });
  const invitation = data.calendar?.invitations?.[0];
  const lunch = data.calendar?.events.find((mock) => mock.event.id === invitation?.eventId)?.event;
  if (!invitation || !lunch) throw new Error("the fixture has no invitation");
  invitation.published = {
    ...lunch,
    uid: "flight@airline.test",
    summary: "Flight LH 400",
    location: "Gate B22",
    start: "2026-10-20T08:30:00.000Z",
    end: "2026-10-20T10:45:00.000Z",
    alarms: [180],
  };
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  const message = data.messages
    .filter((m) => m.summary.subject === "Invitation: Lunch with Ann" && m.summary.mailboxIds.includes(FIXTURE.inbox))
    .at(-1);
  if (!message) throw new Error("the fixture has no invitation message");
  act(() => useUI.getState().select([message.summary.id], message.summary.id));
  return { mock, calls, id: message.summary.id };
}

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

describe("Add to Calendar", () => {
  it("adds the published event once, then says the calendar has it and shows it there", async () => {
    const { mock, calls, id } = await setup();
    const card = await screen.findByRole("region", { name: "Invitation" });
    expect(within(card).getByText("Flight LH 400")).toBeTruthy();
    expect(within(card).getByText("Event")).toBeTruthy();
    expect(within(card).queryByRole("group", { name: "Answer" })).toBeNull();

    const add = within(card).getByRole("button", { name: "Add to Calendar" });
    fireEvent.click(add);
    fireEvent.click(add);
    await within(card).findByText("This event is in your calendar.");
    expect(within(card).queryByRole("button", { name: "Add to Calendar" })).toBeNull();
    expect(calls("calendar.addFromMessage")).toEqual([{ messageId: id }]);

    // The copy is the user's own: nobody to tell, nothing to ring.
    const day = await new Client(mock).calendar.range({ from: "2026-10-20", to: "2026-10-21", timeZone: "UTC" });
    const added = day.find((o) => o.summary === "Flight LH 400");
    if (!added) throw new Error("the calendar has no flight");
    const event = await new Client(mock).calendar.event({ id: added.eventId });
    expect(event.organizer).toBeUndefined();
    expect(event.attendees).toEqual([]);
    expect(event.alarms).toEqual([]);

    fireEvent.click(within(card).getByRole("button", { name: "Show in Calendar" }));
    await waitFor(() => expect(useUI.getState().module).toBe("calendar"));
    expect(useUI.getState().calendarSelected).toEqual({ eventId: added.eventId, recurrenceId: "" });
    expect(useUI.getState().calendarDate).toBe("2026-10-20");
  });
});
