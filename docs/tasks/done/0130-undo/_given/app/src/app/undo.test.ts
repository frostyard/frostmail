// CONTRACT TEST for task card T-0130 (docs/tasks). Do not edit.
import { beforeEach, describe, expect, it } from "vitest";

import { Client, type MessageSummary } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { flagsBack, moveBack, UNDO_LIMIT, undo, undoableFlags, undoableMove, useUndo } from "./undo";

beforeEach(() => {
  useUndo.setState({ entries: [] });
});

function summary(id: number, mailboxIds: number[], flags: Partial<MessageSummary["flags"]> = {}): MessageSummary {
  return {
    id,
    accountId: 1,
    mailboxIds,
    threadId: id,
    subject: `m${id}`,
    from: { name: "", address: "a@x.test" },
    date: "2026-10-07T09:00:00Z",
    preview: "",
    flags: { seen: false, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0, ...flags },
    hasAttachments: false,
    size: 1,
    threadCount: 1,
  };
}

describe("moveBack", () => {
  it("moves each message back from where it went, together when they went together", () => {
    const before = [summary(1, [10]), summary(2, [10]), summary(3, [11])];
    const after = [summary(1, [20]), summary(2, [20]), summary(3, [20])];
    expect(moveBack(before, after)).toEqual([
      { method: "move", params: { ids: [1, 2], mailboxId: 10, fromMailboxId: 20 } },
      { method: "move", params: { ids: [3], mailboxId: 11, fromMailboxId: 20 } },
    ]);
  });

  it("copies back labels a message left without entering one, and the rest it left", () => {
    // A Gmail archive leaves the inbox label; a delete leaves every label for the Trash.
    const before = [summary(1, [1, 5]), summary(2, [1, 5, 6])];
    const after = [summary(1, [5]), summary(2, [9])];
    expect(moveBack(before, after)).toEqual([
      { method: "move", params: { ids: [2], mailboxId: 1, fromMailboxId: 9 } },
      { method: "copy", params: { ids: [1], mailboxId: 1 } },
      { method: "copy", params: { ids: [2], mailboxId: 5 } },
      { method: "copy", params: { ids: [2], mailboxId: 6 } },
    ]);
  });

  it("leaves out messages that are gone or did not move", () => {
    expect(moveBack([summary(1, [10]), summary(2, [10])], [summary(2, [10])])).toEqual([]);
  });
});

describe("flagsBack", () => {
  it("restores each message's earlier flag color or read mark", () => {
    const before = [
      summary(1, [1], { flagged: true, flagColor: 3 }),
      summary(2, [1]),
      summary(3, [1], { flagged: true, flagColor: 1 }),
    ];
    expect(flagsBack(before, { flagColor: 1 })).toEqual([
      { method: "setFlags", params: { ids: [1], changes: { flagColor: 3 } } },
      { method: "setFlags", params: { ids: [2], changes: { flagColor: 0 } } },
    ]);
    const read = [summary(1, [1], { seen: true }), summary(2, [1]), summary(3, [1])];
    expect(flagsBack(read, { seen: true })).toEqual([
      { method: "setFlags", params: { ids: [2, 3], changes: { seen: false } } },
    ]);
  });
});

describe("undo", () => {
  it("takes back the newest action first, and nothing when there is none", async () => {
    const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
    const mock = new MockTransport(data);
    const client = new Client(mock);
    const id = data.messages.find((m) => m.summary.mailboxIds.includes(FIXTURE.inbox))?.summary.id ?? 0;
    const seen = mock.message(id)?.summary.flags.seen ?? false;
    await undoableMove(client, [id], () => client.message.move({ ids: [id], mailboxId: FIXTURE.receipts }));
    await undoableFlags(client, [id], { seen: !seen });
    expect(useUndo.getState().entries).toHaveLength(2);
    expect(await undo(client)).toBe(true);
    expect(mock.message(id)?.summary.flags.seen).toBe(seen);
    expect(mock.message(id)?.summary.mailboxIds).toEqual([FIXTURE.receipts]);
    expect(await undo(client)).toBe(true);
    expect(mock.message(id)?.summary.mailboxIds).toEqual([FIXTURE.inbox]);
    expect(await undo(client)).toBe(false);
  });

  it("remembers at most the last UNDO_LIMIT actions, and not ones that changed nothing", async () => {
    const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
    const client = new Client(new MockTransport(data));
    const id = data.messages[0]?.summary.id ?? 0;
    expect(UNDO_LIMIT).toBe(20);
    for (let i = 0; i < 25; i++) await undoableFlags(client, [id], { flagColor: (i % 7) + 1 });
    expect(useUndo.getState().entries).toHaveLength(20);
    await undoableFlags(client, [id], { flagColor: 4 }); // 25th set color 4 already
    expect(useUndo.getState().entries).toHaveLength(20);
    expect(useUndo.getState().entries.at(-1)).toEqual([
      { method: "setFlags", params: { ids: [id], changes: { flagColor: 3 } } },
    ]);
  });
});
