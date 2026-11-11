// Showing a message by its id, from outside the list: a notification, a
// permanent link, a contact card or a follow-up.
import { useMail, useUI } from "../data/stores";
import type { Client, MessageSummary } from "../rpc/gen/api";

type Source = ReturnType<typeof useUI.getState>["source"];
interface Target {
  source: Source;
  summary: MessageSummary;
}

/** lookup finds what to show for a message: its account's inbox (or its first mailbox), or null. */
async function lookup(client: Client, id: number): Promise<Target | null> {
  const [s] = await client.message.summaries({ ids: [id] });
  if (!s) return null;
  const inbox = useMail.getState().mailboxes.find((mb) => mb.accountId === s.accountId && mb.role === "inbox");
  const mailboxId = inbox && s.mailboxIds.includes(inbox.id) ? inbox.id : s.mailboxIds[0];
  const source: Source = mailboxId === undefined ? { kind: "allInboxes" } : { kind: "mailbox", mailboxId };
  return { source, summary: s };
}

function apply(t: Target): void {
  const ui = useUI.getState();
  // A list filter could hide the message: revealing it shows it.
  if (ui.listFilter !== "all") ui.setListFilter("all");
  ui.setSource(t.source);
  ui.select([t.summary.id], t.summary.id);
  // The list keeps the message selected with its summary, loaded or not.
  ui.setRevealing(t.summary);
}

/** revealMessage shows a message in its account's inbox (or its first mailbox) and selects it. */
export async function revealMessage(client: Client, id: number): Promise<void> {
  const t = await lookup(client, id);
  if (t) apply(t);
}
