// CONTRACT TEST for task card T-0107 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Account, Conditions, Mailbox } from "../../rpc/gen/api";
import { ConditionEditor, type ConditionEditorProps } from "./ConditionEditor";

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

function editor(value: Conditions, over: Partial<ConditionEditorProps> = {}) {
  const onChange = vi.fn();
  render(
    <ConditionEditor
      value={value}
      onChange={onChange}
      accounts={[account(3, "me@x.test"), account(4, "you@y.test")]}
      mailboxes={[mailbox(10, 3, "INBOX"), mailbox(11, 3, "Lists"), mailbox(20, 4, "INBOX")]}
      flagNames={["Urgent", "", "", "", "", "", ""]}
      today="2026-10-11"
      {...over}
    />,
  );
  return onChange;
}

const all = (...conditions: Conditions["conditions"]): Conditions => ({ match: "all", conditions });
const select = (name: string) => screen.getByRole("combobox", { name }) as HTMLSelectElement;
const lastChange = (onChange: ReturnType<typeof vi.fn>) => onChange.mock.lastCall?.[0] as Conditions;

describe("ConditionEditor", () => {
  it("shows the match and a row per condition", () => {
    editor(all({ field: "from", op: "contains", value: "ann" }, { field: "unread", op: "is", value: "false" }));
    expect(select("Match").value).toBe("all");
    expect(Array.from(select("Match").options).map((o) => o.textContent)).toEqual(["all", "any"]);
    expect(select("Condition 1 field").value).toBe("from");
    expect(select("Condition 1 field").options[3]?.textContent).toBe("Any Recipient");
    expect(select("Condition 1 operator").value).toBe("contains");
    expect((screen.getByRole("textbox", { name: "Condition 1 value" }) as HTMLInputElement).value).toBe("ann");
    expect(select("Condition 2 field").value).toBe("unread");
    expect(screen.queryByRole("combobox", { name: "Condition 2 operator" })).toBeNull();
    expect(select("Condition 2 value").value).toBe("false");
    expect(Array.from(select("Condition 2 value").options).map((o) => o.textContent)).toEqual(["Yes", "No"]);
  });

  it("reports every edit as whole conditions", () => {
    const value = all({ field: "from", op: "contains", value: "ann" });
    const onChange = editor(value);
    fireEvent.change(select("Match"), { target: { value: "any" } });
    expect(lastChange(onChange)).toEqual({ match: "any", conditions: value.conditions });
    fireEvent.change(screen.getByRole("textbox", { name: "Condition 1 value" }), { target: { value: "bob" } });
    expect(lastChange(onChange)).toEqual(all({ field: "from", op: "contains", value: "bob" }));
    fireEvent.change(select("Condition 1 operator"), { target: { value: "ends" } });
    expect(lastChange(onChange)).toEqual(all({ field: "from", op: "ends", value: "ann" }));
    fireEvent.change(select("Condition 1 field"), { target: { value: "received" } });
    expect(lastChange(onChange)).toEqual(all({ field: "received", op: "today", value: "" }));
  });

  it("adds after a row and removes a row, keeping one", () => {
    const two = all({ field: "from", op: "contains", value: "a" }, { field: "subject", op: "is", value: "b" });
    const onChange = editor(two);
    fireEvent.click(screen.getByRole("button", { name: "Add condition after 1" }));
    expect(lastChange(onChange).conditions).toEqual([
      { field: "from", op: "contains", value: "a" },
      { field: "from", op: "contains", value: "" },
      { field: "subject", op: "is", value: "b" },
    ]);
    fireEvent.click(screen.getByRole("button", { name: "Remove condition 1" }));
    expect(lastChange(onChange).conditions).toEqual([{ field: "subject", op: "is", value: "b" }]);
  });

  it("will not remove the only row", () => {
    editor(all({ field: "from", op: "contains", value: "a" }));
    expect((screen.getByRole("button", { name: "Remove condition 1" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("edits a duration as a number and a unit", () => {
    const onChange = editor(all({ field: "received", op: "within", value: "7d" }));
    const n = screen.getByRole("spinbutton", { name: "Condition 1 value" }) as HTMLInputElement;
    expect(n.value).toBe("7");
    expect(select("Condition 1 unit").value).toBe("d");
    expect(Array.from(select("Condition 1 unit").options).map((o) => o.textContent)).toEqual([
      "Days",
      "Weeks",
      "Months",
      "Years",
    ]);
    fireEvent.change(n, { target: { value: "14" } });
    expect(lastChange(onChange).conditions[0]?.value).toBe("14d");
    fireEvent.change(select("Condition 1 unit"), { target: { value: "w" } });
    expect(lastChange(onChange).conditions[0]?.value).toBe("7w");
  });

  it("edits a day with a date input and shows nothing for today", () => {
    editor(all({ field: "sent", op: "before", value: "2026-09-01" }, { field: "received", op: "today", value: "" }));
    expect((screen.getByLabelText("Condition 1 value") as HTMLInputElement).type).toBe("date");
    expect((screen.getByLabelText("Condition 1 value") as HTMLInputElement).value).toBe("2026-09-01");
    expect(screen.queryByLabelText("Condition 2 value")).toBeNull();
  });

  it("offers mailboxes by account, colors by their names, and several roles", () => {
    const onChange = editor(
      all(
        { field: "mailbox", op: "is", value: "11" },
        { field: "color", op: "is", value: "1" },
        { field: "role", op: "anyof", value: "sent" },
      ),
    );
    const mailboxes = select("Condition 1 value");
    expect(mailboxes.value).toBe("11");
    const groups = Array.from(mailboxes.querySelectorAll("optgroup"));
    expect(groups.map((g) => g.label)).toEqual(["me@x.test", "you@y.test"]);
    expect(
      within(groups[0] as HTMLElement)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["INBOX", "Lists"]);
    expect(select("Condition 2 value").options[0]?.textContent).toBe("Urgent");
    expect(select("Condition 2 value").options[1]?.textContent).toBe("Orange");

    const roles = screen.getByRole("listbox", { name: "Condition 3 value" }) as HTMLSelectElement;
    expect(roles.multiple).toBe(true);
    for (const option of Array.from(roles.options)) option.selected = ["sent", "trash"].includes(option.value);
    fireEvent.change(roles);
    expect(lastChange(onChange).conditions[2]).toEqual({ field: "role", op: "anyof", value: "sent,trash" });
  });
});
