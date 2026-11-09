// The command palette in the web build (docs/specs/ui.md, Command palette:
// The web build): Firefox keeps Ctrl+Shift+P, so the toolbar has a button.
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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
  const mock = new MockTransport(mockData({ inbox: 3, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("listbox", { name: "Messages" });
  return mock;
}

describe("the web build's Commands button", () => {
  it("opens the palette", async () => {
    await setup();
    fireEvent.click(screen.getByRole("button", { name: "Commands" }));
    const sheet = await screen.findByRole("dialog", { name: "Commands" });
    expect(sheet).toBeTruthy();
    expect(useUI.getState().palette).toBe(true);
  });

  it("keeps Ctrl+Shift+P too", async () => {
    await setup();
    fireEvent.keyDown(window, { key: "P", ctrlKey: true, shiftKey: true });
    await screen.findByRole("dialog", { name: "Commands" });
    fireEvent.keyDown(screen.getByRole("combobox", { name: "Command" }), { key: "Escape" });
    await waitFor(() => expect(useUI.getState().palette).toBe(false));
  });

  it("opens no palette while a contact is being edited", async () => {
    await setup();
    useUI.setState({ module: "people", peopleEditing: "new" });
    fireEvent.click(await screen.findByRole("button", { name: "Commands" }));
    expect(useUI.getState().palette).toBe(false);
    expect(screen.queryByRole("dialog", { name: "Commands" })).toBeNull();
  });
});
