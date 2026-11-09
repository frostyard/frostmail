// Keyboard shortcuts (docs/specs/ui.md, Keyboard map). A binding matches
// when the key matches (letters case-insensitively, Space is " ") and all
// of Ctrl, Alt and Shift match exactly; Meta is never bound. In a text
// field only Escape, Tab, Shift+Tab, Ctrl+1–4 and bindings that combine
// Ctrl with Alt or with Shift still apply, so the field keeps arrows,
// Space, Delete, Backspace, Ctrl+A and Ctrl+F. Ctrl+Shift+P is one of them:
// the command palette (docs/specs/ui.md, Command palette) opens from a text
// field too. PALETTE names each command in the modules where it acts, with
// what it needs, and shortcutText writes its keys for the palette's rows.

import type { Module } from "./modules";

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
  | "undo"
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
  | "redirect"
  | "print"
  | "rawSource"
  | "allHeaders"
  | "palette"
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
  { key: "z", ctrl: true, alt: false, shift: false, command: "undo" },
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
  { key: "e", ctrl: true, alt: false, shift: true, command: "redirect" },
  { key: "p", ctrl: true, alt: false, shift: false, command: "print" },
  { key: "u", ctrl: true, alt: true, shift: false, command: "rawSource" },
  { key: "h", ctrl: true, alt: false, shift: true, command: "allHeaders" },
  { key: ",", ctrl: true, alt: false, shift: false, command: "settings" },
  { key: "p", ctrl: true, alt: false, shift: true, command: "palette" },
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

// KEY_TEXT writes the keys of the map that are not a letter or a digit.
const KEY_TEXT: Record<string, string> = {
  " ": "Space",
  arrowleft: "←",
  arrowright: "→",
  arrowup: "↑",
  arrowdown: "↓",
  delete: "Delete",
  backspace: "Backspace",
  escape: "Escape",
  enter: "Enter",
  tab: "Tab",
  home: "Home",
  end: "End",
  f10: "F10",
  contextmenu: "Menu",
};

function keyText(key: string): string {
  const named = KEY_TEXT[key];
  if (named !== undefined) return named;
  return /^[a-z]$/.test(key) ? key.toUpperCase() : key;
}

/** shortcutText writes a command's keys as the palette's rows show them:
 *  Ctrl, Alt, Shift, then the key ("Ctrl+Alt+A"), and every key of the
 *  command joined by ", " ("Ctrl+Alt+F, Ctrl+F"); "" for a move with none. */
export function shortcutText(command: Command): string {
  return BINDINGS.filter((b) => b.command === command)
    .map(bindingText)
    .join(", ");
}

function bindingText(b: Binding): string {
  return `${b.ctrl ? "Ctrl+" : ""}${b.alt ? "Alt+" : ""}${b.shift ? "Shift+" : ""}${keyText(b.key)}`;
}

/** PaletteEntry names a command in each module where it acts, and says
 *  what it needs (docs/specs/ui.md, Command palette). */
export interface PaletteEntry {
  name: Readonly<Partial<Record<Module, string>>>;
  needs?: "selection" | "message";
}

const each = (name: string) => ({ mail: name, calendar: name, people: name, tasks: name });
const mail = (name: string) => ({ mail: name });
const calendar = (name: string) => ({ calendar: name });
const mailPeople = (name: string) => ({ mail: name, people: name });

/**
 * PALETTE is the palette's commands, in the order it lists them, and null
 * for a move: one with no name of its own to find. Every command has an
 * entry, so a new one fails tsc until it has its place here.
 */
