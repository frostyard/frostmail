// Undo in Calendar and People (docs/specs/pim-ui.md, Undo in Calendar and
// People; ADR-0032): each module keeps its own list of the restores its
// writes returned, newest last, and Ctrl+Z restores the newest. The event
// editor shares the main window's document, so its writes join the same list.
import { create } from "zustand";

import { withNotify } from "../lib/notify";
import type { Client, InviteNotice } from "../rpc/gen/api";
import { UNDO_LIMIT } from "./undo";

/** PimModule is a module with its own undo list. */
export type PimModule = "calendar" | "people";

/** PimRestore is one write's undo: the restore it returned. */
export interface PimRestore {
  module: PimModule;
  restoreId: string;
}

/** usePimUndo keeps each module's restores, oldest first. */
export const usePimUndo = create<Record<PimModule, string[]>>(() => ({ calendar: [], people: [] }));

/** rememberRestore keeps a write's restore in its module's list, at most UNDO_LIMIT of them. */
export function rememberRestore({ module, restoreId }: PimRestore): void {
  if (restoreId === "") return;
  usePimUndo.setState((state) => ({ [module]: [...state[module], restoreId].slice(-UNDO_LIMIT) }));
}

/**
 * undoPim restores a module's newest write and forgets it; false when there
 * is none. A calendar restore that would tell invitees asks first with ask
 * (ADR-0035): when the user cancels, or there is no ask, the write stays on
 * the list and undoPim returns false.
 */
export async function undoPim(
  client: Client,
  module: PimModule,
  ask: (notice: InviteNotice) => Promise<boolean> = async () => false,
): Promise<boolean> {
  const restoreId = usePimUndo.getState()[module].at(-1);
  if (restoreId === undefined) return false;
  usePimUndo.setState((state) => ({ [module]: state[module].slice(0, -1) }));
  if (module === "people") {
    await client.people.restore({ restoreId });
    return true;
  }
  const restored = await withNotify(async (notify) => {
    await client.calendar.restore({ restoreId, ...(notify ? { notify } : {}) });
    return true;
  }, ask);
  if (restored === null) rememberRestore({ module, restoreId });
  return restored === true;
}

/** announceRestore keeps a write in the undo list of the module it changed. */
export async function announceRestore(restore: PimRestore): Promise<void> {
  rememberRestore(restore);
}
