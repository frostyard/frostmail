import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-07T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <SettingsWindow />
    </Session>,
  );
  await screen.findByRole("navigation", { name: "Accounts" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { mock, calls };
}

const email = () => screen.getByLabelText("Email Address:") as HTMLInputElement;

afterEach(() => {
  vi.unstubAllGlobals();
});

/** addAccount adds a password account through the form. */
async function addAccount(user: ReturnType<typeof userEvent.setup>, address: string) {
  await user.click(screen.getByRole("button", { name: "Add Account" }));
  await user.type(email(), address);
  await user.type(screen.getByLabelText("Password:"), "secret");
  const submit = screen
    .getAllByRole("button", { name: "Add Account" })
    .find((b) => b.getAttribute("type") === "submit");
  if (!submit) throw new Error("no submit button");
  await user.click(submit);
}

describe("SettingsWindow", () => {
  // account.create returns before the account list reloads on maild's
  // account.changed; the new account must stay selected, not give its
  // place in the form to the first account (which the user then edits).
  it("keeps a new account selected while the account list catches up", async () => {
    const user = userEvent.setup();
    const { calls } = await setup();
    await waitFor(() => expect(email().value).not.toBe(""));
    const first = email().value;
    const listsBefore = calls("account.list").length;
    await addAccount(user, "new@x.test");
    await waitFor(() => expect(calls("account.setPassword")).toHaveLength(1));
    await waitFor(() => expect(calls("account.list").length).toBeGreaterThan(listsBefore));
    const added = await screen.findByRole("button", { name: /new@x\.test/ });
    expect(added.getAttribute("aria-current")).toBe("true");
    expect(email().value).toBe("new@x.test");
    expect(email().value).not.toBe(first);
    const current = within(screen.getByRole("navigation", { name: "Accounts" }))
      .getAllByRole("button")
      .filter((b) => b.getAttribute("aria-current") === "true");
    expect(current).toEqual([added]);
  });

  it("moves to the first account when the selected one is removed", async () => {
    const user = userEvent.setup();
    await setup();
    await waitFor(() => expect(email().value).not.toBe(""));
    const first = email().value;
    await addAccount(user, "gone@x.test");
    await user.click(await screen.findByRole("button", { name: /gone@x\.test/ }));
    vi.stubGlobal("confirm", () => true);
    await user.click(screen.getByRole("button", { name: "Remove Account" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: /gone@x\.test/ })).toBeNull());
    await waitFor(() => expect(email().value).toBe(first));
  });
});