export const PALETTE: Readonly<Record<Command, PaletteEntry | null>> = {
  compose: { name: { mail: "New Message", calendar: "New Event", people: "New Contact", tasks: "New Task" } },
  reply: { name: mail("Reply"), needs: "selection" },
  replyAll: { name: mail("Reply All"), needs: "selection" },
  forward: { name: mail("Forward"), needs: "selection" },
  redirect: { name: mail("Redirect…"), needs: "selection" },
  archive: { name: mail("Archive"), needs: "selection" },
  delete: { name: { mail: "Delete", calendar: "Delete", tasks: "Delete" }, needs: "selection" },
  junk: { name: mail("Mark as Spam/Not Spam"), needs: "selection" },
  toggleRead: { name: mail("Mark as Read/Unread"), needs: "selection" },
  toggleFlag: { name: mail("Flag/Unflag"), needs: "selection" },
  undo: { name: { mail: "Undo", calendar: "Undo", people: "Undo" } },
  selectAll: { name: mail("Select All") },
  getMail: { name: mail("Get Mail") },
  focusSearch: { name: mailPeople("Search") },
  print: { name: mail("Print…"), needs: "message" },
  rawSource: { name: mail("Raw Source…"), needs: "message" },
  allHeaders: { name: mail("Show/Hide All Headers"), needs: "message" },
  today: { name: calendar("Go to Today") },
  dayView: { name: calendar("Day View") },
  weekView: { name: calendar("Week View") },
  monthView: { name: calendar("Month View") },
  previousPeriod: { name: calendar("Previous Period") },
  nextPeriod: { name: calendar("Next Period") },
  allInboxes: { name: each("All Inboxes") },
  showCalendar: { name: each("Show Calendar") },
  showPeople: { name: each("Show People") },
  showTasks: { name: each("Show Tasks") },
  toggleSidebar: { name: each("Show/Hide Sidebar") },
  settings: { name: each("Settings…") },

  previous: null,
  next: null,
  extendPrevious: null,
  extendNext: null,
  first: null,
  last: null,
  pageDown: null,
  pageUp: null,
  left: null,
  right: null,
  nextPane: null,
  previousPane: null,
  open: null,
  escape: null,
  contextMenu: null,
  palette: null,
};

// SELECTION_REASON says what a module's selection is, for a command that
// needs one (docs/specs/ui.md, Command palette: Disabled rows).
const SELECTION_REASON: Record<Module, string> = {
  mail: "Select a message first",
  calendar: "Select an event first",
  people: "Select a contact first",
  tasks: "Select a task first",
};

const MESSAGE_REASON = "Select one message first";

/** PaletteItem is one row of the palette: a command, its name in the
 *  module, its keys, and why it is disabled, if it is. */
export interface PaletteItem {
  command: Command;
  name: string;
  shortcut: string;
  reason?: string;
}

/** paletteItems lists a module's commands in PALETTE's order, with the
 *  reason of each one that the selection (the count) cannot run. */
export function paletteItems(module: Module, selected: number): PaletteItem[] {
  const items: PaletteItem[] = [];
  for (const [command, entry] of Object.entries(PALETTE) as [Command, PaletteEntry | null][]) {
    const name = entry?.name[module];
    if (!entry || name === undefined) continue;
    const shortcut = shortcutText(command);
    if (entry.needs === "selection" && selected === 0)
      items.push({ command, name, shortcut, reason: SELECTION_REASON[module] });
    else if (entry.needs === "message" && selected !== 1)
      items.push({ command, name, shortcut, reason: MESSAGE_REASON });
    else items.push({ command, name, shortcut });
  }
  return items;
}

/** matchesQuery finds a name by a query, case-insensitively: each word of
 *  the query must start a word of the name, where a word breaks at anything
 *  but a letter or a digit ("arch" finds Archive, "new m" New Message, "unr"
 *  Mark as Read/Unread). An empty query matches every name. */
export function matchesQuery(name: string, query: string): boolean {
  const wanted = query
    .toLowerCase()
    .split(/\s+/)
    .filter((word) => word !== "");
  if (wanted.length === 0) return true;
  const words = name.toLowerCase().split(/[^\p{L}\p{N}]+/u);
  return wanted.every((want) => words.some((word) => word.startsWith(want)));
}
