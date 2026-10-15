// CONTRACT TEST for task card T-0159 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { CalendarEvent } from "../rpc/gen/api";
import { changedFields, draftOf, newDraft, sendsInvitees, validate } from "./eventDraft";

// A meeting the user organizes: themselves, an accepted invitee and an
// optional one who said maybe.
const meeting: CalendarEvent = {
  id: 302,
  recurrenceId: "",
  calendarId: 201,
  accountId: 1,
  uid: "meeting@example.com",
  summary: "Design review",
  location: "",
  description: "",
  allDay: false,
  start: "2026-10-15T13:00:00.000Z",
  end: "2026-10-15T14:00:00.000Z",
  startDate: "",
  endDate: "",
  timeZone: "UTC",
  recurring: false,
  recurrence: "",
  status: "confirmed",
  transparent: false,
  organizer: { email: "me@example.com", name: "Me", role: "chair", answer: "accepted", isUser: true },
  attendees: [
    { email: "me@example.com", name: "Me", role: "required", answer: "accepted", isUser: true },
    { email: "carmen@example.com", name: "Carmen Lindqvist", role: "required", answer: "accepted", isUser: false },
    { email: "ines@example.com", name: "", role: "optional", answer: "tentative", isUser: false },
  ],
  alarms: [],
  readOnly: false,
};

describe("an event draft's invitees", () => {
  it("are the attendees but the user, with their answers", () => {
    expect(draftOf(meeting, "UTC").invitees).toEqual([
      { email: "carmen@example.com", name: "Carmen Lindqvist", role: "required", answer: "accepted" },
      { email: "ines@example.com", name: "", role: "optional", answer: "tentative" },
    ]);
    expect(draftOf({ ...meeting, attendees: [] }, "UTC").invitees).toBeUndefined();
    expect(newDraft(meeting, 201, "UTC").invitees).toBeUndefined();
  });

  it("are sent only when the list changes, by address and role", () => {
    const original = draftOf(meeting, "UTC");
    const invitees = original.invitees ?? [];
    expect(changedFields(original, { ...original, invitees: [...invitees].reverse() })).toEqual({});
    expect(
      changedFields(original, { ...original, invitees: invitees.map((i) => ({ ...i, name: `${i.name} (renamed)` })) }),
    ).toEqual({});
    expect(
      changedFields(original, {
        ...original,
        invitees: [...invitees, { email: "erin@example.com", name: "Erin", role: "required" }],
      }),
    ).toEqual({
      attendees: [
        { email: "carmen@example.com", name: "Carmen Lindqvist", role: "required" },
        { email: "ines@example.com", role: "optional" },
        { email: "erin@example.com", name: "Erin", role: "required" },
      ],
    });
    expect(
      changedFields(original, { ...original, invitees: invitees.map((i) => ({ ...i, role: "required" as const })) }),
    ).toEqual({
      attendees: [
        { email: "carmen@example.com", name: "Carmen Lindqvist", role: "required" },
        { email: "ines@example.com", role: "required" },
      ],
    });
    expect(changedFields(original, { ...original, invitees: [] })).toEqual({ attendees: [] });
  });

  it("go with a new event only when it has some", () => {
    const fresh = newDraft(meeting, 201, "UTC");
    expect(changedFields(null, fresh).attendees).toBeUndefined();
    expect(
      changedFields(null, { ...fresh, invitees: [{ email: "carmen@example.com", name: "", role: "optional" }] })
        .attendees,
    ).toEqual([{ email: "carmen@example.com", role: "optional" }]);
  });

  it("say when a save writes some", () => {
    const original = draftOf(meeting, "UTC");
    expect(sendsInvitees(original, original)).toBe(false);
    expect(sendsInvitees(original, { ...original, summary: "Renamed" })).toBe(false);
    expect(sendsInvitees(original, { ...original, invitees: [] })).toBe(false);
    expect(sendsInvitees(original, { ...original, invitees: (original.invitees ?? []).slice(1) })).toBe(true);
    const fresh = newDraft(meeting, 201, "UTC");
    expect(sendsInvitees(null, fresh)).toBe(false);
    expect(sendsInvitees(null, { ...fresh, invitees: [{ email: "a@example.com", name: "", role: "required" }] })).toBe(
      true,
    );
  });

  it("keep a draft from saving while an address is not one", () => {
    const original = draftOf(meeting, "UTC");
    expect(validate(original)).toEqual({});
    expect(
      validate({
        ...original,
        invitees: [...(original.invitees ?? []), { email: "erin", name: "", role: "required" }],
      }),
    ).toEqual({ invitees: "Check the invitees' addresses" });
    expect(
      validate({
        ...original,
        endTime: "08:00",
        invitees: [{ email: "erin", name: "", role: "required" }],
      }),
    ).toEqual({ end: "The end is before the start", invitees: "Check the invitees' addresses" });
  });
});
