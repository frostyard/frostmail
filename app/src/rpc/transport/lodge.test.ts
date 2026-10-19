import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { connectLodgeAPI, PING_MS } from "./lodge";

// FakeSocket stands in for the browser's WebSocket.
class FakeSocket {
  static last: FakeSocket | null = null;
  sent: string[] = [];
  onopen: (() => void) | null = null;
  onclose: ((e: { code: number }) => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  constructor(readonly url: string) {
    FakeSocket.last = this;
  }
  send(line: string) {
    this.sent.push(line);
  }
}

function fakeDocument(state: DocumentVisibilityState) {
  const target = new EventTarget();
  const doc = Object.assign(target, { visibilityState: state }) as unknown as Document & {
    visibilityState: DocumentVisibilityState;
  };
  const set = (s: DocumentVisibilityState) => {
    (doc as { visibilityState: DocumentVisibilityState }).visibilityState = s;
    target.dispatchEvent(new Event("visibilitychange"));
  };
  return { doc, set };
}

describe("connectLodgeAPI", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    FakeSocket.last = null;
    vi.stubGlobal("WebSocket", FakeSocket);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("opens only once the page is visible", async () => {
    const { doc, set } = fakeDocument("hidden");
    const pending = connectLodgeAPI("wss://lodge.test/app/mail/api", doc);
    await Promise.resolve();
    expect(FakeSocket.last).toBeNull();
    set("visible");
    await vi.waitFor(() => expect(FakeSocket.last?.url).toBe("wss://lodge.test/app/mail/api"));
    FakeSocket.last?.onopen?.();
    await expect(pending).resolves.toBeDefined();
  });

  it("sends one message per frame and pings while visible", async () => {
    const { doc, set } = fakeDocument("visible");
    const pending = connectLodgeAPI("wss://lodge.test/app/mail/api", doc);
    await vi.waitFor(() => expect(FakeSocket.last).not.toBeNull());
    const ws = FakeSocket.last as FakeSocket;
    ws.onopen?.();
    const t = await pending;
    const call = t.call("mailbox.list", {});
    const req = JSON.parse(ws.sent[0] ?? "{}") as { id: number; method: string };
    expect(req.method).toBe("mailbox.list");
    expect(ws.sent[0]).not.toContain("\n");
    ws.onmessage?.({ data: `${JSON.stringify({ jsonrpc: "2.0", id: req.id, result: [] })}\n` });
    await expect(call).resolves.toEqual([]);

    vi.advanceTimersByTime(PING_MS);
    expect(ws.sent.filter((l) => l.includes('"rpc.ping"'))).toHaveLength(1);
    set("hidden");
    vi.advanceTimersByTime(PING_MS * 3);
    expect(ws.sent.filter((l) => l.includes('"rpc.ping"'))).toHaveLength(1);
    set("visible"); // pings at once on coming back
    expect(ws.sent.filter((l) => l.includes('"rpc.ping"'))).toHaveLength(2);

    const closed = new Promise<string>((resolve) => t.onClose(resolve));
    ws.onclose?.({ code: 1001 });
    await expect(closed).resolves.toContain("1001");
    vi.advanceTimersByTime(PING_MS * 2);
    expect(ws.sent.filter((l) => l.includes('"rpc.ping"'))).toHaveLength(2);
  });

  it("fails when lodge refuses the socket", async () => {
    const { doc } = fakeDocument("visible");
    const pending = connectLodgeAPI("wss://lodge.test/app/mail/api", doc);
    await vi.waitFor(() => expect(FakeSocket.last).not.toBeNull());
    FakeSocket.last?.onclose?.({ code: 1006 });
    await expect(pending).rejects.toThrow("1006");
  });
});
