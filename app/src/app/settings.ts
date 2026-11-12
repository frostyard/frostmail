// Opening the settings window (docs/specs/settings-ui.md, Window): a route
// the frame shows.
import { openFrameWindow } from "../lodge/frameWindow";
import { openOutside } from "../lodge/lodge";
import { type SettingsPane, useSettingsPane } from "./settingsPane";

/**
 * openSettings shows the settings window on the pane given: the frame reads
 * the pane from the store, which is the frame's own context.
 */
export async function openSettings(pane?: SettingsPane): Promise<void> {
  if (pane !== undefined) useSettingsPane.getState().setPane(pane);
  openFrameWindow({ kind: "settings" });
}

/** openInBrowser opens an http or https URL in a new tab, through the shell. */
export async function openInBrowser(url: string): Promise<void> {
  if (!openOutside(url)) window.open(url, "_blank", "noopener");
}
