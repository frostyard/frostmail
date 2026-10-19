// The web build inside lodge's shell (docs/specs/lodge-app.md, The web
// build). The build's lodge mode connects through @frostyard/lodge-client;
// everything else in the app asks this module whether it runs in lodge and
// reaches the shell through the Lodge it holds.

/** IN_LODGE is whether this is the web build for lodge's /app/mail/. */
export const IN_LODGE = import.meta.env.MODE === "lodge";

/** STORAGE_PREFIX goes before every local-storage key: lodge's apps share one origin. */
export const STORAGE_PREFIX = IN_LODGE ? "mail:" : "";

export type { Lodge } from "@frostyard/lodge-client";

import type { Lodge } from "@frostyard/lodge-client";

let current: Lodge | null = null;

/** setLodge keeps the connection main.tsx made. */
export function setLodge(l: Lodge | null): void {
  current = l;
}

/** lodge is the shell connection, or null outside lodge. */
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
