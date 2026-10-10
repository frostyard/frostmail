// Keyboard shortcuts (docs/specs/ui.md, Keyboard map). A binding matches
// when the key matches (letters case-insensitively, Space is " ") and all
// of Ctrl, Alt and Shift match exactly; Meta is never bound. In a text
// field only Escape, Tab, Shift+Tab, Ctrl+1–4 and bindings that combine
// Ctrl with Alt or with Shift still apply, so the field keeps arrows,
// Space, Delete, Backspace, Ctrl+A and Ctrl+F.

/** Command is what a shortcut asks the focused pane or the app to do. */
export type Command =
  | "dayView"
  | "weekView"
  | "monthView"
  | "today"
  | "previousPeriod"
  | "nextPeriod"
  | "left"
  | "right"
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
  | "junk"
  | "toggleRead"
  | "toggleFlag"
  | "focusSearch"
  | "escape"
  | "getMail"
  | "allInboxes"
  | "showCalendar"
  | "showPeople"
  | "showTasks"
  | "toggleSidebar"
  | "nextPane"
  | "previousPane"
  | "contextMenu"
  | "open"
  | "compose"
  | "reply"
  | "replyAll"
  | "forward"
  | "settings";

/** KeyInput is the part of a KeyboardEvent the keymap reads. */
export interface KeyInput {
  key: string;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
  metaKey: boolean;
}

interface Binding {
  key: string;
  ctrl: boolean;
  alt: boolean;
  shift: boolean;
  command: Command;
}

const BINDINGS: Binding[] = [
  { key: "1", ctrl: true, alt: true, shift: false, command: "dayView" },
  { key: "2", ctrl: true, alt: true, shift: false, command: "weekView" },
  { key: "3", ctrl: true, alt: true, shift: false, command: "monthView" },
  { key: "t", ctrl: true, alt: false, shift: false, command: "today" },
  { key: "arrowleft", ctrl: true, alt: false, shift: false, command: "previousPeriod" },
  { key: "arrowright", ctrl: true, alt: false, shift: false, command: "nextPeriod" },
  { key: "arrowleft", ctrl: false, alt: false, shift: false, command: "left" },
  { key: "arrowright", ctrl: false, alt: false, shift: false, command: "right" },

  { key: "arrowup", ctrl: false, alt: false, shift: false, command: "previous" },
  { key: "arrowdown", ctrl: false, alt: false, shift: false, command: "next" },
  { key: "arrowup", ctrl: false, alt: false, shift: true, command: "extendPrevious" },
  { key: "arrowdown", ctrl: false, alt: false, shift: true, command: "extendNext" },
  { key: "home", ctrl: false, alt: false, shift: false, command: "first" },
  { key: "end", ctrl: false, alt: false, shift: false, command: "last" },
  { key: "a", ctrl: true, alt: false, shift: false, command: "selectAll" },
  { key: " ", ctrl: false, alt: false, shift: false, command: "pageDown" },
  { key: " ", ctrl: false, alt: false, shift: true, command: "pageUp" },
  { key: "delete", ctrl: false, alt: false, shift: false, command: "delete" },
  { key: "backspace", ctrl: false, alt: false, shift: false, command: "delete" },
  { key: "a", ctrl: true, alt: true, shift: false, command: "archive" },
  { key: "j", ctrl: true, alt: false, shift: true, command: "junk" },
  { key: "u", ctrl: true, alt: false, shift: true, command: "toggleRead" },
  { key: "l", ctrl: true, alt: false, shift: true, command: "toggleFlag" },
  { key: "f", ctrl: true, alt: true, shift: false, command: "focusSearch" },
  { key: "f", ctrl: true, alt: false, shift: false, command: "focusSearch" },
  { key: "escape", ctrl: false, alt: false, shift: false, command: "escape" },
  { key: "n", ctrl: true, alt: false, shift: true, command: "getMail" },
  { key: "2", ctrl: true, alt: false, shift: false, command: "showCalendar" },
  { key: "3", ctrl: true, alt: false, shift: false, command: "showPeople" },
  { key: "4", ctrl: true, alt: false, shift: false, command: "showTasks" },
  { key: "1", ctrl: true, alt: false, shift: false, command: "allInboxes" },
  { key: "s", ctrl: true, alt: true, shift: false, command: "toggleSidebar" },
  { key: "tab", ctrl: false, alt: false, shift: false, command: "nextPane" },
  { key: "tab", ctrl: false, alt: false, shift: true, command: "previousPane" },
  { key: "contextmenu", ctrl: false, alt: false, shift: false, command: "contextMenu" },
  { key: "f10", ctrl: false, alt: false, shift: true, command: "contextMenu" },
  { key: "enter", ctrl: false, alt: false, shift: false, command: "open" },
  { key: "n", ctrl: true, alt: false, shift: false, command: "compose" },
  { key: "r", ctrl: true, alt: false, shift: false, command: "reply" },
  { key: "r", ctrl: true, alt: false, shift: true, command: "replyAll" },
  { key: "f", ctrl: true, alt: false, shift: true, command: "forward" },
  { key: ",", ctrl: true, alt: false, shift: false, command: "settings" },
];

function allowedInTextField(b: Binding): boolean {
  if (b.key === "escape" || b.key === "tab") return true;
  if (["1", "2", "3", "4", ","].includes(b.key) && b.ctrl) return true;
  return b.ctrl && (b.alt || b.shift);
}

/** commandFor maps a key press to a command, or null. */
export function commandFor(e: KeyInput, inTextField: boolean): Command | null {
  if (e.metaKey) return null;
  const key = e.key.toLowerCase();
  for (const b of BINDINGS) {
    if (b.key === key && b.ctrl === e.ctrlKey && b.alt === e.altKey && b.shift === e.shiftKey) {
      if (inTextField && !allowedInTextField(b)) return null;
      return b.command;
    }
  }
  return null;
}
