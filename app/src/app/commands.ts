// Commands on the selection and the window, shared by the keyboard, the
// toolbar and context menus (docs/specs/ui.md, Behavior).
import type { ViewModel } from "../data/view";
import type { Client, Mailbox } from "../rpc/gen/api";

/** selectedSummaries returns the loaded summaries of the selected IDs. */
export function selectedSummaries(model: ViewModel | null, ids: number[]) {
  if (!model) return [];
  return ids.flatMap((id) => {
    const i = model.indexOf(id);
    const row = i >= 0 ? model.row(i) : undefined;
    return row ? [row] : [];
  });
}

/** toggleRead marks the messages seen, or unseen when all already are. */
export async function toggleRead(client: Client, model: ViewModel | null, ids: number[]): Promise<void> {
  if (ids.length === 0) return;
  const rows = selectedSummaries(model, ids);
  const allSeen = rows.length > 0 && rows.every((r) => r.flags.seen);
  await client.message.setFlags({ ids, changes: { seen: !allSeen } });
}

/** toggleFlag clears the flag when the first message is flagged, else flags red. */
export async function toggleFlag(client: Client, model: ViewModel | null, ids: number[]): Promise<void> {
  if (ids.length === 0) return;
  const first = selectedSummaries(model, ids)[0];
  await client.message.setFlags({ ids, changes: { flagColor: first?.flags.flagged ? 0 : 1 } });
}

/** setFlagColor sets a flag color; 0 clears the flag. */
export async function setFlagColor(client: Client, ids: number[], color: number): Promise<void> {
  if (ids.length === 0) return;
  await client.message.setFlags({ ids, changes: { flagColor: color } });
}

/** archiveMailbox is the archive mailbox of the messages' account, if any. */
export function archiveMailbox(model: ViewModel | null, ids: number[], mailboxes: Mailbox[]): Mailbox | undefined {
  const first = selectedSummaries(model, ids)[0];
  if (!first) return undefined;
  return mailboxes.find((mb) => mb.accountId === first.accountId && mb.role === "archive");
}

/** getMail asks maild to sync every account now. */
export async function getMail(client: Client, accountIds: number[]): Promise<void> {
  await Promise.all(accountIds.map((accountId) => client.sync.now({ accountId })));
}

/** step moves a single selection by delta rows, clamped to the list. */
export function step(model: ViewModel, selected: number[], delta: number): number | null {
  if (model.count === 0) return null;
  const current = selected.length > 0 ? model.indexOf(selected[selected.length - 1] ?? -1) : -1;
  const next = current < 0 ? (delta > 0 ? 0 : model.count - 1) : current + delta;
  return Math.max(0, Math.min(model.count - 1, next));
}

/** rangeIds lists the loaded IDs between two indices, inclusive. */
export function rangeIds(model: ViewModel, a: number, b: number): number[] {
  const out: number[] = [];
  for (let i = Math.min(a, b); i <= Math.max(a, b); i++) {
    const row = model.row(i);
    if (row) out.push(row.id);
  }
  return out;
}
