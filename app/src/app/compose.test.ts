// Starting a draft and opening its window (docs/specs/compose-ui.md, Entry
// points).
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { type Client, type Draft, type DraftCreateParams, ErrorCode } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";
import { openCompose, startDraft } from "./compose";

beforeEach(() => {
  vi.spyOn(window, "open").mockReturnValue(null);
});

afterEach(() => {
  vi.restoreAllMocks();
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
    expect(window.open).toHaveBeenCalledWith(`${window.location.pathname}#/compose/7?fresh`, "compose-7");
  });

  it("does not retry other errors", async () => {
    const error = new RPCError(ErrorCode.unavailable, "offline");
    const create = vi.fn().mockRejectedValue(error);
    const client = { draft: { create } } as unknown as Client;
    await expect(startDraft(client, "reply", 12, "selected text")).rejects.toBe(error);
    expect(create).toHaveBeenCalledTimes(1);
    expect(window.open).not.toHaveBeenCalled();
  });

  it("does not retry invalidParams when no quote was sent", async () => {
    const error = new RPCError(ErrorCode.invalidParams, "bad source");
    const create = vi.fn().mockRejectedValue(error);
    const client = { draft: { create } } as unknown as Client;
    await expect(startDraft(client, "reply", 12)).rejects.toBe(error);
    expect(create).toHaveBeenCalledTimes(1);
    expect(window.open).not.toHaveBeenCalled();
  });

  it("does not retry a failed fallback", async () => {
    const error = new RPCError(ErrorCode.invalidParams, "bad source");
    const create = vi.fn().mockRejectedValue(error);
    const client = { draft: { create } } as unknown as Client;
    await expect(startDraft(client, "reply", 12, "selected text")).rejects.toBe(error);
    expect(create).toHaveBeenCalledTimes(2);
    expect(window.open).not.toHaveBeenCalled();
  });
});

describe("openCompose", () => {
  it("opens a tab at the draft's route", async () => {
    await openCompose(7);
    expect(window.open).toHaveBeenCalledWith(`${window.location.pathname}#/compose/7`, "compose-7");
  });
});
