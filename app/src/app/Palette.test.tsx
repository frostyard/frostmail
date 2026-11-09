// The command palette in the main window (docs/specs/ui.md, Command
// palette): it runs a command as its key does, by the same dispatch.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
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

async function setup({ select = true } = {}) {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
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
  if (select) fireEvent.click(row);
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { id, mock, calls };
}

const key = (k: string, mods: { ctrlKey?: boolean; shiftKey?: boolean; altKey?: boolean } = {}) =>
  fireEvent.keyDown(window, { key: k, ...mods });

// openPalette presses Ctrl+Shift+P, and types the query into the palette's field.
function openPalette(query = "") {
  key("P", { ctrlKey: true, shiftKey: true });
  const field = screen.getByRole("combobox", { name: "Command" });
  if (query !== "") fireEvent.change(field, { target: { value: query } });
  return field;
}

const dialog = () => screen.queryByRole("dialog", { name: "Commands" });

describe("the command palette", () => {
  it("opens with Ctrl+Shift+P and archives as Ctrl+Alt+A does", async () => {
    const { id, calls } = await setup();
    const field = openPalette("arch");
    fireEvent.keyDown(field, { key: "Enter" });
    await waitFor(() => expect(calls("message.move")).toEqual([{ ids: [id], mailboxId: FIXTURE.archive }]));
    await waitFor(() => expect(dialog()).toBeNull());
    expect(calls("message.move")).toHaveLength(1);
  });

  it("archives the same way with its key", async () => {
    const { id, calls } = await setup();
    key("a", { ctrlKey: true, altKey: true });
    await waitFor(() => expect(calls("message.move")).toEqual([{ ids: [id], mailboxId: FIXTURE.archive }]));
    expect(dialog()).toBeNull();
  });

  it("leaves focus on the Search field after running Search", async () => {
    await setup();
    const field = openPalette("sear");
    fireEvent.keyDown(field, { key: "Enter" });
    await waitFor(() => expect(dialog()).toBeNull());
    expect(document.activeElement).toBe(screen.getByRole("searchbox", { name: "Search" }));
  });

  it("keeps a command that needs a selection disabled, and does nothing", async () => {
    const { calls } = await setup({ select: false });
    const field = openPalette("reply");
    const row = within(screen.getByRole("listbox", { name: "Commands" })).getAllByRole("option")[0] as HTMLElement;
    expect(row.getAttribute("aria-disabled")).toBe("true");
    expect(row.textContent).toContain("Select a message first");

    fireEvent.keyDown(field, { key: "Enter" });
    // The palette stays, and nothing composed or wrote.
    expect(dialog()).not.toBeNull();
    expect(calls("draft.create")).toEqual([]);
    expect(calls("message.move")).toEqual([]);
  });

  it("closes on Escape without running a command", async () => {
    const { calls } = await setup();
    const field = openPalette("arch");
    fireEvent.keyDown(field, { key: "Escape" });
    await waitFor(() => expect(dialog()).toBeNull());
    expect(calls("message.move")).toEqual([]);
  });
});
