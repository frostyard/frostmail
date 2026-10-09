// CONTRACT TEST for task card T-0089 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { Client, ErrorCode, type Event } from "../gen/api";
import { RPCError } from "../transport";
import { FIXTURE, mockData } from "./fixture";
import { MockTransport } from "./mock";

const NOW = new Date("2026-10-08T12:00:00Z");

async function setup(invitation = true) {
  const mock = new MockTransport(mockData({ inbox: 5, now: NOW, invitation }));
  const events: Event[] = [];
  mock.onEvent((e) => events.push(e));
  const c = new Client(mock);
  const info = await c.view.open({ query: { role: "inbox" } });
  const rows = await c.view.range({ id: info.id, start: 0, end: info.count });
  const message = rows.find((r) => r.subject === "Invitation: Lunch with Ann");
  return { c, events, message };
}

async function code(p: Promise<unknown>): Promise<number | undefined> {
  try {
    await p;
    return undefined;
  } catch (err) {
    return err instanceof RPCError ? err.code : -1;
  }
}

describe("MockTransport invitations", () => {
  it("has an invitation message only when asked", async () => {
    expect((await setup(false)).message).toBeUndefined();
    const { c, message } = await setup();
    expect(message?.from.name).toBe("Ann Smith");
    expect(message?.flags.seen).toBe(false);
    const full = await c.message.get({ id: message?.id ?? -1 });
    expect(full.parts.map((p) => [p.contentType, p.filename])).toContainEqual(["text/calendar", "invite.ics"]);
  });

  it("answers calendar.invitation for it", async () => {
    const { c, message } = await setup();
    const inv = await c.calendar.invitation({ messageId: message?.id ?? -1 });
    expect([inv.method, inv.eventId, inv.answer, inv.canRespond, inv.outdated]).toEqual([
      "request",
      303,
      "needsaction",
      true,
      false,
    ]);
    expect([inv.event.id, inv.event.summary, inv.event.location, inv.from?.name]).toEqual([
      0,
      "Lunch with Ann",
      "Cafe Nord",
      "Ann Smith",
    ]);
    expect(inv.conflicts).toEqual([]);
    expect(inv.adjacent.map((o) => o.summary)).toEqual(["Standup"]);
    expect(await code(c.calendar.invitation({ messageId: FIXTURE.inbox + 999 }))).toBe(ErrorCode.notFound);
  });

  it("answers from the message or the event, and announces it", async () => {
    const { c, events, message } = await setup();
    const id = message?.id ?? -1;
    const accepted = await c.calendar.respond({ messageId: id, answer: "accepted" });
    expect([accepted.id, accepted.answer]).toEqual([303, "accepted"]);
    expect(events.at(-1)).toEqual({ event: "calendar.changed", data: { accountId: FIXTURE.accountId } });
    expect((await c.calendar.invitation({ messageId: id })).answer).toBe("accepted");
    const day = await c.calendar.range({ from: "2026-10-09", to: "2026-10-10", timeZone: "UTC" });
    expect(day.find((o) => o.summary === "Lunch with Ann")?.answer).toBe("accepted");
    const declined = await c.calendar.respond({ eventId: 303, answer: "declined" });
    expect(declined.answer).toBe("declined");
    expect(await code(c.calendar.respond({ eventId: 303, answer: "needsaction" }))).toBe(ErrorCode.invalidParams);
    expect(await code(c.calendar.respond({ answer: "accepted" }))).toBe(ErrorCode.invalidParams);
    expect(await code(c.calendar.respond({ eventId: 99999, answer: "accepted" }))).toBe(ErrorCode.notFound);
  });
});
