// Opening drafts in compose windows (docs/specs/compose-ui.md, Entry
// points). In the app each draft gets its own Tauri window, labeled
// compose-<id>, which learns its draft from an initialization script; in a
// browser (the dev gateway) it opens in a new tab at #/compose/<id>.
import { invoke, isTauri } from "@tauri-apps/api/core";

import type { Client, DraftKind } from "../rpc/gen/api";

declare global {
  interface Window {
    /** Set by the compose window's initialization script. */
    __frostmailCompose?: number;
    /** True when the draft was created for this window. */
    __frostmailComposeFresh?: boolean;
  }
}

/** ComposeTarget is the draft a compose window shows. */
export interface ComposeTarget {
  draftId: number;
  /** The draft was created to open this window, so closing it untouched discards it. */
  fresh: boolean;
}

/** composeTarget returns the draft this window composes, or null in the main window. */
export function composeTarget(): ComposeTarget | null {
  if (typeof window.__frostmailCompose === "number") {
    return { draftId: window.__frostmailCompose, fresh: window.__frostmailComposeFresh === true };
  }
  const m = /^#\/compose\/(\d+)(\?fresh)?$/.exec(window.location.hash);
  return m?.[1] ? { draftId: Number(m[1]), fresh: m[2] !== undefined } : null;
}

/** openCompose shows a draft in its compose window, focusing it when it is already open. */
export async function openCompose(draftId: number, fresh = false): Promise<void> {
  if (isTauri()) {
    await invoke("open_compose", { draftId, fresh });
    return;
  }
  window.open(`${window.location.pathname}#/compose/${draftId}${fresh ? "?fresh" : ""}`, `compose-${draftId}`);
}

/** startDraft creates a draft (a new message, or a reply or forward of sourceId) and opens it. */
export async function startDraft(client: Client, kind: DraftKind, sourceId?: number): Promise<void> {
  const d = await client.draft.create(sourceId === undefined ? { kind } : { kind, sourceId });
  await openCompose(d.id, true);
}

/** openDraftMessage opens a message from a Drafts mailbox as a draft. */
export async function openDraftMessage(client: Client, messageId: number): Promise<void> {
  const d = await client.draft.open({ messageId });
  await openCompose(d.id);
}
