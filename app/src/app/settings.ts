// Opening the settings window (docs/specs/settings-ui.md, Window). In the
// app it is the Tauri window labeled settings, which learns its role from
// an initialization script; in a browser (the dev gateway) it opens in a new
// tab at #/settings.
import { invoke, isTauri } from "@tauri-apps/api/core";

import { openFrameWindow } from "../lodge/frameWindow";
import { openOutside } from "../lodge/lodge";
import { remoteMaild } from "../rpc/transport/tauri";
import { type SettingsPane, useSettingsPane } from "./settingsPane";

declare global {
  interface Window {
    /** Set by the settings window's initialization script. */
    __frostmailSettings?: boolean;
    /** Set by the settings window's initialization script: the pane its opener named. */
    __frostmailSettingsPane?: string;
  }
}

/** isSettingsWindow reports whether this window shows the settings. */
export function isSettingsWindow(): boolean {
  return window.__frostmailSettings === true || window.location.hash === "#/settings";
}

/**
 * openSettings shows the settings window, focusing it when it is open, on the
 * pane given (M6): the shell opens a new window on it or sends the open one
 * the settings-pane event, and lodge's frame reads it from the store, which is
 * the frame's own context. A browser tab keeps Accounts.
 */
export async function openSettings(pane?: SettingsPane): Promise<void> {
  if (pane !== undefined) useSettingsPane.getState().setPane(pane);
  if (openFrameWindow({ kind: "settings" })) return;
  if (isTauri()) {
    await invoke("open_settings", pane === undefined ? {} : { pane });
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
