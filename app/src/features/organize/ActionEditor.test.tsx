// CONTRACT TEST for task card T-0111 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Account, Mailbox, RuleAction } from "../../rpc/gen/api";
import { ActionEditor } from "./ActionEditor";

const account = (id: number, email: string): Account => ({
  id,
  kind: "imap",
  email,
  displayName: "",
  auth: "password",
  imap: { host: "h", port: 993, tls: "tls", username: email },
  smtp: { host: "h", port: 465, tls: "tls", username: email },
  createdAt: "2026-10-01T00:00:00Z",
  readOnly: false,
  notify: true,
  syncDays: 0,
  signedIn: true,
});

const mailbox = (id: number, accountId: number, path: string): Mailbox => ({
  id,
  accountId,
  path,
  name: path,
  delimiter: "/",
  role: "none",
  total: 0,
  unread: 0,
  label: false,
});

function editor(value: RuleAction[]) {
  const onChange = vi.fn();
  render(
    <ActionEditor
      value={value}
      onChange={onChange}
      accounts={[account(3, "me@x.test"), account(4, "you@y.test")]}
      mailboxes={[mailbox(10, 3, "INBOX"), mailbox(11, 3, "Receipts"), mailbox(20, 4, "INBOX")]}
      flagNames={["Urgent", "", "", "", "", "", ""]}
    />,
  );
  return onChange;
}

const select = (name: string) => screen.getByRole("combobox", { name }) as HTMLSelectElement;

describe("ActionEditor", () => {
  it("draws each row with what its kind takes", () => {
    editor([{ kind: "move", mailboxId: 11 }, { kind: "read" }, { kind: "flag", color: 2 }]);
    expect(screen.getByText("Perform the following actions:")).not.toBeNull();
    expect(Array.from(select("Action 1").options, (o) => o.textContent)).toEqual([
      "Move Message",
      "Copy Message",
      "Mark as Read",
      "Mark as Flagged",
      "Delete Message",
      "Send Notification",
      "Stop Evaluating Rules",
    ]);
    expect(select("Action 1").value).toBe("move");
    expect(screen.getByText("to mailbox:")).not.toBeNull();
    const mailboxes = select("Action 1 mailbox");
    expect(mailboxes.value).toBe("11");
    expect(Array.from(mailboxes.querySelectorAll("optgroup"), (g) => g.label)).toEqual(["me@x.test", "you@y.test"]);
    expect(Array.from(mailboxes.options, (o) => [o.value, o.textContent])).toEqual([
      ["10", "INBOX"],
      ["11", "Receipts"],
      ["20", "INBOX"],
    ]);
    expect(screen.queryByRole("combobox", { name: "Action 2 mailbox" })).toBeNull();
    expect(screen.queryByRole("combobox", { name: "Action 2 color" })).toBeNull();
    const colors = select("Action 3 color");
    expect(colors.value).toBe("2");
    expect(Array.from(colors.options, (o) => o.textContent)).toEqual([
      "Urgent",
      "Orange",
      "Yellow",
      "Green",
      "Blue",
      "Purple",
      "Gray",
    ]);
  });

  it("asks for a mailbox, and marks one that is gone", () => {
    editor([{ kind: "copy" }, { kind: "move", mailboxId: 99 }]);
    const none = select("Action 1 mailbox");
    expect(none.value).toBe("");
    expect(none.options[0]?.textContent).toBe("No Mailbox Selected");
    expect(none.options[0]?.disabled).toBe(true);
    const gone = select("Action 2 mailbox");
    expect(gone.value).toBe("99");
    expect(gone.options[0]?.textContent).toBe("Missing Mailbox");
    expect(gone.options[0]?.disabled).toBe(true);
  });

  it("reports each change as a whole new list", () => {
    const actions: RuleAction[] = [{ kind: "move", mailboxId: 11 }, { kind: "read" }];
    const onChange = editor(actions);
    fireEvent.change(select("Action 1"), { target: { value: "copy" } });
    expect(onChange).toHaveBeenLastCalledWith([{ kind: "copy", mailboxId: 11 }, { kind: "read" }]);
    fireEvent.change(select("Action 2"), { target: { value: "flag" } });
    expect(onChange).toHaveBeenLastCalledWith([
      { kind: "move", mailboxId: 11 },
      { kind: "flag", color: 1 },
    ]);
    fireEvent.change(select("Action 1 mailbox"), { target: { value: "20" } });
    expect(onChange).toHaveBeenLastCalledWith([{ kind: "move", mailboxId: 20 }, { kind: "read" }]);
    fireEvent.click(screen.getByRole("button", { name: "Add action after 1" }));
    expect(onChange).toHaveBeenLastCalledWith([{ kind: "move", mailboxId: 11 }, { kind: "move" }, { kind: "read" }]);
    fireEvent.click(screen.getByRole("button", { name: "Remove action 1" }));
    expect(onChange).toHaveBeenLastCalledWith([{ kind: "read" }]);
  });

  it("keeps the last row", () => {
    const onChange = editor([{ kind: "stop" }]);
    const remove = screen.getByRole("button", { name: "Remove action 1" }) as HTMLButtonElement;
    expect(remove.disabled).toBe(true);
    fireEvent.click(remove);
    expect(onChange).not.toHaveBeenCalled();
    expect(within(screen.getByRole("button", { name: "Add action after 1" })).queryByText(/./)).toBeNull();
  });
});
