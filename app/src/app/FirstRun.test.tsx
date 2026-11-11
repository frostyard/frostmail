// The first run of a store with no account (docs/specs/ui.md, First run): the
// welcome in place of the modules, and its action.
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { type MockData, MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";
import { useSettingsPane } from "./settingsPane";

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

const opens = () => vi.mocked(window.open).mock.calls.filter((call) => String(call[0]).endsWith("#/settings"));

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  vi.spyOn(window, "open").mockReturnValue(null);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("first run", () => {
  it("shows the welcome and its action, and opens nothing by itself", async () => {
    await setup();
    const section = await screen.findByRole("region", { name: "Welcome" });
    expect(within(section).getByRole("heading", { name: "Welcome to Frostmail" })).toBeTruthy();
    expect(within(section).getByText("Add a mail account to start.")).toBeTruthy();
    expect(within(section).getByRole("button", { name: "Add Account…" })).toBeTruthy();
    expect(within(section).queryByRole("button", { name: "Connect to lodge…" })).toBeNull();
    expect(opens()).toHaveLength(0);
  });

  it("opens the settings window on Accounts from Add Account", async () => {
    const user = userEvent.setup();
    await setup();
    const section = await screen.findByRole("region", { name: "Welcome" });
    useSettingsPane.setState({ pane: "general" });
    await user.click(within(section).getByRole("button", { name: "Add Account…" }));
    expect(opens()).toHaveLength(1);
    expect(useSettingsPane.getState().pane).toBe("accounts");
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
});
