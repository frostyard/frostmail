import { afterEach, describe, expect, it, vi } from "vitest";

import type { Client, DraftCreateParams } from "../rpc/gen/api";
import { type ComposeAction, compose } from "./commands";

// startDraft opens a browser window after the RPC completes.
afterEach(() => vi.restoreAllMocks());

describe("compose with selected reader text", () => {
  it.each(["reply", "replyAll"] as const)("%s answers the selection's message and sends plain text", async (action) => {
    vi.spyOn(window, "open").mockReturnValue(null);
    const create = vi.fn(async (_p: DraftCreateParams) => ({ id: 7 }));
    const client = { draft: { create } } as unknown as Client;
    await compose(client, action, [99], { messageId: 12, text: "selected < & text" });
    expect(create).toHaveBeenCalledWith({
      kind: action === "reply" ? "reply" : "replyall",
      sourceId: 12,
      quote: "selected < & text",
    });
  });

  it.each(["forward", "forwardAttachment", "compose"] as ComposeAction[])(
    "%s ignores the reader selection",
    async (action) => {
      vi.spyOn(window, "open").mockReturnValue(null);
      const create = vi.fn(async (_p: DraftCreateParams) => ({ id: 7 }));
      const client = { draft: { create } } as unknown as Client;
      await compose(client, action, [99], { messageId: 12, text: "selected" });
      expect(create).toHaveBeenCalledWith(
        action === "compose" ? { kind: "new" } : { kind: action === "forward" ? "forward" : "attached", sourceId: 99 },
      );
    },
  );

  it("quotes the whole selected message when no reader selection is supplied", async () => {
    vi.spyOn(window, "open").mockReturnValue(null);
    const create = vi.fn(async (_p: DraftCreateParams) => ({ id: 7 }));
    const client = { draft: { create } } as unknown as Client;
    await compose(client, "reply", [99]);
    expect(create).toHaveBeenCalledWith({ kind: "reply", sourceId: 99 });
  });
});
