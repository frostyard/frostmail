// A request to open a message waits for a connected window (issue #149):
// docs/design/desktop.md, One app instance.
import { CodeNotConnected } from "@frostyard/lodge-client/rpc";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { useUI } from "../data/stores";
import type { Client } from "../rpc/gen/api";

const env = vi.hoisted(() => ({
  startup: null as number | null,
  invoke: vi.fn(),
  handler: null as ((e: { payload: number }) => void) | null,
}));

vi.mock("@tauri-apps/api/core", () => ({ invoke: env.invoke, isTauri: () => true }));
vi.mock("@tauri-apps/api/event", () => ({
  listen: async (_name: string, h: (e: { payload: number }) => void) => {
    env.handler = h;
    return () => {};
  },
}));

type Summaries = (a: { ids: number[] }) => Promise<unknown[]>;
const fake = (summaries: Summaries) => ({ message: { summaries: vi.fn(summaries) } }) as unknown as Client;
const found: Summaries = async ({ ids }) => [{ id: ids[0], accountId: 1, mailboxIds: [7] }];
const lost = async () => {
  throw new RPCError(CodeNotConnected, "the connection closed: x");
};
const flush = () => new Promise((r) => setTimeout(r, 0));

// Classes are loaded again after vi.resetModules, so the tests take RPCError from the same load.
let mod: typeof import("./openMessage");
let RPCError: typeof import("../rpc/transport").RPCError;
let useUIRef: typeof useUI;
beforeEach(async () => {
  vi.resetModules();
  env.startup = null;
  env.handler = null;
  env.invoke.mockReset();
  env.invoke.mockImplementation(async () => env.startup);
  mod = await import("./openMessage");
  ({ RPCError } = await import("../rpc/transport"));
  const ui = (await import("../data/stores")).useUI;
  ui.getState().select([], null);
  useUIRef = ui;
});

describe("watchOpenRequests", () => {
  it("opens at once with a connected client", async () => {
    const c = fake(found);
    mod.watchOpenRequests(c);
    await flush();
    env.handler?.({ payload: 5 });
    await flush();
    expect(c.message.summaries).toHaveBeenCalledTimes(1);
    expect(useUIRef.getState().anchor).toBe(5);
  });

  it("keeps an event that arrives with no client for the next one (#149)", async () => {
    mod.watchOpenRequests(fake(found))();
    await flush();
    env.handler?.({ payload: 6 });
    await flush();
    expect(useUIRef.getState().anchor).not.toBe(6);
    const c = fake(found);
    mod.watchOpenRequests(c);
    await flush();
    expect(c.message.summaries).toHaveBeenCalledTimes(1);
    expect(useUIRef.getState().anchor).toBe(6);
  });

  it("keeps a request that fails with a lost connection", async () => {
    const dead = fake(lost);
    mod.watchOpenRequests(dead);
    await flush();
    env.handler?.({ payload: 8 });
    await flush();
    expect(dead.message.summaries).toHaveBeenCalledTimes(1);
    const c = fake(found);
    mod.watchOpenRequests(c);
    await flush();
    expect(useUIRef.getState().anchor).toBe(8);
  });

  it("drops a request that maild refused", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const bad = fake(async () => {
      throw new RPCError(-32602, "bad id");
    });
    mod.watchOpenRequests(bad);
    await flush();
    env.handler?.({ payload: 9 });
    await flush();
    const c = fake(found);
    mod.watchOpenRequests(c);
    await flush();
    expect(c.message.summaries).not.toHaveBeenCalled();
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });

  it("asks for the startup message once, across clients", async () => {
    env.startup = 4;
    const stop = mod.watchOpenRequests(fake(found));
    await flush();
    stop();
    mod.watchOpenRequests(fake(found));
    await flush();
    expect(env.invoke.mock.calls.filter((c) => c[0] === "startup_message")).toHaveLength(1);
    expect(useUIRef.getState().anchor).toBe(4);
  });

  it("lets a newer request replace an older one", async () => {
    mod.watchOpenRequests(fake(found))();
    await flush();
    env.handler?.({ payload: 1 });
    env.handler?.({ payload: 2 });
    const c = fake(found);
    mod.watchOpenRequests(c);
    await flush();
    expect(c.message.summaries).toHaveBeenCalledTimes(1);
    expect(useUIRef.getState().anchor).toBe(2);
  });
});
