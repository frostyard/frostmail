// Commands on the selection and the window, shared by the keyboard, the
// toolbar and context menus (docs/specs/ui.md, Behavior).
import type { Source } from "../data/stores";
import type { ViewModel } from "../data/view";
import type { Client, Mailbox, MessageSummary } from "../rpc/gen/api";
import { startDraft } from "./compose";

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

/** applyRules runs the enabled rules on the messages now. */
export async function applyRules(client: Client, ids: number[]): Promise<void> {
  if (ids.length === 0) return;
  await client.rule.apply({ ids });
}

/** remindMessages sets a reminder, or clears it when no time is supplied. */
export async function remindMessages(client: Client, ids: number[], at?: Date): Promise<void> {
  if (ids.length === 0) return;
  await client.message.remind(at ? { ids, at: at.toISOString() } : { ids });
}

/** archiveMailbox is where Archive moves the messages (see archiveOf). */
export function archiveMailbox(model: ViewModel | null, ids: number[], mailboxes: Mailbox[]): Mailbox | undefined {
  const first = selectedSummaries(model, ids)[0];
  return first ? archiveOf(first.accountId, mailboxes) : undefined;
}

/**
 * archiveOf is where Archive moves an account's messages: its archive mailbox,
 * or All Mail on an account whose mailboxes are Gmail labels.
 */
export function archiveOf(accountId: number, mailboxes: Mailbox[]): Mailbox | undefined {
  const own = mailboxes.filter((mb) => mb.accountId === accountId);
  const archive = own.find((mb) => mb.role === "archive");
  if (archive || !own.some((mb) => mb.label)) return archive;
  return own.find((mb) => mb.role === "all");
}

/** junkOf finds the account's junk mailbox. */
export function junkOf(accountId: number, mailboxes: Mailbox[]): Mailbox | undefined {
  return mailboxes.find((mb) => mb.accountId === accountId && mb.role === "junk");
}

/** SpamTarget describes whether the selection leaves junk and its destination. */
export interface SpamTarget {
  notSpam: boolean;
  to: Mailbox | undefined;
}

/** spamTarget chooses junk, or the inbox when every loaded row is in junk. */
export function spamTarget(model: ViewModel | null, ids: number[], mailboxes: Mailbox[]): SpamTarget {
  const rows = selectedSummaries(model, ids);
  const first = rows[0];
  if (!first) return { notSpam: false, to: undefined };
  const junk = junkOf(first.accountId, mailboxes);
  const notSpam = junk !== undefined && rows.every((row) => row.mailboxIds.includes(junk.id));
  const to = notSpam ? mailboxes.find((mb) => mb.accountId === first.accountId && mb.role === "inbox") : junk;
  return { notSpam, to };
}

/** toggleSpam moves the selection into junk or back into the inbox. */
export async function toggleSpam(
  client: Client,
  model: ViewModel | null,
  ids: number[],
  source: Source,
  mailboxes: Mailbox[],
): Promise<void> {
  await moveMessages(client, ids, spamTarget(model, ids, mailboxes).to, source, mailboxes);
}

/** moveTargets lists the message's account's mailboxes that do not hold it. */
export function moveTargets(message: MessageSummary | undefined, mailboxes: Mailbox[]): Mailbox[] {
  if (!message) return [];
  return mailboxes.filter((mb) => mb.accountId === message.accountId && !message.mailboxIds.includes(mb.id));
}

/** copyTargets limits Gmail destinations to labels other than Starred. */
export function copyTargets(message: MessageSummary | undefined, mailboxes: Mailbox[]): Mailbox[] {
  const targets = moveTargets(message, mailboxes);
  const labels = mailboxes.some((mb) => mb.accountId === message?.accountId && mb.label);
  return labels ? targets.filter((mb) => mb.label && mb.role !== "flagged") : targets;
}

/** copyMessages copies the selection to a mailbox. */
export async function copyMessages(client: Client, ids: number[], to: Mailbox | undefined): Promise<void> {
  if (!to || ids.length === 0) return;
  await client.message.copy({ ids, mailboxId: to.id });
}

/**
 * moveParams are message.move's params. When the list shows a mailbox of the
 * destination's account (or a unified one, such as All Sent), the messages
 * leave that one: on Gmail, moving out of a label removes only that label.
 */
export function moveParams(ids: number[], to: Mailbox, source: Source, mailboxes: Mailbox[]) {
  const from =
    source.kind === "mailbox"
      ? mailboxes.find((mb) => mb.id === source.mailboxId && mb.accountId === to.accountId)
      : source.kind === "role"
        ? mailboxes.find((mb) => mb.role === source.role && mb.accountId === to.accountId)
        : undefined;
  return from ? { ids, mailboxId: to.id, fromMailboxId: from.id } : { ids, mailboxId: to.id };
}

/** moveMessages moves messages to a mailbox (see moveParams). */
export async function moveMessages(
  client: Client,
  ids: number[],
  to: Mailbox | undefined,
  source: Source,
  mailboxes: Mailbox[],
): Promise<void> {
  if (!to || ids.length === 0) return;
  await client.message.move(moveParams(ids, to, source, mailboxes));
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

/** ComposeAction starts a draft from the main window. */
export type ComposeAction = "compose" | "reply" | "replyAll" | "forward";

/**
 * compose opens a new message, or a reply, reply all or forward of the last
 * selected message, in a compose window. Without a selection only compose
 * does anything.
 */
export async function compose(client: Client, action: ComposeAction, ids: number[]): Promise<void> {
  if (action === "compose") {
    await startDraft(client, "new");
    return;
  }
  const source = ids[ids.length - 1];
  if (source === undefined) return;
  const kind = action === "reply" ? "reply" : action === "replyAll" ? "replyall" : "forward";
  await startDraft(client, kind, source);
}
