// Opening the settings window (docs/specs/settings-ui.md, Window). In the
// app it is the Tauri window labeled settings, which learns its role from
// an initialization script; in a browser (the dev gateway) it opens in a new
// tab at #/settings.
import { invoke, isTauri } from "@tauri-apps/api/core";

import { openFrameWindow } from "../lodge/frameWindow";
import { openOutside } from "../lodge/lodge";
import { remoteMaild } from "../rpc/transport/tauri";

declare global {
  interface Window {
    /** Set by the settings window's initialization script. */
    __frostmailSettings?: boolean;
  }
}

/** isSettingsWindow reports whether this window shows the settings. */
export function isSettingsWindow(): boolean {
  return window.__frostmailSettings === true || window.location.hash === "#/settings";
}

/** openSettings shows the settings window, focusing it when it is open. */
export async function openSettings(): Promise<void> {
  if (openFrameWindow({ kind: "settings" })) return;
  if (isTauri()) {
    await invoke("open_settings");
    return;
  }
  window.open(`${window.location.pathname}#/settings`, "settings");
}

/** openInBrowser opens an http or https URL in the system browser, or in lodge through the shell. */
export async function openInBrowser(url: string): Promise<void> {
  if (isTauri()) {
    await invoke("open_link", { url });
    return;
  }
  if (!openOutside(url)) window.open(url, "_blank", "noopener");
}

/**
 * pasteSignIn reports whether a sign-in may finish with the address of the
 * page the browser could not open (ADR-0043): in a browser, or in the
 * desktop app on a maild behind lodge. The desktop app on its own maild
 * gets the loopback return, so it shows no such field.
 */
export function pasteSignIn(): boolean {
  return !isTauri() || remoteMaild();
}
