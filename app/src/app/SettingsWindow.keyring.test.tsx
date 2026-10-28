// Adding an account shows maild's own words for the failure. Only the
// sign-in step means a missing OAuth client: it returns unavailable when no
// Google client is set up (docs/specs/settings-ui.md, Accounts pane).
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { ErrorCode } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { RPCError } from "../rpc/transport";
import { SettingsWindow } from "./SettingsWindow";

const NO_CLIENT = "Set up your Google client under Sign-In first.";
const KEYRING = "no account was added: the secret store cannot keep a secret: the prompt was dismissed";

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
  return { mock, open };
}

/** failOne makes one method reject with err; the others answer as before. */
function failOne(mock: MockTransport, method: string, err: unknown) {
  const call = mock.call.bind(mock);
  vi.spyOn(mock, "call").mockImplementation((asked: string, params: unknown) =>
    asked === method ? Promise.reject(err) : call(asked, params),
  );
}

const email = () => screen.getByLabelText("Email Address:") as HTMLInputElement;

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

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

describe("adding an account maild refuses", () => {
  it("shows the secret store's message when no account can be added", async () => {
    const user = userEvent.setup();
    const { mock } = await setup();
    failOne(mock, "account.create", new RPCError(ErrorCode.unavailable, KEYRING));
    await startAdding(user, "new@x.test");
    await user.type(screen.getByLabelText("Password:"), "secret");
    await submit(user);
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(KEYRING);
    expect(screen.queryByText(NO_CLIENT)).toBeNull();
    // No account was added: the form keeps the address, and no step after
    // create ran.
    expect(email().value).toBe("new@x.test");
    expect(screen.queryByRole("button", { name: /new@x\.test/ })).toBeNull();
    const sent = mock.calls.map((c) => c.method);
    expect(sent).not.toContain("account.setPassword");
    expect(sent).not.toContain("account.setService");
  });

  it("says to set up the Google client when the sign-in step cannot start", async () => {
    const user = userEvent.setup();
    const { mock, open } = await setup();
    failOne(mock, "account.authorize", new RPCError(ErrorCode.unavailable, "no OAuth client is set up"));
    await startAdding(user, "new@gmail.test");
    await user.selectOptions(screen.getByLabelText("Account Type:"), "gmail");
    await user.selectOptions(screen.getByLabelText("Sign In:"), "oauth2");
    await submit(user);
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(NO_CLIENT);
    expect(screen.queryByText(/no OAuth client is set up/)).toBeNull();
    expect(open).not.toHaveBeenCalled();
  });
});
