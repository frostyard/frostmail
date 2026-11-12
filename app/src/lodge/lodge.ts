// The web build inside lodge's shell (docs/specs/lodge-app.md, The web
// build): app/ runs only as lodge's mail app, so it connects through
// @frostyard/lodge-client and reaches the shell through the Lodge it holds.

/** STORAGE_PREFIX goes before every local-storage key: lodge's apps share one origin. */
export const STORAGE_PREFIX = "mail:";

export type { Lodge } from "@frostyard/lodge-client";

import type { Lodge } from "@frostyard/lodge-client";

let current: Lodge | null = null;

/** setLodge keeps the connection main.tsx made. */
export function setLodge(l: Lodge | null): void {
  current = l;
}

/** lodge is the shell connection, or null before the connection is made. */
export function lodge(): Lodge | null {
  return current;
}

/**
 * openOutside opens an http, https or mailto URL in a new browser tab
 * through the shell, which adds noopener and noreferrer. It must run in
 * the click that asked, so the tab may open.
 */
export function openOutside(url: string): boolean {
  if (!current) return false;
  current.open(url);
  return true;
}
