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
    "Ctrl+Alt+n",
    "Ctrl+Alt+r",
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

describe("compose shortcuts", () => {
  const key = (k: string, ctrl: boolean, shift: boolean) => ({
    key: k,
    ctrlKey: ctrl,
    altKey: false,
    shiftKey: shift,
    metaKey: false,
  });
  it("maps compose, reply, reply all and forward", () => {
    expect(commandFor(key("n", true, false), false)).toBe("compose");
    expect(commandFor(key("r", true, false), false)).toBe("reply");
    expect(commandFor(key("R", true, true), false)).toBe("replyAll");
    expect(commandFor(key("F", true, true), false)).toBe("forward");
  });
  it("opens the selection with Enter, except in a text field", () => {
    expect(commandFor(key("Enter", false, false), false)).toBe("open");
    expect(commandFor(key("Enter", false, false), true)).toBeNull();
  });
  it("keeps Ctrl+N and Ctrl+R for text fields but forwards from one", () => {
    expect(commandFor(key("n", true, false), true)).toBeNull();
    expect(commandFor(key("r", true, false), true)).toBeNull();
    expect(commandFor(key("F", true, true), true)).toBe("forward");
  });
});
