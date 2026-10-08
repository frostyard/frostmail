// Opening the settings window (docs/specs/settings-ui.md, Window). In the
// app it is the Tauri window labeled settings, which learns its role from
// an initialization script; in a browser (the dev gateway) it opens in a new
// tab at #/settings.
import { invoke, isTauri } from "@tauri-apps/api/core";

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
  if (isTauri()) {
    await invoke("open_settings");
    return;
  }
  window.open(`${window.location.pathname}#/settings`, "settings");
}

/** openInBrowser opens an http or https URL in the system browser. */
export async function openInBrowser(url: string): Promise<void> {
  if (isTauri()) {
    await invoke("open_link", { url });
    return;
  }
  window.open(url, "_blank", "noopener");
}
