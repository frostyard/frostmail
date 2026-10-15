// CONTRACT TEST for task card T-0157 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { Client, ErrorCode, type InviteNotice } from "../gen/api";
import type { RPCError } from "../transport";
import { CALENDARS, mockData } from "./fixture";
import { MockTransport } from "./mock";

// The fixture's day is 2026-10-08 (UTC); the user is Test One
// <test1@mailtest.test>. 302 Design review, which the user organizes in
// Work, today 14:00–15:30, invites Carmen (accepted) and Ines (tentative,
// optional). 303 Lunch with Ann is an invitation to the user.
// The fixture's calendars do not invite, as on an IMAP account; these
// tests turn Work and Home on, as on Google or iCloud.
function client(invites: number[] = [CALENDARS.work, CALENDARS.home]) {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
  for (const col of data.pim?.collections ?? []) col.invites = invites.includes(col.id);
  return new Client(new MockTransport(data));
}

async function refusal(p: Promise<unknown>): Promise<{ code?: number; notice?: unknown }> {
  try {
    await p;
    return {};
  } catch (err) {
    const e = err as RPCError;
    return { code: e.code, notice: e.data };
  }
}

async function asks(p: Promise<unknown>, notice: InviteNotice) {
  expect(await refusal(p)).toEqual({ code: ErrorCode.notifyRequired, notice });
}

const later = {
  allDay: false,
  start: "2026-10-09T15:00:00.000Z",
  end: "2026-10-09T16:00:00.000Z",
  timeZone: "UTC",
};

describe("MockTransport invitees", () => {
  it("says which calendars invite", async () => {
    const invites = async (c: Client) =>
      (await c.account.collections({ kind: "calendar" })).map((col) => [col.id, col.invites]);
    expect(await invites(new Client(new MockTransport(mockData({ inbox: 5 }))))).toEqual([
      [CALENDARS.work, false],
      [CALENDARS.home, false],
      [CALENDARS.holidays, false],
    ]);
    expect(await invites(client())).toEqual([
      [CALENDARS.work, true],
      [CALENDARS.home, true],
      [CALENDARS.holidays, false],
    ]);
  });

  it("asks before inviting, and makes the user the organizer", async () => {
    const c = client();
    const create = (notify?: boolean) =>
      c.calendar.create({
        calendarId: CALENDARS.home,
        ...(notify === undefined ? {} : { notify }),
        fields: {
          summary: "Planning",
          ...later,
          attendees: [
            { email: "Carl@Example.test", name: "Carl Diaz" },
            { email: "dana@example.test", role: "optional" },
            { email: "test1@mailtest.test" },
            { email: "carl@example.test" },
          ],
        },
      });
    await asks(create(), { kind: "invitations", count: 2 });
    expect(
      (await c.calendar.range({ from: "2026-10-09", to: "2026-10-10", timeZone: "UTC" })).map((o) => o.summary),
    ).not.toContain("Planning");
    expect((await refusal(create(false))).code).toBe(ErrorCode.invalidParams);
    const w = await create(true);
    expect(w.event?.organizer).toEqual({
      email: "test1@mailtest.test",
      name: "Test One",
      role: "chair",
      answer: "accepted",
      isUser: true,
    });
    expect(w.event?.attendees).toEqual([
      { email: "carl@example.test", name: "Carl Diaz", role: "required", answer: "needsaction", isUser: false },
      { email: "dana@example.test", name: "", role: "optional", answer: "needsaction", isUser: false },
    ]);
  });

  it("asks when invitees would see the change, and keeps their answers", async () => {
    const c = client();
    const update = (fields: Record<string, unknown>, notify?: boolean) =>
      c.calendar.update({ id: 302, fields, ...(notify === undefined ? {} : { notify }) });

    await update({ alarms: [5] });
    await update({ transparent: true });
    await update({
      attendees: [
        { email: "carmen@vertex.test", name: "Carmen Lindqvist" },
        { email: "ines.kim@vertex.test", role: "optional" },
      ],
    });

    await asks(update({ summary: "Calendar review" }), { kind: "updates", count: 2 });
    await asks(update(later), { kind: "updates", count: 2 });
    await asks(update({ attendees: [{ email: "carmen@vertex.test" }] }), { kind: "cancellations", count: 1 });
    const three = [
      { email: "carmen@vertex.test" },
      { email: "ines.kim@vertex.test", role: "optional" },
      { email: "erin@example.test", name: "Erin" },
    ];
    await asks(update({ attendees: three }), { kind: "invitations", count: 1 });
    await asks(update({ attendees: [{ email: "erin@example.test" }] }), { kind: "updates", count: 3 });

    const w = await update({ summary: "Calendar review", attendees: three }, true);
    expect(w.event?.summary).toBe("Calendar review");
    expect(w.event?.attendees.map((a) => [a.email, a.role, a.answer])).toEqual([
      ["carmen@vertex.test", "required", "accepted"],
      ["ines.kim@vertex.test", "optional", "tentative"],
      ["erin@example.test", "required", "needsaction"],
    ]);
    expect((await refusal(update({ calendarId: CALENDARS.home }, true))).code).toBe(ErrorCode.invalidParams);
  });

  it("asks before cancelling, and again before undoing it", async () => {
    const c = client();
    await asks(c.calendar.delete({ id: 302 }), { kind: "cancellations", count: 2 });
    const { restoreId } = await c.calendar.delete({ id: 302, notify: true });
    expect(
      (await c.calendar.range({ from: "2026-10-08", to: "2026-10-09", timeZone: "UTC" })).map((o) => o.eventId),
    ).not.toContain(302);
    await asks(c.calendar.restore({ restoreId }), { kind: "invitations", count: 2 });
    await c.calendar.restore({ restoreId, notify: true });
    expect((await c.calendar.event({ id: 302 })).attendees).toHaveLength(2);

    const { restoreId: renamed } = await c.calendar.update({ id: 302, fields: { summary: "Renamed" }, notify: true });
    await asks(c.calendar.restore({ restoreId: renamed }), { kind: "updates", count: 2 });
  });

  it("tells nobody about invitations, events without invitees, or calendars that do not invite", async () => {
    const c = client([CALENDARS.work]);
    await c.calendar.update({ id: 303, fields: { alarms: [30] } });
    expect((await refusal(c.calendar.update({ id: 303, fields: { attendees: [] }, notify: true }))).code).toBe(
      ErrorCode.invalidParams,
    );
    const w = await c.calendar.create({ calendarId: CALENDARS.work, fields: { summary: "Focus", ...later } });
    await c.calendar.update({ id: w.event?.id ?? 0, fields: { summary: "Deep focus" } });
    expect(
      (
        await refusal(
          c.calendar.create({
            calendarId: CALENDARS.home,
            notify: true,
            fields: { summary: "Planning", ...later, attendees: [{ email: "carl@example.test" }] },
          }),
        )
      ).code,
    ).toBe(ErrorCode.invalidParams);
    expect(
      (
        await refusal(
          c.calendar.create({
            calendarId: CALENDARS.work,
            notify: true,
            fields: { summary: "Planning", ...later, attendees: [{ email: "not an address" }] },
          }),
        )
      ).code,
    ).toBe(ErrorCode.invalidParams);
  });
});
