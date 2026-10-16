// Microsoft accounts in the account form (plan 0010, Phase 2): Outlook.com
// signs in only with Microsoft, in the browser, and offers no services
// until Graph's contacts and calendars land.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { AccountForm, type AccountFormProps, type AccountFormValue } from "./AccountForm";

const value: AccountFormValue = {
  kind: "imap",
  email: "cy@hotmail.com",
  displayName: "Cy",
  auth: "password",
  imap: { host: "", port: 993, tls: "tls", username: "" },
  smtp: { host: "", port: 587, tls: "starttls", username: "" },
  readOnly: false,
  notify: true,
  password: "",
};

const microsoft: AccountFormValue = {
  ...value,
  kind: "microsoft",
  auth: "oauth2",
  imap: { host: "outlook.office365.com", port: 993, tls: "tls", username: "cy@hotmail.com" },
  smtp: { host: "smtp-mail.outlook.com", port: 587, tls: "starttls", username: "cy@hotmail.com" },
  services: [],
};

function form(over: Partial<AccountFormProps> = {}) {
  const props: AccountFormProps = {
    mode: "add",
    value,
    onChange: vi.fn(),
    discovery: { kind: "idle" },
    onDiscover: vi.fn(),
    signedIn: true,
    onSignIn: vi.fn(),
    busy: false,
    error: null,
    onSubmit: vi.fn(),
    onCancel: vi.fn(),
    ...over,
  };
  render(<AccountForm {...props} />);
  return props;
}

describe("Microsoft accounts", () => {
  it("switch to Microsoft sign-in with no services", () => {
    const { onChange } = form();
    fireEvent.change(screen.getByLabelText("Account Type:"), { target: { value: "microsoft" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...value, kind: "microsoft", auth: "oauth2", services: [] });
  });

  it("offer Microsoft sign-in only, and explain it when adding", () => {
    form({ value: microsoft });
    const auth = screen.getByLabelText("Sign In:") as HTMLSelectElement;
    expect(Array.from(auth.options).map((o) => [o.value, o.text])).toEqual([["oauth2", "Microsoft sign-in"]]);
    expect(screen.queryByLabelText("Password:")).toBeNull();
    expect(screen.getByText("Microsoft:")).toBeTruthy();
    expect(screen.getByText("After adding the account, sign in with Microsoft in your browser.")).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Contacts, Calendars and Tasks" })).toBeNull();
  });

  it("go back to a password when the kind changes away", () => {
    const { onChange } = form({ value: microsoft });
    fireEvent.change(screen.getByLabelText("Account Type:"), { target: { value: "imap" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...microsoft, kind: "imap", auth: "password", services: [] });
  });

  it("sign in again from the edit pane", () => {
    const { onSignIn } = form({ mode: "edit", signedIn: false, value: microsoft });
    expect(screen.getByText("Not signed in").className).toContain("text-flag-1");
    fireEvent.click(screen.getByRole("button", { name: "Sign In…" }));
    expect(onSignIn).toHaveBeenCalledTimes(1);
  });
});
