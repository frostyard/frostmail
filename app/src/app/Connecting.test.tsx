// The "Server Settings…" button of a window with no maild (docs/specs/
// settings-ui.md, Server pane; plan 0013's Phase 6): only the desktop app, and
// only after a connection it lost, offers the Server pane.
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useMail } from "../data/stores";
import { Connecting } from "./App";

const env = vi.hoisted(() => ({ tauri: true, invoke: vi.fn() }));

vi.mock("@tauri-apps/api/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/core")>()),
  invoke: env.invoke,
  isTauri: () => env.tauri,
}));
vi.mock("@tauri-apps/api/window", () => ({
  getCurrentWindow: () => ({
    close: async () => {},
    isMaximized: async () => false,
    onResized: async () => () => {},
    show: async () => {},
  }),
}));
vi.mock("@tauri-apps/api/webview", () => ({
  getCurrentWebview: () => ({ onDragDropEvent: async () => () => {} }),
}));
vi.mock("@tauri-apps/api/event", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/event")>()),
  listen: async () => () => {},
}));

beforeEach(() => {
  env.tauri = true;
  env.invoke.mockReset();
  env.invoke.mockImplementation(async () => null);
});

const button = () => screen.queryByRole("button", { name: "Server Settings…" });

describe("a window with no maild", () => {
  it("offers the Server pane after a lost connection", async () => {
    const user = userEvent.setup();
    useMail.setState({ connection: { state: "lost", reason: "no maild" } });
    render(<Connecting />);
    expect(screen.getByText("Reconnecting to maild… (no maild)")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Server Settings…" }));
    expect(env.invoke).toHaveBeenCalledWith("open_settings", { pane: "server" });
  });

  it("offers none while the first connection is made", () => {
    useMail.setState({ connection: { state: "connecting" } });
    render(<Connecting />);
    expect(screen.getByText("Connecting to maild…")).toBeTruthy();
    expect(button()).toBeNull();
  });

  it("offers none in a browser", () => {
    env.tauri = false;
    useMail.setState({ connection: { state: "lost", reason: "no maild" } });
    render(<Connecting />);
    expect(button()).toBeNull();
  });
});
