// What the settings window keeps outside its connection to maild
// (docs/specs/settings-ui.md, Window and Server pane): the open pane, which a
// reconnection keeps, and the Server pane's save, which ends the window's own
// connection in the Flatpak (ADR-0046), so its progress and error must
// outlive the pane's remount.
import { create } from "zustand";

/** SETTINGS_PANES are the settings window's panes, in tab order. */
export const SETTINGS_PANES = ["general", "accounts", "signatures", "rules", "signin", "server"] as const;

/** SettingsPane names a pane of the settings window. */
export type SettingsPane = (typeof SETTINGS_PANES)[number];

/** paneFrom is the pane a value names, or null for anything else. */
export function paneFrom(value: unknown): SettingsPane | null {
  if (typeof value !== "string") return null;
  return SETTINGS_PANES.find((pane) => pane === value) ?? null;
}

/** SettingsPaneState is the settings window's own state. */
export interface SettingsPaneState {
  /** The pane shown: the one the window's opener named, else Accounts. */
  pane: SettingsPane;
  /** The Server pane's save runs; a change of mode can take a minute. */
  busy: boolean;
  /** What the running save does (saveStatus in ServerPane), fixed when it starts. */
  status: string | null;
  /** The Server pane's last save's error. */
  error: string | null;
  setPane: (pane: SettingsPane) => void;
  setBusy: (busy: boolean, status?: string | null) => void;
  setError: (error: string | null) => void;
}

/** useSettingsPane is the settings window's store. */
export const useSettingsPane = create<SettingsPaneState>((set) => ({
  // The shell's initialization script names the pane the opener asked for
  // (app/src-tauri, open_settings).
  pane: paneFrom(window.__frostmailSettingsPane) ?? "accounts",
  busy: false,
  status: null,
  error: null,
  setPane: (pane) => set({ pane }),
  setBusy: (busy, status = null) => set({ busy, status }),
  setError: (error) => set({ error }),
}));
