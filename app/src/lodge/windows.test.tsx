// The app's other windows as routes inside lodge's frame (lodge/frameWindow.ts).
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "../app/App";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { closeFrameWindow, openFrameWindow, useFrameWindow, windowFromRoute, windowRoute } from "./frameWindow";

vi.mock("./lodge", async (original) => ({ ...(await original<typeof import("./lodge")>()), IN_LODGE: true }));

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useFrameWindow.setState({ open: null });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("frame window routes", () => {
  it("round-trip every window", () => {
    for (const w of [
      { kind: "compose", draftId: 12, fresh: true },
      { kind: "compose", draftId: 3, fresh: false },
      { kind: "settings" },
      { kind: "reminders" },
      { kind: "event", target: { kind: "edit", id: 4, recurrenceId: "" } },
      { kind: "event", target: { kind: "edit", id: 4, recurrenceId: "20261018T090000Z" } },
    ] as const) {
      expect(windowFromRoute(windowRoute(w))).toEqual(w);
    }
    expect(windowRoute({ kind: "compose", draftId: 12, fresh: true })).toBe("/compose/12?fresh");
  });

  it("are not the main window's routes", () => {
    for (const r of ["/", "/mailbox/3", "/message/4", "/compose/x", "/compose/0", "/settingsx", "/event/x"]) {
      expect(windowFromRoute(r)).toBeNull();
    }
  });

  it("open and close in the frame", () => {
    expect(openFrameWindow({ kind: "settings" })).toBe(true);
    expect(useFrameWindow.getState().open).toEqual({ kind: "settings" });
    expect(closeFrameWindow()).toBe(true);
    expect(useFrameWindow.getState().open).toBeNull();
  });
});

describe("the lodge frame", () => {
  async function setup() {
    const mock = new MockTransport(mockData({ inbox: 3, now: new Date("2026-10-07T12:00:00Z") }), { undoMs: 50 });
    const close = vi.spyOn(window, "close").mockImplementation(() => {});
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    render(<App connect={() => Promise.resolve(mock)} />);
    await screen.findByRole("listbox", { name: "Messages" });
    const calls = (method: string) => mock.calls.filter((c) => c.method === method);
    return { mock, close, open, calls };
  }

  it("composes and sends a new message in place of the main window", async () => {
    const user = userEvent.setup();
    const { calls, close, open } = await setup();
    fireEvent.keyDown(window, { key: "n", ctrlKey: true });
    await screen.findByRole("toolbar", { name: "Compose" });
    expect(useFrameWindow.getState().open).toMatchObject({ kind: "compose", fresh: true });
    expect(open).not.toHaveBeenCalled();
    expect(screen.getByRole("listbox", { name: "Messages", hidden: true }).closest("[hidden]")).not.toBeNull();

    // The main window's keys stay quiet while compose covers it.
    fireEvent.keyDown(window, { key: "n", ctrlKey: true });
    expect(calls("draft.create")).toHaveLength(1);

    await user.type(screen.getByRole("textbox", { name: "Subject" }), "From lodge");
    await user.type(screen.getByRole("combobox", { name: "To" }), "bob@x.test,");
    await user.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(useFrameWindow.getState().open).toBeNull());
    expect(calls("draft.send")).toHaveLength(1);
    expect(close).not.toHaveBeenCalled();
    const list = screen.getByRole("listbox", { name: "Messages" });
    expect(list.closest("[hidden]")).toBeNull();
    expect(within(list).getAllByRole("option").length).toBeGreaterThan(0);
  });

  it("opens Settings in the frame and closes it back to the mail", async () => {
    const { close, open } = await setup();
    fireEvent.keyDown(window, { key: ",", ctrlKey: true });
    await waitFor(() => expect(useFrameWindow.getState().open).toEqual({ kind: "settings" }));
    expect(open).not.toHaveBeenCalled();
    closeFrameWindow();
    await waitFor(() => expect(screen.getByRole("listbox", { name: "Messages" }).closest("[hidden]")).toBeNull());
    expect(close).not.toHaveBeenCalled();
  });
});
