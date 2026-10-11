// The mock's sorted views, message source and Save As, and sender photos,
// as maild gives them.
import { describe, expect, it } from "vitest";

import type { MessageSource, MessageSummary, SenderPhoto, ViewInfo } from "../gen/api";
import { ErrorCode } from "../gen/api";
import { mockData } from "./fixture";
import { MockTransport } from "./mock";

function setup() {
  const data = mockData({ inbox: 12, now: new Date("2026-10-08T12:00:00Z") });
  return { data, mock: new MockTransport(data) };
}

async function rows(mock: MockTransport, query: object): Promise<MessageSummary[]> {
  const view = await mock.call<ViewInfo>("view.open", { query });
  return mock.call<MessageSummary[]>("view.range", { id: view.id, start: 0, end: view.count });
}

describe("MockTransport sorting", () => {
  it("orders by a key either way, ties newest first", async () => {
    const { mock } = setup();
    const bySize = await rows(mock, { sort: "size", ascending: true });
    const sizes = bySize.map((s) => s.size);
    expect(sizes).toEqual([...sizes].sort((a, b) => a - b));
    const byFrom = await rows(mock, { sort: "from" });
    const names = byFrom.map((s) => (s.from.name || s.from.address).toLowerCase());
    expect(names).toEqual([...names].sort().reverse());
    const unread = await rows(mock, { sort: "unread" });
    const firstRead = unread.findIndex((s) => s.flags.seen);
    expect(unread.slice(firstRead).every((s) => s.flags.seen)).toBe(true);
    const dates = (await rows(mock, { sort: "date", ascending: true })).map((s) => s.date);
    expect(dates).toEqual([...dates].sort());
  });
});

describe("MockTransport message source and Save As", () => {
  it("gives the source with its headers, and saves to an absolute path", async () => {
    const { data, mock } = setup();
    const id = data.messages[0]?.summary.id ?? 0;
    const src = await mock.call<MessageSource>("message.source", { id });
    expect(src.headers).toContain(`Subject: ${data.messages[0]?.summary.subject}`);
    expect(src.text.startsWith(src.headers)).toBe(true);
    expect(src.truncated).toBe(false);
    await mock.call("message.save", { id, path: "/tmp/x.eml" });
    await expect(mock.call("message.save", { id, path: "x.eml" })).rejects.toMatchObject({
      code: ErrorCode.invalidParams,
    });
  });
});

describe("MockTransport sender photos", () => {
  it("names nobody without photos, and bounds the request", async () => {
    const { mock } = setup();
    expect(await mock.call<SenderPhoto[]>("people.senders", { addresses: ["nobody@x.test"] })).toEqual([]);
    await expect(mock.call("people.senders", { addresses: Array(501).fill("a@x.test") })).rejects.toMatchObject({
      code: ErrorCode.invalidParams,
    });
  });
});
