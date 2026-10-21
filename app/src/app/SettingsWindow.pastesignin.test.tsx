// Finishing a sign-in with a pasted address (ADR-0043): the field shows
// while this window's sign-in waits, also for an account that is already
// signed in and signs in again, and goes when the sign-in finishes.
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

const HINT = /copy that page's address and paste it here/;

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-21T12:00:00Z") }));
  vi.stubGlobal("open", vi.fn());
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <SettingsWindow />
    </Session>,
  );
  await screen.findByRole("navigation", { name: "Accounts" });
  return { mock };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

async function addGmail(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Add Account" }));
  await user.type(screen.getByLabelText("Email Address:"), "new@gmail.test");
  await user.selectOptions(screen.getByLabelText("Account Type:"), "gmail");
  await user.selectOptions(screen.getByLabelText("Sign In:"), "oauth2");
  const submit = screen
    .getAllByRole("button", { name: "Add Account" })
    .find((b) => b.getAttribute("type") === "submit");
  if (!submit) throw new Error("no submit button");
  await user.click(submit);
  await screen.findByRole("button", { name: "Sign In Again…" });
}

describe("finishing a sign-in with a pasted address", () => {
  it("hides the field once a new account's sign-in finishes", async () => {
    const user = userEvent.setup();
    await setup();
    await addGmail(user);
    await waitFor(() => expect(screen.queryByText(HINT)).toBeNull());
  });

  it("shows the field when a signed-in account signs in again, and finishes with the address", async () => {
    const user = userEvent.setup();
    const { mock } = await setup();
    await addGmail(user);
    await user.click(screen.getByRole("button", { name: "Sign In Again…" }));
    expect(await screen.findByText(HINT)).toBeTruthy();
    const address = "http://127.0.0.1:39221/?code=c0de&state=s1";
    await user.type(screen.getByLabelText("Address"), address);
    await user.click(screen.getByRole("button", { name: "Finish Sign-In" }));
    await waitFor(() =>
      expect(mock.calls.filter((c) => c.method === "account.finishSignIn").map((c) => c.params)).toEqual([
        { url: address },
      ]),
    );
    await waitFor(() => expect(screen.queryByText(HINT)).toBeNull());
  });
});
