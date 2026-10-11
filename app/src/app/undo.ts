import { create } from "zustand";

import type { Client, FlagChanges, MessageCopyParams, MessageSetFlagsParams, MessageSummary } from "../rpc/gen/api";

/** UNDO_LIMIT is the number of actions the main window remembers. */
export const UNDO_LIMIT = 20;

/** UndoCall restores a message's earlier mailbox membership or flags. */
export type UndoCall =
  | { method: "move"; params: { ids: number[]; mailboxId: number; fromMailboxId: number } }
  | { method: "copy"; params: MessageCopyParams }
  | { method: "setFlags"; params: MessageSetFlagsParams };

/** useUndo keeps inverse actions in memory, oldest first. */
export const useUndo = create<{ entries: UndoCall[][] }>(() => ({ entries: [] }));

function mergeCalls(calls: UndoCall[]): UndoCall[] {
  const merged = new Map<string, UndoCall>();
  for (const call of calls) {
    const { ids, ...params } = call.params;
    const key = JSON.stringify({ method: call.method, ...params });
    const existing = merged.get(key);
    if (existing) existing.params.ids.push(...ids);
    else merged.set(key, call);
  }
  return [...merged.values()];
}

/** moveBack restores the memberships each surviving message left. */
export function moveBack(before: MessageSummary[], after: MessageSummary[]): UndoCall[] {
  const current = new Map(after.map((row) => [row.id, row]));
  const moves: UndoCall[] = [];
  const copies: UndoCall[] = [];
  for (const row of before) {
    const next = current.get(row.id);
    if (!next) continue;
    const left = row.mailboxIds.filter((id) => !next.mailboxIds.includes(id));
    const entered = next.mailboxIds.filter((id) => !row.mailboxIds.includes(id));
    const mailboxId = left[0];
    const fromMailboxId = entered[0];
    if (mailboxId === undefined) continue;
    if (fromMailboxId !== undefined) {
      moves.push({ method: "move", params: { ids: [row.id], mailboxId, fromMailboxId } });
    }
    for (const id of fromMailboxId === undefined ? left : left.slice(1)) {
      copies.push({ method: "copy", params: { ids: [row.id], mailboxId: id } });
    }
  }
  return mergeCalls([...moves, ...copies]);
}

/** flagsBack restores the earlier read marks and flag colors that changed. */
export function flagsBack(before: MessageSummary[], changes: FlagChanges): UndoCall[] {
  const calls: UndoCall[] = [];
  for (const row of before) {
    if (changes.seen !== undefined && changes.seen !== row.flags.seen) {
      calls.push({ method: "setFlags", params: { ids: [row.id], changes: { seen: row.flags.seen } } });
    }
    const flagColor = row.flags.flagged ? row.flags.flagColor : 0;
    if (changes.flagColor !== undefined && changes.flagColor !== flagColor) {
      calls.push({ method: "setFlags", params: { ids: [row.id], changes: { flagColor } } });
    }
  }
  return mergeCalls(calls);
}

function remember(calls: UndoCall[]) {
  if (calls.length === 0) return;
  useUndo.setState((state) => ({ entries: [...state.entries, calls].slice(-UNDO_LIMIT) }));
}

/** undoableMove remembers the inverse of a successful move or delete. */
export async function undoableMove(client: Client, ids: number[], act: () => Promise<void>): Promise<void> {
  if (ids.length === 0) return;
  const before = await client.message.summaries({ ids });
  await act();
  const after = await client.message.summaries({ ids });
  remember(moveBack(before, after));
}

/** undoableFlags changes flags and remembers the earlier values. */
export async function undoableFlags(client: Client, ids: number[], changes: FlagChanges): Promise<void> {
  if (ids.length === 0) return;
  const before = await client.message.summaries({ ids });
  await client.message.setFlags({ ids, changes });
  remember(flagsBack(before, changes));
}

/** undo removes and performs the newest inverse action, if there is one. */
export async function undo(client: Client): Promise<boolean> {
  const entry = useUndo.getState().entries.at(-1);
  if (!entry) return false;
  useUndo.setState((state) => ({ entries: state.entries.slice(0, -1) }));
  for (const call of entry) {
    switch (call.method) {
      case "move":
        await client.message.move(call.params);
        break;
      case "copy":
        await client.message.copy(call.params);
        break;
      case "setFlags":
        await client.message.setFlags(call.params);
        break;
    }
  }
  return true;
}
