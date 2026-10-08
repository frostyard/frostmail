// CONTRACT TEST for task card T-0057 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { AccountForm, type AccountFormProps, type AccountFormValue } from "./AccountForm";

const value: AccountFormValue = {
  kind: "imap",
  email: "ann@x.test",
  displayName: "Ann Example",
  auth: "password",
  imap: { host: "imap.x.test", port: 993, tls: "tls", username: "ann" },
  smtp: { host: "smtp.x.test", port: 465, tls: "tls", username: "ann" },
  readOnly: false,
  notify: true,
  password: "",
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

function field(label: string): HTMLInputElement {
  return screen.getByLabelText(label) as HTMLInputElement;
}

function button(name: string): HTMLButtonElement {
  return screen.getByRole("button", { name }) as HTMLButtonElement;
}

describe("AccountForm", () => {
  it("shows a new account's fields and both servers", () => {
    form();
    expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("Add Account");
    expect(field("Email Address:").value).toBe("ann@x.test");
    expect(field("Email Address:").type).toBe("email");
    expect(field("Email Address:").readOnly).toBe(false);
    expect(field("Full Name:").value).toBe("Ann Example");
    const kind = screen.getByLabelText("Account Type:") as HTMLSelectElement;
    expect(Array.from(kind.options).map((o) => [o.value, o.text])).toEqual([
      ["imap", "IMAP"],
      ["gmail", "Gmail"],
      ["icloud", "iCloud"],
    ]);
    expect(kind.disabled).toBe(false);
    const auth = screen.getByLabelText("Sign In:") as HTMLSelectElement;
    expect(Array.from(auth.options).map((o) => o.value)).toEqual(["password"]);
    expect(field("Password:").type).toBe("password");
    expect(field("Password:").autocomplete).toBe("new-password");
    expect(screen.getByRole("group", { name: "Incoming Mail (IMAP)" })).toBeTruthy();
    expect(screen.getByRole("group", { name: "Outgoing Mail (SMTP)" })).toBeTruthy();
    expect((document.getElementById("smtp-host") as HTMLInputElement).value).toBe("smtp.x.test");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("reports edits as new values", () => {
    const { onChange } = form();
    fireEvent.change(field("Full Name:"), { target: { value: "Ann E." } });
    expect(onChange).toHaveBeenLastCalledWith({ ...value, displayName: "Ann E." });
    fireEvent.change(field("Password:"), { target: { value: "s3cret" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...value, password: "s3cret" });
    fireEvent.change(document.getElementById("smtp-host") as HTMLInputElement, { target: { value: "mail.x.test" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...value, smtp: { ...value.smtp, host: "mail.x.test" } });
    fireEvent.click(screen.getByLabelText("Read only: never change anything on the server"));
    expect(onChange).toHaveBeenLastCalledWith({ ...value, readOnly: true });
    fireEvent.click(screen.getByLabelText("Notify me about new mail"));
    expect(onChange).toHaveBeenLastCalledWith({ ...value, notify: false });
  });

  it("finds settings for a complete address and says what it found", () => {
    const { onDiscover } = form({ discovery: { kind: "found", source: "autoconfig" } });
    fireEvent.click(button("Find Settings"));
    expect(onDiscover).toHaveBeenCalledTimes(1);
    expect(screen.getByText("Found settings for this address.").className).toContain("text-secondary");
  });

  it("cannot look up an incomplete address or while looking", () => {
    form({ value: { ...value, email: "ann@" } });
    expect(button("Find Settings").disabled).toBe(true);
  });

  it("shows the lookup in progress", () => {
    form({ discovery: { kind: "finding" } });
    expect(button("Find Settings").disabled).toBe(true);
    expect(screen.getByText("Looking up servers…")).toBeTruthy();
  });

  it("asks for the servers when nothing was found", () => {
    form({ discovery: { kind: "found", source: "none" } });
    expect(screen.getByText("No settings found; enter the servers below.")).toBeTruthy();
  });

  it("shows a failed lookup as an error", () => {
    form({ discovery: { kind: "failed", message: "lookup timed out" } });
    expect(screen.getByText("lookup timed out").className).toContain("text-flag-1");
  });

  it("offers Google sign-in for Gmail and app-password hints", () => {
    form({ value: { ...value, kind: "gmail" } });
    const auth = screen.getByLabelText("Sign In:") as HTMLSelectElement;
    expect(Array.from(auth.options).map((o) => [o.value, o.text])).toEqual([
      ["password", "Password"],
      ["oauth2", "Google sign-in"],
    ]);
    expect(screen.getByText("Use an app password from your Google account.").className).toContain("text-secondary");
  });

  it("hints at iCloud's app-specific passwords", () => {
    form({ value: { ...value, kind: "icloud" } });
    expect(screen.getByText("Use an app-specific password from appleid.apple.com.")).toBeTruthy();
  });

  it("drops Google sign-in when the kind changes away from Gmail", () => {
    const gmail = { ...value, kind: "gmail" as const, auth: "oauth2" as const };
    const { onChange } = form({ value: gmail });
    fireEvent.change(screen.getByLabelText("Account Type:"), { target: { value: "icloud" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...gmail, kind: "icloud", auth: "password" });
  });

  it("changes the kind and the sign-in", () => {
    const { onChange } = form();
    fireEvent.change(screen.getByLabelText("Account Type:"), { target: { value: "gmail" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...value, kind: "gmail" });
    fireEvent.change(screen.getByLabelText("Sign In:"), { target: { value: "password" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...value, auth: "password" });
  });

  it("explains Google sign-in when adding", () => {
    form({ value: { ...value, kind: "gmail", auth: "oauth2" } });
    expect(screen.queryByLabelText("Password:")).toBeNull();
    expect(screen.getByText("After adding the account, sign in with Google in your browser.")).toBeTruthy();
  });

  it("adds or cancels", () => {
    const { onSubmit, onCancel } = form();
    fireEvent.click(button("Add Account"));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(button("Add Account").type).toBe("submit");
    expect(button("Add Account").className).toContain("bg-accent");
    fireEvent.click(button("Cancel"));
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("edits an account without changing its address, kind or sign-in", () => {
    form({ mode: "edit", value: { ...value, displayName: " " } });
    expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("ann@x.test");
    expect(field("Email Address:").readOnly).toBe(true);
    expect((screen.getByLabelText("Account Type:") as HTMLSelectElement).disabled).toBe(true);
    expect((screen.getByLabelText("Sign In:") as HTMLSelectElement).disabled).toBe(true);
    expect(field("Password:").placeholder).toBe("Unchanged");
    expect(screen.queryByRole("button", { name: "Find Settings" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
    expect(button("Save").type).toBe("submit");
  });

  it("signs a Google account in again", () => {
    const { onSignIn } = form({ mode: "edit", value: { ...value, kind: "gmail", auth: "oauth2" } });
    expect(screen.getByText("Signed in").className).toContain("text-secondary");
    fireEvent.click(button("Sign In Again…"));
    expect(onSignIn).toHaveBeenCalledTimes(1);
  });

  it("signs in a Google account that is not signed in", () => {
    form({ mode: "edit", signedIn: false, value: { ...value, kind: "gmail", auth: "oauth2" } });
    expect(screen.getByText("Not signed in").className).toContain("text-flag-1");
    expect(button("Sign In…")).toBeTruthy();
  });

  it("waits while busy and shows the last error", () => {
    form({ mode: "edit", busy: true, error: "the server refused the password" });
    expect(button("Save").disabled).toBe(true);
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toBe("the server refused the password");
    expect(alert.className).toContain("text-flag-1");
    expect((screen.getByRole("group", { name: "Incoming Mail (IMAP)" }) as HTMLFieldSetElement).disabled).toBe(true);
  });

  it("submits the form with Enter", () => {
    const { onSubmit } = form({ mode: "edit" });
    const el = screen.getByRole("button", { name: "Save" }).closest("form");
    if (!el) throw new Error("no form");
    fireEvent.submit(el);
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(within(el).getByLabelText("Full Name:")).toBeTruthy();
  });
});
