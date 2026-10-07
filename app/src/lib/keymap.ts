// Keyboard shortcuts (docs/specs/ui.md, Keyboard map). Task T-0034
// implements commandFor; the stub maps nothing.

/** Command is what a shortcut asks the focused pane or the app to do. */
export type Command =
  | "previous"
  | "next"
  | "extendPrevious"
  | "extendNext"
  | "first"
  | "last"
  | "selectAll"
  | "pageDown"
  | "pageUp"
  | "delete"
  | "archive"
  | "toggleRead"
  | "toggleFlag"
  | "focusSearch"
  | "escape"
  | "getMail"
  | "allInboxes"
  | "toggleSidebar"
  | "nextPane"
  | "previousPane"
  | "contextMenu";

/** KeyInput is the part of a KeyboardEvent the keymap reads. */
export interface KeyInput {
  key: string;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
  metaKey: boolean;
}

/** commandFor maps a key press to a command, or null. */
export function commandFor(_e: KeyInput, _inTextField: boolean): Command | null {
  return null;
}
