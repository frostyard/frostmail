// The mock's unsubscribe, Redirect and Forward as Attachment, as maild's.
import { describe, expect, it } from "vitest";

import type { Draft, Message, OutboxItem, Unsubscribe } from "../gen/api";
import { ErrorCode } from "../gen/api";
import { FIXTURE, mockData } from "./fixture";
import { MockTransport } from "./mock";

function setup() {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
  const list = data.messages[0];
  if (!list) throw new Error("no messages");
  list.unsubscribe = {
    methods: ["oneclick", "mail", "web"],
    list: "Weekly",
    host: "list.example",
    address: "leave@list.example",
    url: "https://list.example/u/1",
  };
  return { data, id: list.summary.id, mock: new MockTransport(data) };
}

describe("MockTransport unsubscribe", () => {
  it("offers the methods, fails a one-click when told, and remembers the list", async () => {
    const { mock, id, data } = setup();
    const msg = await mock.call<Message>("message.get", { id });
    expect(msg.listUnsubscribe).toContain("https://list.example/u/1");
    const info = await mock.call<Unsubscribe>("message.unsubscribeInfo", { id });
    expect([info.methods, info.list, info.done]).toEqual([["oneclick", "mail", "web"], "Weekly", false]);
    mock.oneClickFails = true;
    await expect(mock.call("message.unsubscribe", { id, method: "oneclick" })).rejects.toMatchObject({
      code: ErrorCode.unavailable,
    });
    expect(await mock.call("message.unsubscribe", { id, method: "web" })).toEqual({ url: "https://list.example/u/1" });
    expect((await mock.call<Unsubscribe>("message.unsubscribeInfo", { id })).done).toBe(true);
    const other = data.messages[1]?.summary.id ?? 0;
    const none = await mock.call<Unsubscribe>("message.unsubscribeInfo", { id: other });
    expect(none.methods).toEqual([]);
    await expect(mock.call("message.unsubscribe", { id: other, method: "mail" })).rejects.toMatchObject({
      code: ErrorCode.invalidParams,
    });
  });
});

describe("MockTransport Redirect and Forward as Attachment", () => {
  it("queues a redirect and attaches the message whole", async () => {
    const { mock, id, data } = setup();
    const item = await mock.call<OutboxItem>("message.redirect", { id, to: [{ name: "Bob", address: "bob@x.test" }] });
    expect([item.subject, item.to, item.state]).toEqual([
      data.messages[0]?.summary.subject,
      [{ name: "Bob", address: "bob@x.test" }],
      "queued",
    ]);
    await expect(mock.call("message.redirect", { id, to: [] })).rejects.toMatchObject({
      code: ErrorCode.invalidParams,
    });
    const d = await mock.call<Draft>("draft.create", { kind: "attached", sourceId: id, accountId: FIXTURE.accountId });
    expect(d.kind).toBe("forward");
    expect(d.content.subject).toMatch(/^Fwd: /);
    expect(d.attachments.map((a) => [a.contentType, a.filename.endsWith(".eml")])).toEqual([["message/rfc822", true]]);
  });
});
