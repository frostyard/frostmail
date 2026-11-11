// The pane the settings window shows (docs/specs/settings-ui.md, Window). It
// lives outside the window's connection to maild, so a reconnection keeps it.
import { create } from "zustand";

/** SettingsPane names a pane of the settings window. */
export type SettingsPane = "general" | "accounts" | "signatures" | "snippets" | "rules" | "signin";

/** SettingsPaneState is the settings window's own state. */
export interface SettingsPaneState {
  /** The pane shown: the one the window's opener named, else Accounts. */
  pane: SettingsPane;
  setPane: (pane: SettingsPane) => void;
}

/** useSettingsPane is the settings window's store. */
export const useSettingsPane = create<SettingsPaneState>((set) => ({
  pane: "accounts",
  setPane: (pane) => set({ pane }),
}));
