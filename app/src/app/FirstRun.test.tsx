// The first run of a store with no account (docs/specs/ui.md, First run; plan
// 0013's Phase 6): the welcome in place of the modules, its two actions, and
// the settings window the main window opens on Accounts itself, once.
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { type MockData, MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

const env = vi.hoisted(() => ({ tauri: true, remote: false, invoke: vi.fn(), listen: vi.fn() }));

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
    setTitle: async () => {},
    show: async () => {},
  }),
}));
vi.mock("@tauri-apps/api/event", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/event")>()),
  emitTo: env.listen,
  listen: env.listen,
}));
vi.mock("../rpc/transport/tauri", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../rpc/transport/tauri")>()),
  remoteMaild: () => env.remote,
}));

/** EMPTY is a maild with no account at all. */
const EMPTY: MockData = { accounts: [], mailboxes: [], messages: [] };

const welcome = () => screen.queryByRole("region", { name: "Welcome" });

async function setup(data: MockData = EMPTY) {
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  return { mock };
}

const opens = () => env.invoke.mock.calls.filter((call) => call[0] === "open_settings");

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  env.tauri = true;
  env.remote = false;
  env.invoke.mockReset();
  env.invoke.mockImplementation(async () => null);
  env.listen.mockReset();
  env.listen.mockImplementation(async () => () => {});
});

describe("first run", () => {
  it("shows the welcome with both actions on this computer's maild", async () => {
    await setup();
    const section = await screen.findByRole("region", { name: "Welcome" });
    expect(within(section).getByRole("heading", { name: "Welcome to Frostmail" })).toBeTruthy();
    expect(
      within(section).getByText("Add a mail account to start. Frostmail keeps its mail on this computer."),
    ).toBeTruthy();
    expect(within(section).getByRole("button", { name: "Add Account…" })).toBeTruthy();
    expect(within(section).getByRole("button", { name: "Connect to lodge…" })).toBeTruthy();
  });

  it("opens the settings window on the pane each action names", async () => {
    const user = userEvent.setup();
    await setup();
    const section = await screen.findByRole("region", { name: "Welcome" });
    await waitFor(() => expect(opens()).toContainEqual(["open_settings", { pane: "accounts" }]));
    await user.click(within(section).getByRole("button", { name: "Add Account…" }));
    expect(opens()).toContainEqual(["open_settings", { pane: "accounts" }]);
    await user.click(within(section).getByRole("button", { name: "Connect to lodge…" }));
    await waitFor(() => expect(opens()).toContainEqual(["open_settings", { pane: "server" }]));
  });

  it("opens the settings window on Accounts once, and not again after its account goes", async () => {
    const { mock } = await setup();
    await screen.findByRole("region", { name: "Welcome" });
    await waitFor(() => expect(opens()).toHaveLength(1));
    const created = { kind: "imap", email: "ann@x.test", displayName: "Ann", auth: "password" };
    await act(() => mock.call("account.create", created));
    await waitFor(() => expect(welcome()).toBeNull());
    await act(() => mock.call("account.delete", { id: 1 }));
    await screen.findByRole("region", { name: "Welcome" });
    expect(opens()).toHaveLength(1);
  });

  it("opens nothing by itself when the last account of a store that had one goes", async () => {
    const { mock } = await setup(mockData({ inbox: 1, now: new Date("2026-10-08T12:00:00Z") }));
    await screen.findByRole("tree", { name: "Mailboxes" });
    const accounts = await mock.call<{ id: number }[]>("account.list", {});
    for (const account of accounts) await act(() => mock.call("account.delete", { id: account.id }));
    await screen.findByRole("region", { name: "Welcome" });
    expect(opens()).toHaveLength(0);
  });

  it("shows the modules and no welcome with an account", async () => {
    await setup(mockData({ inbox: 1, now: new Date("2026-10-08T12:00:00Z") }));
    await screen.findByRole("tree", { name: "Mailboxes" });
    expect(welcome()).toBeNull();
    expect(opens()).toHaveLength(0);
  });

  it("shows no lodge action on lodge's maild", async () => {
    env.remote = true;
    await setup();
    const section = await screen.findByRole("region", { name: "Welcome" });
    expect(within(section).getByText("Add a mail account to start.")).toBeTruthy();
    expect(within(section).queryByRole("button", { name: "Connect to lodge…" })).toBeNull();
  });

  it("shows no lodge action in a browser, and opens nothing by itself", async () => {
    env.tauri = false;
    await setup();
    const section = await screen.findByRole("region", { name: "Welcome" });
    expect(within(section).getByText("Add a mail account to start.")).toBeTruthy();
    expect(within(section).getByRole("button", { name: "Add Account…" })).toBeTruthy();
    expect(within(section).queryByRole("button", { name: "Connect to lodge…" })).toBeNull();
    expect(opens()).toHaveLength(0);
  });
});
