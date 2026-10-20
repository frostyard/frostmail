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

/** useUnsavedDrafts returns the unsaved drafts of a scope, refreshed on draft and Outbox changes and when count changes. */
export function useUnsavedDrafts(client: Client, scope: { accountId?: number } | null, count: number): Draft[] {
  const [drafts, setDrafts] = useState<Draft[]>([]);
  const accountId = scope?.accountId;
  const active = scope !== null;
  // biome-ignore lint/correctness/useExhaustiveDependencies: count re-reads them when the list changes, as a server copy arrives.
  useEffect(() => {
    if (!active) {
      setDrafts([]);
      return;
    }
    let current = true;
    const load = () => {
      void client.draft
        .unsaved(accountId === undefined ? {} : { accountId })
        .then((list) => {
          if (current) setDrafts(list);
        })
        .catch((err: unknown) => console.warn("unsaved drafts", err));
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
  }, [client, active, accountId, count]);
  return drafts;
}
