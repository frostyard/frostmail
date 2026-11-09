// Ctrl+F finds in the message shown, and keeps the search field's meaning
// where no message shows (ADR-0052; docs/specs/ui.md, Find in the message,
// Keyboard map).
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
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

async function setup({ select = true } = {}) {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
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
  if (select) {
    fireEvent.click(row);
    const reader = screen.getByRole("region", { name: "Message" });
    await waitFor(() =>
      expect(within(reader).getAllByRole("button", { name: "More Actions" }).length).toBeGreaterThan(0),
    );
  }
  return { list, row };
}

/** press sends a key to the window and says whether the window took it. */
function press(key: string, mods: { ctrlKey?: boolean; shiftKey?: boolean; altKey?: boolean } = {}): boolean {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...mods });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}

const findField = () => screen.queryByRole("searchbox", { name: "Find in Message" });
const searchField = () => screen.getByRole("searchbox", { name: "Search" });

describe("Ctrl+F", () => {
  it("opens the find bar with the field focused while a message shows", async () => {
    await setup();
    expect(press("f", { ctrlKey: true })).toBe(true);
    const field = await screen.findByRole("searchbox", { name: "Find in Message" });
    await waitFor(() => expect(document.activeElement).toBe(field));
    // A second Ctrl+F keeps the bar, and the field comes back to it.
    fireEvent.change(field, { target: { value: "Lisbon" } });
    fireEvent.blur(field);
    expect(press("f", { ctrlKey: true })).toBe(true);
    expect(findField()).toBe(field);
    expect(document.activeElement).toBe(field);
  });

  it("keeps the bar's own words out of the search, and out of the print", async () => {
    await setup();
    press("f", { ctrlKey: true });
    const field = await screen.findByRole("searchbox", { name: "Find in Message" });
    fireEvent.change(field, { target: { value: "Previous Match" } });
    expect(screen.getByText("No Matches")).toBeTruthy();
    // The bar marks itself as no part of the message, which no match may
    // touch, and the print leaves it out.
    const bar = field.closest("[data-find-skip]");
    expect(bar?.className).toContain("print:hidden");
  });

  it("focuses the search field instead when no message shows", async () => {
    await setup({ select: false });
    expect(press("f", { ctrlKey: true })).toBe(true);
    expect(document.activeElement).toBe(searchField());
    expect(findField()).toBeNull();
  });

  it("closes the bar on Done, and the reader has the keys again", async () => {
    await setup();
    press("f", { ctrlKey: true });
    await screen.findByRole("searchbox", { name: "Find in Message" });
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(findField()).toBeNull());
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Message" }));
  });

  it("closes the bar when the reader shows another selection", async () => {
    const { list } = await setup();
    press("f", { ctrlKey: true });
    await screen.findByRole("searchbox", { name: "Find in Message" });
    const rows = within(list).getAllByRole("option");
    const other = rows[1];
    if (!other) throw new Error("no second row");
    fireEvent.click(other);
    await waitFor(() => expect(findField()).toBeNull());
    // Back on the first message, the bar stays closed.
    const first = within(list).getAllByRole("option")[0];
    if (!first) throw new Error("no first row");
    fireEvent.click(first);
    const reader = screen.getByRole("region", { name: "Message" });
    await waitFor(() =>
      expect(within(reader).getAllByRole("button", { name: "More Actions" }).length).toBeGreaterThan(0),
    );
    expect(findField()).toBeNull();
  });
});

describe("Ctrl+Alt+F", () => {
  it("focuses the search field while a message shows", async () => {
    await setup();
    expect(press("f", { ctrlKey: true, altKey: true })).toBe(true);
    expect(document.activeElement).toBe(searchField());
    expect(findField()).toBeNull();
  });
});
