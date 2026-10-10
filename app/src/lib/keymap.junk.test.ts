// CONTRACT TEST for task card T-0103 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { commandFor } from "./keymap";

const key = (k: string, mods: { ctrl?: boolean; alt?: boolean; shift?: boolean } = {}) => ({
  key: k,
  ctrlKey: mods.ctrl ?? false,
  altKey: mods.alt ?? false,
  shiftKey: mods.shift ?? false,
  metaKey: false,
});

describe("the junk shortcut", () => {
  it("is Ctrl+Shift+J, also in a text field", () => {
    expect(commandFor(key("J", { ctrl: true, shift: true }), false)).toBe("junk");
    expect(commandFor(key("j", { ctrl: true, shift: true }), true)).toBe("junk");
    expect(commandFor(key("j", { ctrl: true }), false)).toBeNull();
    expect(commandFor(key("j", { ctrl: true, alt: true, shift: true }), false)).toBeNull();
  });
});
