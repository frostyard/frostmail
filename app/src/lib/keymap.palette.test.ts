// The command palette's commands (docs/specs/ui.md, Command palette).
import { describe, expect, it } from "vitest";

import { type Command, commandFor, matchesQuery, PALETTE, paletteItems, shortcutText } from "./keymap";

const press = (key: string, mods: { ctrlKey?: boolean; shiftKey?: boolean; altKey?: boolean } = {}) => ({
  key,
  ctrlKey: mods.ctrlKey ?? false,
  shiftKey: mods.shiftKey ?? false,
  altKey: mods.altKey ?? false,
  metaKey: false,
});

describe("Ctrl+Shift+P", () => {
  it("opens the palette, in a text field too", () => {
    expect(commandFor(press("P", { ctrlKey: true, shiftKey: true }), false)).toBe("palette");
    expect(commandFor(press("P", { ctrlKey: true, shiftKey: true }), true)).toBe("palette");
  });

  it("leaves Ctrl+P to print", () => {
    expect(commandFor(press("p", { ctrlKey: true }), false)).toBe("print");
  });
});

describe("PALETTE", () => {
  const entries = Object.entries(PALETTE) as [Command, (typeof PALETTE)[Command]][];

  it("has an entry, or null, for every command", () => {
    expect(entries.length).toBeGreaterThanOrEqual(45);
  });

  it("leaves the moves to their keys", () => {
    const moves = entries.filter(([, entry]) => entry === null).map(([command]) => command);
    expect(new Set(moves)).toEqual(
      new Set([
        "previous",
        "next",
        "extendPrevious",
        "extendNext",
        "first",
        "last",
        "pageDown",
        "pageUp",
        "left",
        "right",
        "nextPane",
        "previousPane",
        "open",
        "escape",
        "contextMenu",
        "palette",
      ]),
    );
  });

  it("names every command it lists, with its keys", () => {
    for (const [command, entry] of entries) {
      if (entry === null) continue;
      const names = Object.values(entry.name).filter((name) => name !== undefined && name !== "");
      expect(names.length, command).toBeGreaterThan(0);
      expect(shortcutText(command), command).not.toBe("");
    }
  });
});

describe("shortcutText", () => {
  it("writes a command's keys as the palette shows them", () => {
    expect(shortcutText("archive")).toBe("Ctrl+Alt+A");
    expect(shortcutText("focusSearch")).toBe("Ctrl+Alt+F");
    expect(shortcutText("find")).toBe("Ctrl+F");
    expect(shortcutText("delete")).toBe("Delete, Backspace");
    expect(shortcutText("previousPeriod")).toBe("Ctrl+←");
    expect(shortcutText("palette")).toBe("Ctrl+Shift+P");
    expect(shortcutText("settings")).toBe("Ctrl+,");
    expect(shortcutText("pageDown")).toBe("Space");
    expect(shortcutText("contextMenu")).toBe("Menu, Shift+F10");
  });
});

describe("paletteItems", () => {
  it("lists Mail's commands with the reason of each one the selection cannot run", () => {
    const items = paletteItems("mail", 0);
    const row = (name: string) => items.find((item) => item.name === name);
    expect(row("Reply")?.reason).toBe("Select a message first");
    expect(row("Print…")?.reason).toBe("Select one message first");
    expect(row("Find in Message")?.reason).toBe("Select one message first");
    expect(row("New Message")?.reason).toBeUndefined();
    expect(row("New Message")?.shortcut).toBe("Ctrl+N");
  });

  it("gives Find in Message its keys, right after Search, while one message shows", () => {
    const items = paletteItems("mail", 1);
    const commands = items.map((item) => item.command);
    const at = commands.indexOf("focusSearch");
    expect(commands.slice(at, at + 2)).toEqual(["focusSearch", "find"]);
    const find = items.find((item) => item.command === "find");
    expect(find?.name).toBe("Find in Message");
    expect(find?.shortcut).toBe("Ctrl+F");
    expect(find?.reason).toBeUndefined();
  });

  it("leaves Find in Message out of the web build's palette, which has the browser's find", () => {
    const names = paletteItems("mail", 1, { web: true }).map((item) => item.name);
    expect(names).not.toContain("Find in Message");
    expect(names).toContain("Search");
  });

  it("lists Calendar's own commands", () => {
    const names = paletteItems("calendar", 0).map((item) => item.name);
    expect(names).toContain("New Event");
    expect(names).toContain("Day View");
    expect(names).not.toContain("Archive");
  });

  it("keeps PALETTE's order", () => {
    expect(
      paletteItems("mail", 1)
        .map((item) => item.command)
        .slice(0, 3),
    ).toEqual(["compose", "reply", "replyAll"]);
  });
});

describe("matchesQuery", () => {
  it("finds a name by the start of its words", () => {
    expect(matchesQuery("Archive", "arch")).toBe(true);
    expect(matchesQuery("Mark as Read/Unread", "unr")).toBe(true);
    expect(matchesQuery("Archive", "rch")).toBe(false);
    expect(matchesQuery("New Message", "new m")).toBe(true);
    expect(matchesQuery("New Message", "NEW")).toBe(true);
    expect(matchesQuery("Archive", "")).toBe(true);
    expect(matchesQuery("Raw Source…", "raw s")).toBe(true);
  });
});
