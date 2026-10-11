// CONTRACT TEST for task card T-0131 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { commandFor } from "../lib/keymap";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function setup({ readOnly = false, select = true } = {}) {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
  for (const a of data.accounts) a.readOnly = readOnly;
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  const list = await screen.findByRole("listbox", { name: "Messages" });
  const row = await waitFor(() => {
    const r = within(list).queryAllByRole("option")[0];
    if (!r) throw new Error("no rows yet");
    return r;
  });
  const id = Number(row.getAttribute("data-message-id"));
  const reader = screen.getByRole("region", { name: "Message" });
  if (select) {
    fireEvent.click(row);
    await waitFor(() =>
      expect(within(reader).getAllByRole("button", { name: "More Actions" }).length).toBeGreaterThan(0),
    );
  }
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { id, row, reader, calls };
}

const key = (k: string, mods: { ctrlKey?: boolean; shiftKey?: boolean; altKey?: boolean }) =>
  fireEvent.keyDown(window, { key: k, ...mods });

describe("the keymap's Phase 6 shortcuts", () => {
  it("maps Mail.app's keys with Ctrl for ⌘", () => {
    const k = (key: string, ctrlKey: boolean, shiftKey: boolean, altKey: boolean) => ({
      key,
      ctrlKey,
      shiftKey,
      altKey,
      metaKey: false,
    });
    expect(commandFor(k("E", true, true, false), false)).toBe("redirect");
    expect(commandFor(k("p", true, false, false), false)).toBe("print");
    expect(commandFor(k("u", true, false, true), false)).toBe("rawSource");
    expect(commandFor(k("H", true, true, false), false)).toBe("allHeaders");
    expect(commandFor(k("z", true, false, false), false)).toBe("undo");
    expect(commandFor(k("p", true, false, false), true)).toBeNull();
    expect(commandFor(k("z", true, false, false), true)).toBeNull();
  });
});

describe("the reader's shortcuts", () => {
  it("opens Raw Source with Ctrl+Alt+U", async () => {
    const { id, calls } = await setup();
    key("u", { ctrlKey: true, altKey: true });
    await screen.findByRole("dialog", { name: "Raw Source" });
    expect(calls("message.source")).toEqual([{ id }]);
  });

  it("shows and hides all headers with Ctrl+Shift+H", async () => {
    const { reader } = await setup();
    key("H", { ctrlKey: true, shiftKey: true });
    await within(reader).findByRole("region", { name: "All Headers" });
    key("H", { ctrlKey: true, shiftKey: true });
    await waitFor(() => expect(within(reader).queryByRole("region", { name: "All Headers" })).toBeNull());
  });

  it("prints with Ctrl+P", async () => {
    const print = vi.fn();
    vi.stubGlobal("print", print);
    await setup();
    key("p", { ctrlKey: true });
    expect(print).toHaveBeenCalledTimes(1);
  });

  it("does nothing without a selection", async () => {
    const print = vi.fn();
    vi.stubGlobal("print", print);
    const { calls } = await setup({ select: false });
    key("p", { ctrlKey: true });
    key("u", { ctrlKey: true, altKey: true });
    key("E", { ctrlKey: true, shiftKey: true });
    expect(print).not.toHaveBeenCalled();
    expect(calls("message.source")).toEqual([]);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("shows the keys in the More Actions menu", async () => {
    const { reader } = await setup();
    fireEvent.click(within(reader).getAllByRole("button", { name: "More Actions" })[0] as HTMLElement);
    const menu = screen.getByRole("menu");
    const text = (label: string) =>
      Array.from(menu.querySelectorAll('[role="menuitem"]')).find((el) => el.textContent?.startsWith(label))
        ?.textContent;
    expect(text("Show All Headers")).toContain("Ctrl+Shift+H");
    expect(text("Raw Source…")).toContain("Ctrl+Alt+U");
    expect(text("Print…")).toContain("Ctrl+P");
  });
});

describe("Redirect's shortcut", () => {
  it("opens the Redirect sheet for the selection with Ctrl+Shift+E", async () => {
    const { row } = await setup();
    key("E", { ctrlKey: true, shiftKey: true });
    await screen.findByRole("dialog", { name: "Redirect" });
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.contextMenu(row, { clientX: 50, clientY: 50 });
    const item = Array.from((await screen.findByRole("menu")).querySelectorAll('[role="menuitem"]')).find((el) =>
      el.textContent?.startsWith("Redirect…"),
    );
    expect(item?.textContent).toContain("Ctrl+Shift+E");
  });

  it("does nothing on a read-only account", async () => {
    await setup({ readOnly: true });
    key("E", { ctrlKey: true, shiftKey: true });
    expect(screen.queryByRole("dialog", { name: "Redirect" })).toBeNull();
  });
});
