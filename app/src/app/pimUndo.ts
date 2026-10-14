// Undo in Calendar and People (docs/specs/pim-ui.md, Undo in Calendar and
// People; ADR-0032): each module keeps its own list of the restores its
// writes returned, newest last, and Ctrl+Z restores the newest. Writes in
// another window (the event editor) announce their restores to the main
// window: a Tauri event there, a BroadcastChannel message in a browser.
import { isTauri } from "@tauri-apps/api/core";
import { emitTo, listen } from "@tauri-apps/api/event";
import { create } from "zustand";

import type { Client } from "../rpc/gen/api";
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

const EVENT = "pim-restore";

/** rememberRestore keeps a write's restore in its module's list, at most UNDO_LIMIT of them. */
export function rememberRestore({ module, restoreId }: PimRestore): void {
  if (restoreId === "") return;
  usePimUndo.setState((state) => ({ [module]: [...state[module], restoreId].slice(-UNDO_LIMIT) }));
}

/** undoPim restores a module's newest write and forgets it; false when there is none. */
export async function undoPim(client: Client, module: PimModule): Promise<boolean> {
  const restoreId = usePimUndo.getState()[module].at(-1);
  if (restoreId === undefined) return false;
  usePimUndo.setState((state) => ({ [module]: state[module].slice(0, -1) }));
  if (module === "calendar") await client.calendar.restore({ restoreId });
  else await client.people.restore({ restoreId });
  return true;
}

/** announceRestore hands a write made in another window to the main window's undo list. */
export async function announceRestore(restore: PimRestore): Promise<void> {
  if (isTauri()) {
    await emitTo("main", EVENT, restore);
    return;
  }
  const channel = new BroadcastChannel("frostmail");
  channel.postMessage({ kind: EVENT, ...restore });
  channel.close();
}

function isRestore(data: unknown): data is PimRestore {
  if (typeof data !== "object" || data === null) return false;
  const d = data as Record<string, unknown>;
  return (d.module === "calendar" || d.module === "people") && typeof d.restoreId === "string";
}

/** watchRestores keeps the restores other windows announce, in the main window; it returns the cleanup. */
export function watchRestores(): () => void {
  let stopped = false;
  const channel = new BroadcastChannel("frostmail");
  channel.onmessage = (event: MessageEvent<unknown>) => {
    if (event.origin !== "" && event.origin !== window.location.origin) return;
    const data = event.data;
    if (typeof data === "object" && data !== null && "kind" in data && data.kind === EVENT && isRestore(data)) {
      rememberRestore({ module: data.module, restoreId: data.restoreId });
    }
  };
  let stop: (() => void) | null = null;
  if (isTauri()) {
    void listen<unknown>(EVENT, (event) => {
      if (isRestore(event.payload)) rememberRestore(event.payload);
    })
      .then((unlisten) => {
        if (stopped) unlisten();
        else stop = unlisten;
      })
      .catch((err: unknown) => console.warn("watch restores", err));
  }
  return () => {
    stopped = true;
    channel.close();
    stop?.();
  };
}
