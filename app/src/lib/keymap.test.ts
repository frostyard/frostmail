// CONTRACT TEST for task card T-0034 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { type Command, commandFor, type KeyInput } from "./keymap";

function k(spec: string): KeyInput {
  const parts = spec.split("+");
  const key = parts.pop() ?? "";
  return {
    key: key === "Space" ? " " : key,
    ctrlKey: parts.includes("Ctrl"),
    altKey: parts.includes("Alt"),
    shiftKey: parts.includes("Shift"),
    metaKey: parts.includes("Meta"),
  };
}

describe("commandFor outside text fields", () => {
  it.each<[string, Command]>([
    ["ArrowUp", "previous"],
    ["ArrowDown", "next"],
    ["Shift+ArrowUp", "extendPrevious"],
    ["Shift+ArrowDown", "extendNext"],
    ["Home", "first"],
    ["End", "last"],
    ["Ctrl+a", "selectAll"],
    ["Ctrl+A", "selectAll"],
    ["Space", "pageDown"],
    ["Shift+Space", "pageUp"],
    ["Delete", "delete"],
    ["Backspace", "delete"],
    ["Ctrl+Alt+a", "archive"],
    ["Ctrl+Shift+U", "toggleRead"],
    ["Ctrl+Shift+u", "toggleRead"],
    ["Ctrl+Shift+L", "toggleFlag"],
    ["Ctrl+Alt+f", "focusSearch"],
    ["Ctrl+f", "focusSearch"],
    ["Escape", "escape"],
    ["Ctrl+Shift+N", "getMail"],
    ["Ctrl+1", "allInboxes"],
    ["Ctrl+Alt+s", "toggleSidebar"],
    ["Tab", "nextPane"],
    ["Shift+Tab", "previousPane"],
    ["ContextMenu", "contextMenu"],
    ["Shift+F10", "contextMenu"],
  ])("%s is %s", (spec, want) => {
    expect(commandFor(k(spec), false)).toBe(want);
  });

  it.each([
    "a",
    "Ctrl+Shift+a",
    "Alt+ArrowUp",
    "Ctrl+ArrowDown",
    "Ctrl+Delete",
    "Shift+Delete",
    "Ctrl+Alt+Shift+a",
    "Meta+a",
    "Meta+ArrowDown",
    "Ctrl+Meta+1",
    "Ctrl+2",
    "Ctrl+n",
    "Ctrl+r",
    "F10",
    "Alt+Tab",
  ])("%s is nothing", (spec) => {
    expect(commandFor(k(spec), false)).toBeNull();
  });
});

describe("commandFor in a text field", () => {
  it.each<[string, Command]>([
    ["Escape", "escape"],
    ["Tab", "nextPane"],
    ["Shift+Tab", "previousPane"],
    ["Ctrl+Shift+N", "getMail"],
    ["Ctrl+1", "allInboxes"],
    ["Ctrl+Alt+f", "focusSearch"],
    ["Ctrl+Alt+s", "toggleSidebar"],
    ["Ctrl+Shift+U", "toggleRead"],
  ])("%s is still %s", (spec, want) => {
    expect(commandFor(k(spec), true)).toBe(want);
  });

  it.each([
    "ArrowUp",
    "ArrowDown",
    "Home",
    "End",
    "Space",
    "Delete",
    "Backspace",
    "Ctrl+a",
    "Ctrl+f",
    "ContextMenu",
    "Shift+ArrowDown",
  ])("%s belongs to the field", (spec) => {
    expect(commandFor(k(spec), true)).toBeNull();
  });
});
