// compose's entry points from the main window (docs/specs/compose-ui.md,
// Entry points): each answers the last selected message, and the draft
// quotes the whole message.
import { describe, expect, it } from "vitest";

import type { Client, DraftCreateParams } from "../rpc/gen/api";
import { compose } from "./commands";

function clientOf(): { client: Client; created: DraftCreateParams[] } {
  const created: DraftCreateParams[] = [];
  const create = async (params: DraftCreateParams) => {
    created.push(params);
    return { id: 7 };
  };
  return { client: { draft: { create } } as unknown as Client, created };
}

describe("compose", () => {
  it.each(["reply", "replyAll"] as const)("%s answers the last selected message", async (action) => {
    const { client, created } = clientOf();
    await compose(client, action, [99, 12]);
    expect(created).toEqual([{ kind: action === "reply" ? "reply" : "replyall", sourceId: 12 }]);
  });

  it.each(["forward", "forwardAttachment"] as const)("%s carries the message", async (action) => {
    const { client, created } = clientOf();
    await compose(client, action, [99]);
    expect(created).toEqual([{ kind: action === "forward" ? "forward" : "attached", sourceId: 99 }]);
  });

  it("starts a new message for compose, and nothing for a reply without a selection", async () => {
    const { client, created } = clientOf();
    await compose(client, "compose", [99]);
    await compose(client, "reply", []);
    expect(created).toEqual([{ kind: "new" }]);
  });
});
