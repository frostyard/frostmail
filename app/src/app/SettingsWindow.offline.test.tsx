// The settings window without a maild (docs/specs/settings-ui.md, Window and
// Server pane; plan 0013's Phase 6): the Server pane still works, every other
// pane says what the main window says, the opener's pane wins, and a save's
// error outlives the pane's remount and reads the stored choice back.
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useMail } from "../data/stores";
import { SettingsWindow } from "./SettingsWindow";
import { useSettingsPane } from "./settingsPane";

const env = vi.hoisted(() => ({
  invoke: vi.fn(),
  listen: vi.fn(),
  pane: null as ((event: { payload: string }) => void) | null,
}));
const server = vi.hoisted(() => ({ lodge: null as string | null, fail: null as string | null, reads: 0 }));

vi.mock("@tauri-apps/api/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/core")>()),
  invoke: env.invoke,
  isTauri: () => true,
}));
vi.mock("@tauri-apps/api/window", () => ({ getCurrentWindow: () => ({ close: async () => {} }) }));
vi.mock("@tauri-apps/api/event", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/event")>()),
  listen: env.listen,
}));

beforeEach(() => {
  useSettingsPane.setState(useSettingsPane.getInitialState());
  useMail.setState({ connection: { state: "connecting" } });
  server.lodge = null;
  server.fail = null;
  server.reads = 0;
  env.pane = null;
  env.listen.mockReset();
  env.listen.mockImplementation(async (_name: string, handler: (event: { payload: string }) => void) => {
    env.pane = handler;
    return () => {};
  });
  env.invoke.mockReset();
  env.invoke.mockImplementation(async (command: string, args?: { config?: { lodge?: string } }) => {
    switch (command) {
      case "app_config_get":
        server.reads += 1;
        return server.lodge === null ? {} : { lodge: server.lodge };
      case "server_facts":
        return { flatpak: true, localData: true };
      case "app_config_set":
        if (server.fail !== null) throw server.fail;
        server.lodge = args?.config?.lodge ?? null;
        return null;
      default:
        return null;
    }
  });
});

const lodgeField = () => screen.getByRole("textbox", { name: "lodge:" });
const save = () => screen.getByRole("button", { name: "Save" }) as HTMLButtonElement;

/** chooseLodge asks for a maild behind lodge at an address. */
function chooseLodge(address = "https://lodge.example.ts.net") {
  fireEvent.click(screen.getByRole("radio", { name: "maild behind lodge" }));
  fireEvent.change(lodgeField(), { target: { value: address } });
}

describe("the settings window without a maild", () => {
  it("shows the Server pane and its toolbar", async () => {
    useSettingsPane.setState({ pane: "server" });
    render(<SettingsWindow offline />);
    expect(await screen.findByRole("form", { name: "Server" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Server" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button", { name: "Accounts" })).toBeTruthy();
  });

  it("says what a change of mode does, from the shell's facts", async () => {
    useSettingsPane.setState({ pane: "server" });
    render(<SettingsWindow offline />);
    await screen.findByRole("form", { name: "Server" });
    chooseLodge();
    await waitFor(() => expect(screen.getByRole("note").textContent).toMatch(/^This computer's maild stops\./));
  });

  it("says the first connecting text in place of a pane that needs maild", () => {
    useSettingsPane.setState({ pane: "accounts" });
    render(<SettingsWindow offline />);
    expect(screen.getByText("Connecting to maild…")).toBeTruthy();
    expect(screen.queryByRole("navigation", { name: "Accounts" })).toBeNull();
  });

  it("says why the maild is gone", () => {
    useSettingsPane.setState({ pane: "general" });
    useMail.setState({ connection: { state: "lost", reason: "lodge is down" } });
    render(<SettingsWindow offline />);
    expect(screen.getByText("Reconnecting to maild… (lodge is down)")).toBeTruthy();
  });

  it("switches to the pane its opener names", async () => {
    useSettingsPane.setState({ pane: "accounts" });
    render(<SettingsWindow offline />);
    await waitFor(() => expect(env.listen).toHaveBeenCalledWith("settings-pane", expect.any(Function)));
    act(() => env.pane?.({ payload: "server" }));
    expect(await screen.findByRole("form", { name: "Server" })).toBeTruthy();
    act(() => env.pane?.({ payload: "not-a-pane" }));
    expect(screen.getByRole("form", { name: "Server" })).toBeTruthy();
  });

  it("keeps a save's error across a remount", async () => {
    server.fail = "This computer's maild did not stop. Frostmail still uses it.";
    useSettingsPane.setState({ pane: "server" });
    const first = render(<SettingsWindow offline />);
    await screen.findByRole("form", { name: "Server" });
    chooseLodge();
    fireEvent.click(save());
    expect((await screen.findByRole("alert")).textContent).toBe(server.fail);
    first.unmount();
    render(<SettingsWindow offline />);
    expect((await screen.findByRole("alert")).textContent).toBe(server.fail);
  });

  it("reads the stored choice again after a failed save", async () => {
    server.fail = "This computer's maild did not stop. Frostmail still uses it.";
    useSettingsPane.setState({ pane: "server" });
    render(<SettingsWindow offline />);
    await screen.findByRole("form", { name: "Server" });
    await waitFor(() => expect(server.reads).toBe(1));
    chooseLodge();
    fireEvent.click(save());
    await screen.findByRole("alert");
    await waitFor(() => expect(server.reads).toBe(2));
    // The shell wrote the previous choice back: this computer's maild again.
    await waitFor(() =>
      expect((screen.getByRole("radio", { name: "maild on this computer" }) as HTMLInputElement).checked).toBe(true),
    );
    expect(save().disabled).toBe(true);
  });

  it("shows the stored choice when a save fails after the pane remounted", async () => {
    // The shell writes lodge first, then fails to stop maild and writes the
    // previous choice back; meanwhile the window lost its maild and remounted.
    let fail: (reason: string) => void = () => {};
    env.invoke.mockImplementation(async (command: string, args?: { config?: { lodge?: string } }) => {
      switch (command) {
        case "app_config_get":
          server.reads += 1;
          return server.lodge === null ? {} : { lodge: server.lodge };
        case "server_facts":
          return { flatpak: true, localData: true };
        case "app_config_set":
          server.lodge = args?.config?.lodge ?? null;
          return new Promise((_resolve, reject) => {
            fail = (reason) => {
              server.lodge = null;
              reject(reason);
            };
          });
        default:
          return null;
      }
    });
    useSettingsPane.setState({ pane: "server" });
    const first = render(<SettingsWindow offline />);
    await screen.findByRole("form", { name: "Server" });
    chooseLodge();
    fireEvent.click(save());
    await screen.findByRole("status");
    first.unmount();
    render(<SettingsWindow offline />);
    // The new pane read the lodge the shell wrote for the change.
    await waitFor(() => expect((lodgeField() as HTMLInputElement).value).toBe("https://lodge.example.ts.net"));
    act(() => fail("This computer's maild did not stop. Frostmail still uses it."));
    expect((await screen.findByRole("alert")).textContent).toBe(
      "This computer's maild did not stop. Frostmail still uses it.",
    );
    await waitFor(() =>
      expect((screen.getByRole("radio", { name: "maild on this computer" }) as HTMLInputElement).checked).toBe(true),
    );
    expect(screen.queryByRole("textbox", { name: "lodge:" })).toBeNull();
  });
});
