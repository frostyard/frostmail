// Opening the settings window (docs/specs/settings-ui.md, Window). In lodge
// it is a route the frame shows; outside lodge's frame it opens in a new tab
// at #/settings.
import { openFrameWindow } from "../lodge/frameWindow";
import { openOutside } from "../lodge/lodge";
import { type SettingsPane, useSettingsPane } from "./settingsPane";

/** isSettingsWindow reports whether this window shows the settings. */
export function isSettingsWindow(): boolean {
  return window.location.hash === "#/settings";
}

/**
 * openSettings shows the settings window on the pane given: lodge's frame
 * reads it from the store, which is the frame's own context. A browser tab
 * keeps Accounts.
 */
export async function openSettings(pane?: SettingsPane): Promise<void> {
  if (pane !== undefined) useSettingsPane.getState().setPane(pane);
  if (openFrameWindow({ kind: "settings" })) return;
  window.open(`${window.location.pathname}#/settings`, "settings");
}

/** openInBrowser opens an http or https URL in a new tab, in lodge through the shell. */
export async function openInBrowser(url: string): Promise<void> {
  if (!openOutside(url)) window.open(url, "_blank", "noopener");
}
