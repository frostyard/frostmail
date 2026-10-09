// CONTRACT TEST for task card T-0072 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { commandFor, type KeyInput } from "./keymap";

const key = (k: string, mods: Partial<KeyInput> = {}): KeyInput => ({
  key: k,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  metaKey: false,
  ...mods,
});

describe("calendar keys", () => {
  it("switches views with Ctrl+Alt+1, 2 and 3, even in a text field", () => {
    for (const field of [false, true]) {
      expect(commandFor(key("1", { ctrlKey: true, altKey: true }), field)).toBe("dayView");
      expect(commandFor(key("2", { ctrlKey: true, altKey: true }), field)).toBe("weekView");
      expect(commandFor(key("3", { ctrlKey: true, altKey: true }), field)).toBe("monthView");
    }
    expect(commandFor(key("4", { ctrlKey: true, altKey: true }), false)).toBeNull();
  });

  it("keeps Ctrl+1 to 4 for the modules", () => {
    expect(commandFor(key("1", { ctrlKey: true }), false)).toBe("allInboxes");
    expect(commandFor(key("2", { ctrlKey: true }), false)).toBe("showCalendar");
  });

  it("goes to today with Ctrl+T and pages with Ctrl+arrows, outside text fields", () => {
    expect(commandFor(key("t", { ctrlKey: true }), false)).toBe("today");
    expect(commandFor(key("T", { ctrlKey: true }), false)).toBe("today");
    expect(commandFor(key("ArrowLeft", { ctrlKey: true }), false)).toBe("previousPeriod");
    expect(commandFor(key("ArrowRight", { ctrlKey: true }), false)).toBe("nextPeriod");
    for (const k of ["t", "ArrowLeft", "ArrowRight"]) expect(commandFor(key(k, { ctrlKey: true }), true)).toBeNull();
  });

  it("moves with the left and right arrows, outside text fields", () => {
    expect(commandFor(key("ArrowLeft"), false)).toBe("left");
    expect(commandFor(key("ArrowRight"), false)).toBe("right");
    expect(commandFor(key("ArrowLeft"), true)).toBeNull();
    expect(commandFor(key("ArrowRight", { shiftKey: true }), false)).toBeNull();
  });
});
