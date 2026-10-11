// CONTRACT TEST for task card T-0111 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Account, Mailbox } from "../../rpc/gen/api";
import { newRuleDraft, type RuleDraft, RuleSheet, type RuleSheetProps } from "./RuleSheet";

const account: Account = {
  id: 3,
  kind: "imap",
  email: "me@x.test",
  displayName: "",
  auth: "password",
  imap: { host: "h", port: 993, tls: "tls", username: "me@x.test" },
  smtp: { host: "h", port: 465, tls: "tls", username: "me@x.test" },
  createdAt: "2026-10-01T00:00:00Z",
  readOnly: false,
  notify: true,
  syncDays: 0,
  signedIn: true,
};

const receipts: Mailbox = {
  id: 11,
  accountId: 3,
  path: "Receipts",
  name: "Receipts",
  delimiter: "/",
  role: "none",
  total: 0,
  unread: 0,
  label: false,
};

function sheet(over: Partial<RuleSheetProps> = {}) {
  const props: RuleSheetProps = {
    title: "New Rule",
    initial: newRuleDraft(3),
    accounts: [account],
    mailboxes: [receipts],
    today: "2026-10-11",
    onSave: vi.fn(),
    onCancel: vi.fn(),
    ...over,
  };
  render(<RuleSheet {...props} />);
  return props;
}

const description = () => screen.getByLabelText("Description:") as HTMLInputElement;
const ok = () => screen.getByRole("button", { name: "OK" }) as HTMLButtonElement;

describe("RuleSheet", () => {
  it("starts a new rule as Mail.app does", () => {
    expect(newRuleDraft(3)).toEqual({
      name: "Rule 3",
      conditions: { match: "any", conditions: [{ field: "from", op: "contains", value: "" }] },
      actions: [{ kind: "move" }],
    });
    sheet();
    const dialog = screen.getByRole("dialog", { name: "New Rule" });
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(description().value).toBe("Rule 3");
    expect(description().maxLength).toBe(100);
    expect(document.activeElement).toBe(description());
    expect(dialog.textContent).toContain("If");
    expect(dialog.textContent).toContain("of the following conditions are met:");
    expect(dialog.textContent).not.toContain("Contains messages that match");
    expect(dialog.textContent).toContain("Perform the following actions:");
    // A Move Message with no mailbox cannot be saved.
    expect(ok().disabled).toBe(true);
    fireEvent.change(screen.getByRole("combobox", { name: "Action 1 mailbox" }), { target: { value: "11" } });
    expect(ok().disabled).toBe(false);
  });

  it("saves the description, the conditions and the actions", () => {
    const initial: RuleDraft = {
      name: "Receipts",
      conditions: { match: "all", conditions: [{ field: "subject", op: "contains", value: "receipt" }] },
      actions: [{ kind: "move", mailboxId: 11 }],
    };
    const props = sheet({ title: "Edit Rule", initial });
    fireEvent.change(description(), { target: { value: " Shop receipts " } });
    fireEvent.change(screen.getByRole("textbox", { name: "Condition 1 value" }), { target: { value: "invoice" } });
    fireEvent.click(screen.getByRole("button", { name: "Add action after 1" }));
    fireEvent.change(screen.getByRole("combobox", { name: "Action 2" }), { target: { value: "read" } });
    fireEvent.click(ok());
    expect(props.onSave).toHaveBeenCalledWith({
      name: "Shop receipts",
      conditions: { match: "all", conditions: [{ field: "subject", op: "contains", value: "invoice" }] },
      actions: [{ kind: "move", mailboxId: 11 }, { kind: "read" }],
    });
  });

  it("needs a mailbox that exists, and shows maild's error", () => {
    const gone: RuleDraft = { ...newRuleDraft(1), actions: [{ kind: "copy", mailboxId: 99 }] };
    sheet({ initial: gone, error: "action 1: mailbox 99 does not exist" });
    expect(ok().disabled).toBe(true);
    expect(screen.getByRole("alert").textContent).toContain("mailbox 99 does not exist");
  });

  it("waits while busy", () => {
    const ready: RuleDraft = { ...newRuleDraft(1), actions: [{ kind: "read" }] };
    sheet({ initial: ready, busy: true });
    expect(ok().disabled).toBe(true);
  });

  it("will not save a blank description", () => {
    const ready: RuleDraft = { ...newRuleDraft(1), actions: [{ kind: "read" }] };
    sheet({ initial: ready });
    expect(ok().disabled).toBe(false);
    fireEvent.change(description(), { target: { value: "   " } });
    expect(ok().disabled).toBe(true);
  });

  it("cancels with Cancel or Escape", () => {
    const props = sheet();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(props.onCancel).toHaveBeenCalledTimes(2);
    expect(props.onSave).not.toHaveBeenCalled();
  });
});
