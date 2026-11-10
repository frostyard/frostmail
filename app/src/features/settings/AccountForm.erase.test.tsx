// The account form's Mailbox Behaviors (docs/specs/settings-ui.md,
// AccountForm; ADR-0051, plan 0014 Phase 4): how long maild keeps the
// account's deleted and junk mail.
import { fireEvent, render, screen } from "@testing-library/react";
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
  syncDays: 0,
  eraseTrashDays: 0,
  eraseJunkDays: 0,
  password: "",
};

const NAMED: AccountFormProps["eraseMailboxes"] = { trash: "Trash", junk: "Junk" };

function form(over: Partial<AccountFormProps> = {}) {
  const props: AccountFormProps = {
    mode: "edit",
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
    eraseMailboxes: NAMED,
    ...over,
  };
  render(<AccountForm {...props} />);
  return props;
}

function trash(): HTMLSelectElement {
  return screen.getByLabelText("Erase deleted messages:") as HTMLSelectElement;
}

function junk(): HTMLSelectElement {
  return screen.getByLabelText("Erase junk messages:") as HTMLSelectElement;
}

function chosen(select: HTMLSelectElement): string {
  return select.options[select.selectedIndex]?.text ?? "";
}

describe("the account form's mailbox behaviors", () => {
  it("shows the section when editing, named for what it does", () => {
    form();
    expect(screen.getByRole("heading", { level: 3, name: "Mailbox Behaviors" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "Mailbox Behaviors" })).toBeTruthy();
    expect(trash().className).toContain("border-separator");
  });

  it("is absent when adding an account, which erases nothing", () => {
    form({ mode: "add" });
    expect(screen.queryByRole("heading", { level: 3, name: "Mailbox Behaviors" })).toBeNull();
    expect(screen.queryByLabelText("Erase deleted messages:")).toBeNull();
    expect(screen.queryByLabelText("Erase junk messages:")).toBeNull();
  });

  it("offers Never, a day, a week and a month, in that order", () => {
    form();
    expect(Array.from(trash().options).map((o) => [o.value, o.text])).toEqual([
      ["0", "Never"],
      ["1", "After One Day"],
      ["7", "After One Week"],
      ["30", "After One Month"],
    ]);
    expect(Array.from(junk().options).map((o) => [o.value, o.text])).toEqual([
      ["0", "Never"],
      ["1", "After One Day"],
      ["7", "After One Week"],
      ["30", "After One Month"],
    ]);
  });

  it("shows the stored behaviors", () => {
    form({ value: { ...value, eraseTrashDays: 7, eraseJunkDays: 30 } });
    expect(chosen(trash())).toBe("After One Week");
    expect(chosen(junk())).toBe("After One Month");
  });

  it("reports each chosen behavior", () => {
    const { onChange } = form();
    fireEvent.change(trash(), { target: { value: "7" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...value, eraseTrashDays: 7 });
    fireEvent.change(junk(), { target: { value: "1" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...value, eraseJunkDays: 1 });
  });

  it("waits while busy", () => {
    form({ busy: true });
    expect(trash().disabled).toBe(true);
    expect(junk().disabled).toBe(true);
  });

  it("waits while the account is read only, which maild never erases", () => {
    form({ value: { ...value, eraseTrashDays: 7, readOnly: true } });
    expect(trash().disabled).toBe(true);
    expect(junk().disabled).toBe(true);
  });

  it("does not offer a behavior the account has no mailbox for, and says Never", () => {
    form({ value: { ...value, eraseJunkDays: 30 }, eraseMailboxes: { trash: "Trash" } });
    expect(trash().disabled).toBe(false);
    expect(junk().disabled).toBe(true);
    expect(chosen(junk())).toBe("Never");
  });

  it("names the mailboxes it erases", () => {
    form();
    expect(
      screen.getByText(
        "Messages are erased from the server after they have been in “Trash” or “Junk” that long, and cannot be recovered. Mail older than this account's sync window is left on the server.",
      ).className,
    ).toContain("text-secondary");
  });

  it("names the one mailbox it has", () => {
    form({ eraseMailboxes: { junk: "Spam" } });
    expect(
      screen.getByText(
        "Messages are erased from the server after they have been in “Spam” that long, and cannot be recovered. Mail older than this account's sync window is left on the server.",
      ),
    ).toBeTruthy();
  });

  it("explains nothing when the account has neither mailbox", () => {
    form({ eraseMailboxes: {} });
    expect(screen.queryByText(/cannot be recovered/)).toBeNull();
    expect(trash().disabled).toBe(true);
    expect(junk().disabled).toBe(true);
  });
});
