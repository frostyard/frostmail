// Opening a message from outside the window: a notification runs
// frostmail --open-message <id> (docs/design/desktop.md, One app instance).
// The first launch asks for its request once; later launches arrive as the
// open-message event.
import { invoke, isTauri } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";

import { useMail, useUI } from "../data/stores";
import type { Client } from "../rpc/gen/api";

/** revealMessage shows a message in its account's inbox (or its first mailbox) and selects it. */
export async function revealMessage(client: Client, id: number): Promise<void> {
  const [s] = await client.message.summaries({ ids: [id] });
  if (!s) return;
  const inbox = useMail.getState().mailboxes.find((mb) => mb.accountId === s.accountId && mb.role === "inbox");
  const mailboxId = inbox && s.mailboxIds.includes(inbox.id) ? inbox.id : s.mailboxIds[0];
  const ui = useUI.getState();
  ui.setSource(mailboxId === undefined ? { kind: "allInboxes" } : { kind: "mailbox", mailboxId });
  ui.select([id], id);
}

/** watchOpenRequests reveals the launch's message and every later one; it returns the cleanup. */
export function watchOpenRequests(client: Client): () => void {
  if (!isTauri()) return () => {};
  const reveal = (id: number) =>
    void revealMessage(client, id).catch((err: unknown) => console.warn("open message", err));
  void invoke<number | null>("startup_message").then((id) => {
    if (id !== null) reveal(id);
  });
  let stop: (() => void) | null = null;
  let stopped = false;
  void listen<number>("open-message", (e) => reveal(e.payload)).then((unlisten) => {
    if (stopped) unlisten();
    else stop = unlisten;
  });
  return () => {
    stopped = true;
    stop?.();
  };
}
