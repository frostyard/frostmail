// Finishing a sign-in with the address of the page the browser could not
// open (ADR-0043): an edit pane shows the field while this window's
// sign-in waits, unless the desktop app is on its own maild.
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AccountForm, type AccountFormProps, type AccountFormValue } from "./AccountForm";

const env = vi.hoisted(() => ({ paste: true }));
vi.mock("../../app/settings", () => ({ pasteSignIn: () => env.paste }));

const gmail: AccountFormValue = {
  kind: "gmail",
  email: "ann@gmail.com",
  displayName: "Ann",
  auth: "oauth2",
  imap: { host: "imap.gmail.com", port: 993, tls: "tls", username: "ann@gmail.com" },
  smtp: { host: "smtp.gmail.com", port: 465, tls: "tls", username: "ann@gmail.com" },
  readOnly: false,
  notify: true,
  password: "",
};

function form(over: Partial<AccountFormProps> = {}) {
  const props: AccountFormProps = {
    mode: "edit",
    value: gmail,
    onChange: vi.fn(),
    discovery: { kind: "idle" },
    onDiscover: vi.fn(),
    signedIn: false,
    onSignIn: vi.fn(),
    signInWaiting: true,
    onFinishSignIn: vi.fn().mockResolvedValue(undefined),
    busy: false,
    error: null,
    onSubmit: vi.fn(),
    onCancel: vi.fn(),
    ...over,
  };
  render(<AccountForm {...props} />);
  return props;
}

beforeEach(() => {
  env.paste = true;
});

describe("finishing a sign-in with a pasted address", () => {
  it("shows the field while a remote client's sign-in waits, and sends the address", async () => {
    const props = form();
    expect(
      screen.getByText(
        "If your browser cannot open the page after you sign in, copy that page's address and paste it here.",
      ),
    ).toBeTruthy();
    const field = screen.getByLabelText("Address") as HTMLInputElement;
    expect([field.type, field.autocomplete, field.getAttribute("spellcheck")]).toEqual(["url", "off", "false"]);
    const button = screen.getByRole("button", { name: "Finish Sign-In" }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    const address = "http://localhost:39221/?code=c0de&state=s1";
    fireEvent.change(field, { target: { value: address } });
    expect(button.disabled).toBe(false);
    fireEvent.click(button);
    await waitFor(() => expect(props.onFinishSignIn).toHaveBeenCalledWith(address));
    // The row goes once the account is signed in; until then the address
    // stays, to correct or send again after an error.
    await waitFor(() => expect(button.disabled).toBe(false));
    expect(field.value).toBe(address);
  });

  it("stays away on the desktop app on its own maild", () => {
    env.paste = false;
    form();
    expect(screen.queryByText(/copy that page's address/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Finish Sign-In" })).toBeNull();
  });

  it("stays away for another account's sign-in", () => {
    form({ signInWaiting: false });
    expect(screen.queryByText(/copy that page's address/)).toBeNull();
  });
});
