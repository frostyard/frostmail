// Opening drafts in compose windows (docs/specs/compose-ui.md, Entry
// points). Each is a route the frame shows.
import { openFrameWindow } from "../lodge/frameWindow";
import type { Client, DraftCreateParams, DraftKind } from "../rpc/gen/api";

/** openCompose shows a draft in its compose window, focusing it when it is already open. */
export async function openCompose(draftId: number, fresh = false): Promise<void> {
  openFrameWindow({ kind: "compose", draftId, fresh });
}

/** startDraft creates a draft (a new message, or a reply or forward of sourceId) and opens it. */
export async function startDraft(client: Client, kind: DraftKind, sourceId?: number): Promise<void> {
  const params: DraftCreateParams = sourceId === undefined ? { kind } : { kind, sourceId };
  const d = await client.draft.create(params);
  await openCompose(d.id, true);
}

/** composeTo opens a new message to an address (the People module, contact cards). */
export async function composeTo(client: Client, address: string, name = ""): Promise<void> {
  const d = await client.draft.create({ kind: "new", to: [{ name, address }] });
  await openCompose(d.id, true);
}

/** openDraftMessage opens a message from a Drafts mailbox as a draft. */
export async function openDraftMessage(client: Client, messageId: number): Promise<void> {
  const d = await client.draft.open({ messageId });
  await openCompose(d.id);
}
