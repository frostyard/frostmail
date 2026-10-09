// CONTRACT TEST for task card T-0093 (docs/tasks). Do not edit.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-07T12:00:00Z") }));
  const open = vi.fn();
  vi.stubGlobal("open", open);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <SettingsWindow />
    </Session>,
  );
  await screen.findByRole("navigation", { name: "Accounts" });
  // The requests that change the new account, in the order sent.
  const writes = () =>
    mock.calls
      .filter((c) =>
        ["account.create", "account.setPassword", "account.setService", "account.authorize"].includes(c.method),
      )
      .map((c) => {
        const p = c.params as { service?: string };
        return c.method === "account.setService" ? `setService ${p.service}` : c.method.replace("account.", "");
      });
  return { mock, open, writes };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

const email = () => screen.getByLabelText("Email Address:") as HTMLInputElement;
const services = () => screen.getByRole("region", { name: "Contacts, Calendars and Tasks" });
const box = (name: string) => within(services()).getByRole("checkbox", { name }) as HTMLInputElement;

async function startAdding(user: ReturnType<typeof userEvent.setup>, address: string) {
  await user.click(screen.getByRole("button", { name: "Add Account" }));
  await user.type(email(), address);
}

async function submit(user: ReturnType<typeof userEvent.setup>) {
  const button = screen
    .getAllByRole("button", { name: "Add Account" })
    .find((b) => b.getAttribute("type") === "submit");
  if (!button) throw new Error("no submit button");
  await user.click(button);
}

describe("adding an account with its services", () => {
  it("signs in to Google once, for mail and the services checked", async () => {
    const user = userEvent.setup();
    const { open, writes } = await setup();
    await startAdding(user, "new@gmail.test");
    await user.selectOptions(screen.getByLabelText("Account Type:"), "gmail");
    await user.selectOptions(screen.getByLabelText("Sign In:"), "oauth2");
    expect([box("Contacts").checked, box("Calendars").checked, box("Tasks").checked]).toEqual([true, true, true]);
    await user.click(box("Tasks"));
    await submit(user);
    await waitFor(() =>
      expect(writes()).toEqual(["create", "setService contacts", "setService calendar", "authorize"]),
    );
    expect(open).toHaveBeenCalledTimes(1);
    expect(String(open.mock.calls[0]?.[0])).toMatch(/^https:\/\//);
    await waitFor(() => expect(email().value).toBe("new@gmail.test"));
    await waitFor(() => expect(box("Contacts").checked).toBe(true));
    expect([box("Calendars").checked, box("Tasks").checked]).toEqual([true, false]);
    expect(screen.queryByText(/^Sign in to Google to allow/)).toBeNull();
  });

  it("turns a password account's services on after its password", async () => {
    const user = userEvent.setup();
    const { open, writes } = await setup();
    await startAdding(user, "ann@icloud.test");
    await user.selectOptions(screen.getByLabelText("Account Type:"), "icloud");
    await user.type(screen.getByLabelText("Password:"), "app-specific");
    expect(within(services()).queryByRole("checkbox", { name: "Tasks" })).toBeNull();
    await submit(user);
    await waitFor(() =>
      expect(writes()).toEqual(["create", "setPassword", "setService contacts", "setService calendar"]),
    );
    expect(open).not.toHaveBeenCalled();
  });

  it("adds an IMAP account with no services unless asked", async () => {
    const user = userEvent.setup();
    const { writes } = await setup();
    await startAdding(user, "plain@x.test");
    await user.type(screen.getByLabelText("Password:"), "secret");
    expect([box("Contacts").checked, box("Calendars").checked, box("Tasks").checked]).toEqual([false, false, false]);
    await submit(user);
    await waitFor(() => expect(writes()).toEqual(["create", "setPassword"]));
  });

  it("keeps the account when a service cannot be found, and says which", async () => {
    const user = userEvent.setup();
    const { writes } = await setup();
    await startAdding(user, "plain@x.test");
    await user.type(screen.getByLabelText("Password:"), "secret");
    await user.click(box("Contacts"));
    await user.click(box("Calendars"));
    await submit(user);
    await waitFor(() =>
      expect(writes()).toEqual(["create", "setPassword", "setService contacts", "setService calendar"]),
    );
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/^Could not turn on Contacts: .*enter its address/);
    expect(email().value).toBe("plain@x.test");
    const added = await screen.findByRole("button", { name: /plain@x\.test/ });
    expect(added.getAttribute("aria-current")).toBe("true");
  });

  it("checks the found kind's services after Find Settings", async () => {
    const user = userEvent.setup();
    await setup();
    await startAdding(user, "found@gmail.com");
    await user.click(screen.getByRole("button", { name: "Find Settings" }));
    await waitFor(() => expect((screen.getByLabelText("Account Type:") as HTMLSelectElement).value).toBe("gmail"));
    expect([box("Contacts").checked, box("Calendars").checked, box("Tasks").checked]).toEqual([true, true, true]);
  });
});
