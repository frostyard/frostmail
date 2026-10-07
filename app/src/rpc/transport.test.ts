import { describe, expect, it } from "vitest";

import { Client, ErrorCode, PROTOCOL } from "./gen/api";
import { JsonRpcSession, RPCError } from "./transport";

function fakeServer() {
  const sent: Record<string, unknown>[] = [];
  const session = new JsonRpcSession(async (line) => {
    sent.push(JSON.parse(line) as Record<string, unknown>);
  });
  return { session, sent };
}

describe("JsonRpcSession", () => {
  it("matches responses to calls by id, in any order", async () => {
    const { session, sent } = fakeServer();
    const client = new Client(session);
    const hello = client.rpc.hello({ protocol: PROTOCOL, client: "test" });
    const list = client.mailbox.list();
    await Promise.resolve();
    expect(sent.map((m) => m.method)).toEqual(["rpc.hello", "mailbox.list"]);
    expect(sent[1]?.params).toEqual({});
    session.receive(JSON.stringify({ jsonrpc: "2.0", id: sent[1]?.id, result: [] }));
    session.receive(JSON.stringify({ jsonrpc: "2.0", id: sent[0]?.id, result: { protocol: 1, server: "maild" } }));
    await expect(list).resolves.toEqual([]);
    await expect(hello).resolves.toEqual({ protocol: 1, server: "maild" });
  });

  it("rejects with RPCError carrying the code", async () => {
    const { session, sent } = fakeServer();
    const get = new Client(session).account.get({ id: 9 });
    await Promise.resolve();
    session.receive(
      JSON.stringify({
        jsonrpc: "2.0",
        id: sent[0]?.id,
        error: { code: ErrorCode.notFound, message: "account 9 does not exist" },
      }),
    );
    await expect(get).rejects.toEqual(new RPCError(ErrorCode.notFound, "account 9 does not exist"));
  });

  it("delivers events and fails pending calls on close", async () => {
    const { session } = fakeServer();
    const events: unknown[] = [];
    session.onEvent((e) => events.push(e));
    session.receive(
      JSON.stringify({
        jsonrpc: "2.0",
        method: "event",
        params: { seq: 3, event: "account.changed", data: { id: 1, deleted: false } },
      }),
    );
    expect(events).toEqual([{ seq: 3, event: "account.changed", data: { id: 1, deleted: false } }]);
    const pending = new Client(session).account.list();
    await Promise.resolve();
    let reason = "";
    session.onClose((r) => (reason = r));
    session.close("maild closed the connection");
    await expect(pending).rejects.toThrow("maild closed the connection");
    expect(reason).toBe("maild closed the connection");
  });
});
