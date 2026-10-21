// The pane splitters (docs/specs/ui.md, Layout: Panes): lodge-ui's Splitter,
// which resizes a pane with the pointer and with the arrow keys, and
// reports the pane's share of the window.
import { fireEvent, render, screen } from "@testing-library/react";
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

/** setup renders the wide main window, with its panes and splitters. */
async function setup() {
  vi.stubGlobal("matchMedia", (media: string) => ({
    matches: false,
    media,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("listbox", { name: "Messages" });
}

describe("the pane splitters", () => {
  it("report their pane's share of the window", async () => {
    await setup();
    const sidebar = Math.round((useUI.getState().sidebarWidth / window.innerWidth) * 100);
    const list = Math.round((useUI.getState().listWidth / window.innerWidth) * 100);
    expect(screen.getByRole("separator", { name: "Resize sidebar" }).getAttribute("aria-valuenow")).toBe(
      String(sidebar),
    );
    expect(screen.getByRole("separator", { name: "Resize message list" }).getAttribute("aria-valuenow")).toBe(
      String(list),
    );
  });

  it("resize their pane with the arrow keys, 16 pixels at a time", async () => {
    await setup();
    const splitter = screen.getByRole("separator", { name: "Resize sidebar" });
    const before = useUI.getState().sidebarWidth;

    fireEvent.keyDown(splitter, { key: "ArrowRight" });
    expect(useUI.getState().sidebarWidth).toBe(before + 16);
    fireEvent.keyDown(splitter, { key: "ArrowLeft" });
    fireEvent.keyDown(splitter, { key: "ArrowLeft" });
    expect(useUI.getState().sidebarWidth).toBe(before - 16);
  });

  it("stop at the pane's limits when a key presses past them", async () => {
    await setup();
    const splitter = screen.getByRole("separator", { name: "Resize message list" });
    for (let i = 0; i < 40; i++) fireEvent.keyDown(splitter, { key: "ArrowRight" });
    // The list stops at its own maximum, or where the reader's minimum
    // leaves room (docs/specs/ui.md, Layout: Panes).
    const widest = useUI.getState().listWidth;
    expect(widest).toBeGreaterThan(360);
    expect(widest).toBeLessThanOrEqual(560);
    for (let i = 0; i < 40; i++) fireEvent.keyDown(splitter, { key: "ArrowLeft" });
    expect(useUI.getState().listWidth).toBe(280);
  });
});
