// In lodge's web build Ctrl+F belongs to the browser, whose find reaches the
// reader's content-origin frame (ADR-0052; docs/specs/ui.md, Find in the
// message).
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { useFrameWindow } from "../lodge/frameWindow";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

vi.mock("../lodge/lodge", async (original) => ({
  ...(await original<typeof import("../lodge/lodge")>()),
  IN_LODGE: true,
}));

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useFrameWindow.setState({ open: null });
});

async function setup() {
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
  fireEvent.click(row);
  const reader = screen.getByRole("region", { name: "Message" });
  await waitFor(() =>
    expect(within(reader).getAllByRole("button", { name: "More Actions" }).length).toBeGreaterThan(0),
  );
}

/** press sends a key to the window and says whether the window took it. */
function press(key: string, mods: { ctrlKey?: boolean; shiftKey?: boolean; altKey?: boolean } = {}): boolean {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...mods });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}

describe("Ctrl+F in the web build", () => {
  it("leaves the key to the browser while a message shows", async () => {
    await setup();
    expect(press("f", { ctrlKey: true })).toBe(false);
    expect(screen.queryByRole("searchbox", { name: "Find in Message" })).toBeNull();
  });

  it("keeps Ctrl+Alt+F for the search field", async () => {
    await setup();
    expect(press("f", { ctrlKey: true, altKey: true })).toBe(true);
    expect(document.activeElement).toBe(screen.getByRole("searchbox", { name: "Search" }));
  });

  it("lists no Find in Message in the palette", async () => {
    await setup();
    press("P", { ctrlKey: true, shiftKey: true });
    const list = await screen.findByRole("listbox", { name: "Commands" });
    expect(within(list).queryByText("Find in Message")).toBeNull();
    expect(within(list).getByText("Search")).toBeTruthy();
  });
});
