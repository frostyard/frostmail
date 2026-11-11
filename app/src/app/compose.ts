// Opening drafts in compose windows (docs/specs/compose-ui.md, Entry
// points). In lodge each is a route the frame shows; in a browser on the dev
// gateway it opens in a new tab at #/compose/<id>.
import { openFrameWindow } from "../lodge/frameWindow";
import { type Client, type DraftCreateParams, type DraftKind, ErrorCode } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";

/** ComposeTarget is the draft a compose window shows. */
export interface ComposeTarget {
  draftId: number;
  /** The draft was created to open this window, so closing it untouched discards it. */
  fresh: boolean;
}

/** composeTarget returns the draft this window composes, or null in the main window. */
export function composeTarget(): ComposeTarget | null {
  const m = /^#\/compose\/(\d+)(\?fresh)?$/.exec(window.location.hash);
  return m?.[1] ? { draftId: Number(m[1]), fresh: m[2] !== undefined } : null;
}

/** openCompose shows a draft in its compose window, focusing it when it is already open. */
export async function openCompose(draftId: number, fresh = false): Promise<void> {
  if (openFrameWindow({ kind: "compose", draftId, fresh })) return;
  window.open(`${window.location.pathname}#/compose/${draftId}${fresh ? "?fresh" : ""}`, `compose-${draftId}`);
}

/** startDraft creates a draft (a new message, or a reply or forward of sourceId) and opens it. */
export async function startDraft(client: Client, kind: DraftKind, sourceId?: number, quote?: string): Promise<void> {
  const params: DraftCreateParams =
    sourceId === undefined ? { kind } : { kind, sourceId, ...(quote === undefined ? {} : { quote }) };
  const d = await client.draft.create(params).catch((err: unknown) => {
    if (params.quote === undefined || !(err instanceof RPCError) || err.code !== ErrorCode.invalidParams) throw err;
    // Older maild versions reject the unknown member. Keep Reply usable with the whole quote.
    return client.draft.create({ kind, sourceId });
  });
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
