// CONTRACT TEST for task card T-0064 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { commandFor, type KeyInput } from "./keymap";

const ctrl = (key: string): KeyInput => ({ key, ctrlKey: true, altKey: false, shiftKey: false, metaKey: false });

describe("module keys", () => {
  it("map Ctrl+2, Ctrl+3 and Ctrl+4 to the modules, Ctrl+1 to All Inboxes", () => {
    expect(commandFor(ctrl("1"), false)).toBe("allInboxes");
    expect(commandFor(ctrl("2"), false)).toBe("showCalendar");
    expect(commandFor(ctrl("3"), false)).toBe("showPeople");
    expect(commandFor(ctrl("4"), false)).toBe("showTasks");
  });

  it("work in text fields", () => {
    for (const key of ["1", "2", "3", "4"]) expect(commandFor(ctrl(key), true)).not.toBeNull();
  });

  it("need Ctrl alone", () => {
    expect(commandFor({ ...ctrl("3"), shiftKey: true }, false)).toBeNull();
    expect(commandFor({ ...ctrl("3"), ctrlKey: false }, false)).toBeNull();
  });
});
