// Ctrl+F focuses the search field where no message shows, and Ctrl+Alt+F
// always does; with one message shown, Ctrl+F is the browser's find
// (ADR-0052; docs/specs/ui.md, Find in the message, Keyboard map).
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
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

const searchField = () => screen.getByRole("searchbox", { name: "Search" });

describe("Ctrl+F", () => {
  it("focuses the search field when no message shows", async () => {
    await setup({ select: false });
    expect(press("f", { ctrlKey: true })).toBe(true);
    expect(document.activeElement).toBe(searchField());
  });
});

describe("Ctrl+Alt+F", () => {
  it("focuses the search field while a message shows", async () => {
    await setup();
    expect(press("f", { ctrlKey: true, altKey: true })).toBe(true);
    expect(document.activeElement).toBe(searchField());
  });
});
