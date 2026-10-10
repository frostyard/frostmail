// CONTRACT TEST for task card T-0098 (docs/tasks). Do not edit.
import { render, screen, waitFor } from "@testing-library/react";
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
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Signatures" }));
  await screen.findByRole("list", { name: "Identities" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { mock, user, calls };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

const button = (name: string) => screen.getByRole("button", { name }) as HTMLButtonElement;

describe("adding and removing addresses", () => {
  it("adds an alias, selects it, and removes it after asking", async () => {
    const { user, calls } = await setup();
    expect(button("Remove Address").disabled).toBe(true);
    await user.click(button("Add Address"));
    await user.type(screen.getByLabelText("Email Address:"), "alias@mailtest.test");
    await user.click(button("Add"));
    await waitFor(() => expect(calls("identity.create")).toEqual([{ accountId: 1, email: "alias@mailtest.test" }]));
    const added = await screen.findByRole("button", { name: /alias@mailtest\.test/ });
    await waitFor(() => expect(added.getAttribute("aria-current")).toBe("true"));

    const confirm = vi.fn(() => true);
    vi.stubGlobal("confirm", confirm);
    await user.click(button("Remove Address"));
    expect(confirm).toHaveBeenCalledWith(
      "Remove alias@mailtest.test? Drafts from it will be sent from test1@mailtest.test.",
    );
    await waitFor(() => expect(calls("identity.delete")).toHaveLength(1));
    await waitFor(() => expect(screen.queryByRole("button", { name: /alias@mailtest\.test/ })).toBeNull());
  });

  it("keeps an address when removing is not confirmed", async () => {
    const { user, calls } = await setup();
    await user.click(button("Add Address"));
    await user.type(screen.getByLabelText("Email Address:"), "keep@mailtest.test");
    await user.type(screen.getByLabelText("Full Name:"), "Keeper");
    await user.click(button("Add"));
    await waitFor(() =>
      expect(calls("identity.create")).toEqual([{ accountId: 1, email: "keep@mailtest.test", name: "Keeper" }]),
    );
    await screen.findByRole("button", { name: /keep@mailtest\.test/ });
    vi.stubGlobal("confirm", () => false);
    await user.click(button("Remove Address"));
    expect(calls("identity.delete")).toHaveLength(0);
  });

  it("shows maild's refusal of an address the account has", async () => {
    const { user } = await setup();
    await user.click(button("Add Address"));
    await user.type(screen.getByLabelText("Email Address:"), "TEST1@mailtest.test");
    await user.click(button("Add"));
    expect((await screen.findByRole("alert")).textContent).toMatch(/already an address/);
    expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("Add Address");
  });
});
