// Starting a draft and opening its window (docs/specs/compose-ui.md, Entry
// points): the draft shows in the frame, in place of the main window.
import { describe, expect, it } from "vitest";

import { useFrameWindow } from "../lodge/frameWindow";
import { type Client, type Draft, type DraftCreateParams, ErrorCode } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";
import { openCompose, startDraft } from "./compose";

describe("startDraft", () => {
  it("creates the draft, which quotes the whole message, and shows it in the frame", async () => {
    const created: DraftCreateParams[] = [];
    const create = async (params: DraftCreateParams) => {
      created.push(params);
      return { id: 7 } as Draft;
    };
    const client = { draft: { create } } as unknown as Client;
    await startDraft(client, "reply", 12);
    expect(created).toEqual([{ kind: "reply", sourceId: 12 }]);
    expect(useFrameWindow.getState().open).toEqual({ kind: "compose", draftId: 7, fresh: true });
  });

  it("fails as maild does, and opens nothing", async () => {
    const error = new RPCError(ErrorCode.unavailable, "offline");
    const create = async () => Promise.reject(error);
    const client = { draft: { create } } as unknown as Client;
    await expect(startDraft(client, "reply", 12)).rejects.toBe(error);
    expect(useFrameWindow.getState().open).toBeNull();
  });
});

describe("openCompose", () => {
  it("shows the draft's window", async () => {
    await openCompose(7);
    expect(useFrameWindow.getState().open).toEqual({ kind: "compose", draftId: 7, fresh: false });
  });
});
