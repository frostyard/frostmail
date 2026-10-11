// The mock's mailbox operations and favorites, as maild keeps them
// (docs/design/organize.md, Mailboxes).
import { describe, expect, it } from "vitest";

import type { Mailbox, MessageSummary, Settings } from "../gen/api";
import { ErrorCode } from "../gen/api";
import { FIXTURE, mockData } from "./fixture";
import { MockTransport } from "./mock";

function setup() {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
  return { data, mock: new MockTransport(data) };
}

const paths = async (mock: MockTransport) =>
  (await mock.call<Mailbox[]>("mailbox.list", {})).map((mb) => mb.path).sort();

describe("MockTransport mailbox operations", () => {
  it("creates, renames, moves and deletes mailboxes with those inside them", async () => {
    const { data, mock } = setup();
    const top = await mock.call<Mailbox>("mailbox.create", { accountId: FIXTURE.accountId, name: " Clients " });
    expect([top.path, top.name, top.role]).toEqual(["Clients", "Clients", "none"]);
    const inner = await mock.call<Mailbox>("mailbox.create", {
      accountId: FIXTURE.accountId,
      name: "Acme",
      parentId: top.id,
    });
    expect(inner.path).toBe("Clients/Acme");
    const id = data.messages[0]?.summary.id ?? 0;
    await mock.call("message.move", { ids: [id], mailboxId: inner.id });

    await mock.call("mailbox.rename", { id: top.id, name: "Customers" });
    expect(await paths(mock)).toContain("Customers/Acme");
    const moved = await mock.call<Mailbox>("mailbox.move", { id: inner.id, parentId: FIXTURE.projects });
    expect(moved.path).toBe("Projects/Acme");
    await mock.call("mailbox.delete", { id: inner.id });
    expect(await paths(mock)).not.toContain("Projects/Acme");
    expect(await mock.call<MessageSummary[]>("message.summaries", { ids: [id] })).toEqual([]);

    for (const [method, params, code] of [
      ["mailbox.create", { accountId: FIXTURE.accountId, name: " " }, ErrorCode.invalidParams],
      ["mailbox.create", { accountId: FIXTURE.accountId, name: "a/b" }, ErrorCode.invalidParams],
      ["mailbox.create", { accountId: FIXTURE.accountId, name: "Inbox" }, ErrorCode.conflict],
      ["mailbox.create", { accountId: FIXTURE.accountId, name: "Receipts" }, ErrorCode.conflict],
      ["mailbox.rename", { id: FIXTURE.trash, name: "Bin" }, ErrorCode.conflict],
      ["mailbox.delete", { id: FIXTURE.inbox }, ErrorCode.conflict],
      ["mailbox.move", { id: top.id, parentId: top.id }, ErrorCode.conflict],
      ["mailbox.rename", { id: 999, name: "X" }, ErrorCode.notFound],
    ] as const) {
      await expect(mock.call(method, params)).rejects.toMatchObject({ code });
    }
  });

  it("uses a mailbox for a role, and erases a trash mailbox", async () => {
    const { data, mock } = setup();
    const receipts = await mock.call<Mailbox>("mailbox.setRole", { id: FIXTURE.receipts, role: "trash" });
    expect(receipts.role).toBe("trash");
    const list = await mock.call<Mailbox[]>("mailbox.list", {});
    expect(list.find((mb) => mb.id === FIXTURE.trash)?.role).toBe("none");
    await expect(mock.call("mailbox.setRole", { id: FIXTURE.archive, role: "inbox" })).rejects.toMatchObject({
      code: ErrorCode.invalidParams,
    });

    const ids = data.messages.slice(0, 2).map((m) => m.summary.id);
    await mock.call("message.move", { ids, mailboxId: FIXTURE.receipts });
    const before = (await mock.call<Mailbox[]>("mailbox.list", {})).find((mb) => mb.id === FIXTURE.receipts)?.total;
    expect(before).toBeGreaterThanOrEqual(2);
    expect(await mock.call<number>("mailbox.erase", { id: FIXTURE.receipts })).toBe(before);
    expect((await mock.call<Mailbox[]>("mailbox.list", {})).find((mb) => mb.id === FIXTURE.receipts)?.total).toBe(0);
    expect(await mock.call<MessageSummary[]>("message.summaries", { ids })).toEqual([]);
    await expect(mock.call("mailbox.erase", { id: FIXTURE.inbox })).rejects.toMatchObject({
      code: ErrorCode.invalidParams,
    });
  });

  it("keeps favorites in order and forgets deleted mailboxes", async () => {
    const { mock } = setup();
    expect((await mock.call<Settings>("settings.get", {})).favorites).toBeUndefined();
    const extra = await mock.call<Mailbox>("mailbox.create", { accountId: FIXTURE.accountId, name: "Extra" });
    const s = await mock.call<Settings>("settings.set", { favorites: [extra.id, FIXTURE.receipts] });
    expect(s.favorites).toEqual([extra.id, FIXTURE.receipts]);
    await mock.call("mailbox.delete", { id: extra.id });
    expect((await mock.call<Settings>("settings.get", {})).favorites).toEqual([FIXTURE.receipts]);
    for (const favorites of [[FIXTURE.receipts, FIXTURE.receipts], [999]]) {
      await expect(mock.call("settings.set", { favorites })).rejects.toBeTruthy();
    }
  });

  it("refuses a read-only account's mailboxes", async () => {
    const { mock } = setup();
    await mock.call("account.update", { id: FIXTURE.accountId, readOnly: true });
    await expect(mock.call("mailbox.create", { accountId: FIXTURE.accountId, name: "X" })).rejects.toMatchObject({
      code: ErrorCode.conflict,
    });
  });
});
