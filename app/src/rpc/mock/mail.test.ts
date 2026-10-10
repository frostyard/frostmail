import { describe, expect, it } from "vitest";

import type { MessageSummary, ViewInfo } from "../gen/api";
import { FIXTURE, mockData } from "./fixture";
import { MockTransport } from "./mock";

function setup() {
  const data = mockData({ inbox: 20, now: new Date("2026-10-08T12:00:00Z") });
  return { data, mock: new MockTransport(data) };
}

async function rows(mock: MockTransport, view: ViewInfo): Promise<MessageSummary[]> {
  return mock.call<MessageSummary[]>("view.range", { id: view.id, start: 0, end: view.count + 10 });
}

describe("MockTransport mail", () => {
  it("filters by attachments", async () => {
    const { data, mock } = setup();
    const view = await mock.call<ViewInfo>("view.open", { query: { mailboxId: FIXTURE.inbox, hasAttachments: true } });
    const want = data.messages.filter(
      (m) => m.summary.mailboxIds.includes(FIXTURE.inbox) && m.summary.hasAttachments,
    ).length;
    expect(want).toBeGreaterThan(0);
    expect(view.count).toBe(want);
    for (const r of await rows(mock, view)) expect(r.hasAttachments).toBe(true);
  });

  it("keeps rows read while an unread view is open", async () => {
    const { mock } = setup();
    const view = await mock.call<ViewInfo>("view.open", { query: { mailboxId: FIXTURE.inbox, unread: true } });
    const [first] = await rows(mock, view);
    if (!first) throw new Error("no unread message");
    await mock.call("message.setFlags", { ids: [first.id], changes: { seen: true } });
    expect((await rows(mock, view)).map((r) => r.id)).toContain(first.id);
    const reopened = await mock.call<ViewInfo>("view.open", { query: { mailboxId: FIXTURE.inbox, unread: true } });
    expect(reopened.count).toBe(view.count - 1);
  });

  it("copies messages into another mailbox as new ones", async () => {
    const { mock } = setup();
    const inbox = await mock.call<ViewInfo>("view.open", { query: { mailboxId: FIXTURE.inbox } });
    const [first] = await rows(mock, inbox);
    if (!first) throw new Error("no message");
    await mock.call("message.copy", { ids: [first.id], mailboxId: FIXTURE.receipts });
    const receipts = await mock.call<ViewInfo>("view.open", { query: { mailboxId: FIXTURE.receipts } });
    const copy = (await rows(mock, receipts)).find((r) => r.subject === first.subject && r.id !== first.id);
    expect(copy?.mailboxIds).toEqual([FIXTURE.receipts]);
    expect(mock.message(first.id)?.summary.mailboxIds).toEqual([FIXTURE.inbox]);
    await expect(mock.call("message.copy", { ids: [first.id], mailboxId: 999 })).rejects.toThrow();
  });
});
