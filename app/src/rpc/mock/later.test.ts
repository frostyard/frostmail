// The mock's Send Later and Remind Me (ADR-0025), as maild keeps them.
import { describe, expect, it } from "vitest";

import type { Draft, MessageSummary, OutboxItem } from "../gen/api";
import { ErrorCode } from "../gen/api";
import { FIXTURE, mockData } from "./fixture";
import { MockTransport } from "./mock";

function setup() {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
  return { data, mock: new MockTransport(data, { undoMs: 50 }) };
}

async function draft(mock: MockTransport): Promise<Draft> {
  const d = await mock.call<Draft>("draft.create", { kind: "new", accountId: FIXTURE.accountId });
  return mock.call<Draft>("draft.update", {
    id: d.id,
    content: { ...d.content, to: [{ name: "Bob", address: "bob@x.test" }], subject: "Later" },
  });
}

describe("MockTransport Send Later", () => {
  it("schedules a time past the undo delay and reschedules it", async () => {
    const { mock } = setup();
    const at = new Date(Date.now() + 3_600_000).toISOString();
    const item = await mock.call<OutboxItem>("draft.send", { id: (await draft(mock)).id, sendAt: at });
    expect([item.scheduled, item.state, item.sendAt]).toEqual([true, "queued", at]);

    const later = new Date(Date.now() + 7_200_000).toISOString();
    const moved = await mock.call<OutboxItem>("outbox.reschedule", { id: item.id, sendAt: later });
    expect([moved.scheduled, moved.sendAt]).toEqual([true, later]);
    const now = await mock.call<OutboxItem>("outbox.reschedule", { id: item.id, sendAt: new Date().toISOString() });
    expect(now.scheduled).toBe(false);
    await expect(mock.call("outbox.reschedule", { id: item.id, sendAt: later })).rejects.toMatchObject({
      code: ErrorCode.conflict,
    });
    await expect(mock.call("outbox.reschedule", { id: 999, sendAt: later })).rejects.toMatchObject({
      code: ErrorCode.notFound,
    });
  });

  it("sends a time already past after the undo delay", async () => {
    const { mock } = setup();
    const item = await mock.call<OutboxItem>("draft.send", {
      id: (await draft(mock)).id,
      sendAt: new Date(Date.now() - 60_000).toISOString(),
    });
    expect(item.scheduled).toBe(false);
  });
});

describe("MockTransport Remind Me", () => {
  it("sets, shows and clears reminders, and refuses what maild refuses", async () => {
    const { data, mock } = setup();
    const id = data.messages[0]?.summary.id ?? 0;
    const at = new Date(Date.now() + 3_600_000).toISOString();
    await mock.call("message.remind", { ids: [id], at });
    const [s] = await mock.call<MessageSummary[]>("message.summaries", { ids: [id] });
    expect(s?.remindAt).toBe(at);
    await mock.call("message.remind", { ids: [id] });
    const [cleared] = await mock.call<MessageSummary[]>("message.summaries", { ids: [id] });
    expect(cleared?.remindAt).toBeUndefined();

    await expect(
      mock.call("message.remind", { ids: [id], at: new Date(Date.now() - 1000).toISOString() }),
    ).rejects.toMatchObject({ code: ErrorCode.invalidParams });
    await expect(mock.call("message.remind", { ids: [99999], at })).rejects.toMatchObject({
      code: ErrorCode.notFound,
    });
    await mock.call("message.remind", { ids: [id], at });
    await mock.call("message.delete", { ids: [id] });
    const [deleted] = await mock.call<MessageSummary[]>("message.summaries", { ids: [id] });
    expect(deleted?.remindAt).toBeUndefined();
    await mock.call("account.update", { id: FIXTURE.accountId, readOnly: true });
    await expect(mock.call("message.remind", { ids: [id], at })).rejects.toMatchObject({
      code: ErrorCode.conflict,
    });
  });
});
