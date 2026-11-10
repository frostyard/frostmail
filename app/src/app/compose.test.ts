// A compose window learns which maild owns its draft, so it never opens that
// id on another one (docs/specs/settings-ui.md, Server pane).
import { beforeEach, describe, expect, it, vi } from "vitest";

import { type Client, type Draft, type DraftCreateParams, ErrorCode } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";

const env = vi.hoisted(() => ({
  invoke: vi.fn(async () => null),
  maild: "local" as string | undefined,
  tauri: true,
}));

vi.mock("@tauri-apps/api/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/core")>()),
  invoke: env.invoke,
  isTauri: () => env.tauri,
}));
vi.mock("../rpc/transport/tauri", () => ({ maildKey: () => env.maild }));

const { openCompose, startDraft } = await import("./compose");

beforeEach(() => {
  env.tauri = true;
  env.maild = "local";
  env.invoke.mockClear();
});

describe("startDraft", () => {
  it("retries once without quote when an older maild rejects it", async () => {
    const create = vi
      .fn<(params: DraftCreateParams) => Promise<Draft>>()
      .mockRejectedValueOnce(new RPCError(ErrorCode.invalidParams, "unknown member quote"))
      .mockResolvedValueOnce({ id: 7 } as Draft);
    const client = { draft: { create } } as unknown as Client;
    await startDraft(client, "reply", 12, "selected text");
    expect(create.mock.calls).toEqual([
      [{ kind: "reply", sourceId: 12, quote: "selected text" }],
      [{ kind: "reply", sourceId: 12 }],
    ]);
    expect(env.invoke).toHaveBeenCalledWith("open_compose", { draftId: 7, fresh: true, maild: "local" });
  });

  it("does not retry other errors", async () => {
    const error = new RPCError(ErrorCode.unavailable, "offline");
    const create = vi.fn().mockRejectedValue(error);
    const client = { draft: { create } } as unknown as Client;
    await expect(startDraft(client, "reply", 12, "selected text")).rejects.toBe(error);
    expect(create).toHaveBeenCalledTimes(1);
    expect(env.invoke).not.toHaveBeenCalled();
  });

  it("does not retry invalidParams when no quote was sent", async () => {
    const error = new RPCError(ErrorCode.invalidParams, "bad source");
    const create = vi.fn().mockRejectedValue(error);
    const client = { draft: { create } } as unknown as Client;
    await expect(startDraft(client, "reply", 12)).rejects.toBe(error);
    expect(create).toHaveBeenCalledTimes(1);
    expect(env.invoke).not.toHaveBeenCalled();
  });

  it("does not retry a failed fallback", async () => {
    const error = new RPCError(ErrorCode.invalidParams, "bad source");
    const create = vi.fn().mockRejectedValue(error);
    const client = { draft: { create } } as unknown as Client;
    await expect(startDraft(client, "reply", 12, "selected text")).rejects.toBe(error);
    expect(create).toHaveBeenCalledTimes(2);
    expect(env.invoke).not.toHaveBeenCalled();
  });
});

describe("openCompose", () => {
  it("passes the key of the maild that owns the draft", async () => {
    await openCompose(7, true);
    expect(env.invoke).toHaveBeenCalledWith("open_compose", { draftId: 7, fresh: true, maild: "local" });
  });

  it("passes no key before any connection", async () => {
    env.maild = undefined;
    await openCompose(7);
    expect(env.invoke).toHaveBeenCalledWith("open_compose", { draftId: 7, fresh: false, maild: null });
  });
});
