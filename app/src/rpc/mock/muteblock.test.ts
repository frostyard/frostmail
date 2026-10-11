// The mock's mute and block, as maild's.
import { describe, expect, it } from "vitest";

import type { BlockedSender, MessageSummary, Settings } from "../gen/api";
import { ErrorCode } from "../gen/api";
import { mockData } from "./fixture";
import { MockTransport } from "./mock";

function setup() {
  const data = mockData({ inbox: 20, now: new Date("2026-10-08T12:00:00Z") });
  return { data, mock: new MockTransport(data) };
}

describe("MockTransport mute", () => {
  it("mutes and unmutes whole conversations", async () => {
    const { data, mock } = setup();
    const threaded = data.messages.find(
      (m) => data.messages.filter((o) => o.summary.threadId === m.summary.threadId).length > 1,
    );
    if (!threaded) throw new Error("the fixture has no conversation");
    const members = data.messages
      .filter((m) => m.summary.threadId === threaded.summary.threadId)
      .map((m) => m.summary.id);
    const events: string[] = [];
    mock.onEvent((e) => events.push(e.event));
    await mock.call("message.mute", { ids: [threaded.summary.id], muted: true });
    const rows = await mock.call<MessageSummary[]>("message.summaries", { ids: members });
    expect(rows.map((r) => r.muted)).toEqual(members.map(() => true));
    expect(events).toContain("message.changed");
    await mock.call("message.mute", { ids: [members[0]], muted: false });
    const after = await mock.call<MessageSummary[]>("message.summaries", { ids: members });
    expect(after.every((r) => r.muted === undefined)).toBe(true);
    await expect(mock.call("message.mute", { ids: [], muted: true })).rejects.toMatchObject({
      code: ErrorCode.invalidParams,
    });
  });
});

describe("MockTransport block", () => {
  it("keeps blocked senders and the block action", async () => {
    const { mock } = setup();
    const events: string[] = [];
    mock.onEvent((e) => events.push(e.event));
    const list = await mock.call<BlockedSender[]>("block.add", { addresses: ["Spam@Y.test", "ann@x.test"] });
    expect(list.map((b) => b.address)).toEqual(["ann@x.test", "spam@y.test"]);
    await mock.call("block.add", { addresses: ["spam@y.test"] });
    await mock.call("block.remove", { addresses: ["ANN@x.test"] });
    expect((await mock.call<BlockedSender[]>("block.list", {})).map((b) => b.address)).toEqual(["spam@y.test"]);
    expect(events.filter((e) => e === "block.changed")).toHaveLength(2);
    await expect(mock.call("block.add", { addresses: [] })).rejects.toMatchObject({ code: ErrorCode.invalidParams });
    expect((await mock.call<Settings>("settings.get", {})).blockAction).toBe("mark");
    expect((await mock.call<Settings>("settings.set", { blockAction: "trash" })).blockAction).toBe("trash");
    await expect(mock.call("settings.set", { blockAction: "delete" })).rejects.toMatchObject({
      code: ErrorCode.invalidParams,
    });
  });
});
