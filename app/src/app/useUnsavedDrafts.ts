// The drafts a Drafts list shows before their server copies arrive
// (docs/specs/ui.md, Message list): draft.unsaved for the Drafts mailbox's
// account, or every account's for All Drafts, refreshed as drafts, the
// Outbox and the list change.
import { useEffect, useState } from "react";

import type { Source } from "../data/stores";
import type { Client, Draft, Mailbox } from "../rpc/gen/api";

/** draftsScope is the drafts a source lists: one account's for its Drafts mailbox, every account's for All Drafts, else none. */
export function draftsScope(source: Source, mailboxes: readonly Mailbox[]): { accountId?: number } | null {
  if (source.kind === "role") return source.role === "drafts" ? {} : null;
  if (source.kind !== "mailbox") return null;
  const mailbox = mailboxes.find((mb) => mb.id === source.mailboxId);
  return mailbox?.role === "drafts" ? { accountId: mailbox.accountId } : null;
}

/** Unsaved is what useUnsavedDrafts answers: the drafts, and why they could not be listed. */
export interface Unsaved {
  drafts: Draft[];
  error: string | null;
}

const NONE: Unsaved = { drafts: [], error: null };

/**
 * useUnsavedDrafts returns the unsaved drafts of a scope, refreshed on
 * draft and Outbox changes and when count changes. It answers a scope's
 * own drafts only: another scope's, still shown while this one's load, would
 * put another account's drafts in this Drafts list.
 */
export function useUnsavedDrafts(client: Client, scope: { accountId?: number } | null, count: number): Unsaved {
  const accountId = scope?.accountId;
  const key = scope === null ? null : (accountId ?? "all");
  const [state, setState] = useState<Unsaved & { key: number | "all" | null }>({ ...NONE, key: null });
  // biome-ignore lint/correctness/useExhaustiveDependencies: count re-reads them when the list changes, as a server copy arrives.
  useEffect(() => {
    if (key === null) return;
    let current = true;
    const load = () => {
      void client.draft
        .unsaved(accountId === undefined ? {} : { accountId })
        .then((drafts) => {
          if (current) setState({ key, drafts, error: null });
        })
        .catch((err: unknown) => {
          console.warn("unsaved drafts", err);
          if (current) setState({ key, drafts: [], error: err instanceof Error ? err.message : String(err) });
        });
    };
    load();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const off = client.transport.onEvent((event) => {
      if (event.event !== "draft.changed" && event.event !== "outbox.changed") return;
      clearTimeout(timer);
      timer = setTimeout(load, 100);
    });
    return () => {
      current = false;
      clearTimeout(timer);
      off();
    };
  }, [client, key, accountId, count]);
  return key !== null && state.key === key ? state : NONE;
}
