// CONTRACT TEST for task card T-0055 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Account } from "../../rpc/gen/api";
import { AccountList } from "./AccountList";

const server = { host: "h", port: 993, tls: "tls" as const, username: "u" };

function account(over: Partial<Account>): Account {
  return {
    id: 1,
    kind: "imap",
    email: "ann@x.test",
    displayName: "Ann Example",
    auth: "password",
    imap: server,
    smtp: server,
    createdAt: "2026-10-01T00:00:00Z",
    readOnly: false,
    notify: true,
    syncDays: 0,
    signedIn: true,
    ...over,
  };
}

const accounts = [
  account({ id: 1, kind: "gmail", displayName: "Ann Example", email: "ann@gmail.com" }),
  account({ id: 2, kind: "icloud", displayName: "  ", email: "ann@icloud.com", readOnly: true }),
  account({ id: 3, kind: "imap", displayName: "Work", email: "ann@work.test", signedIn: false }),
];

function list(selected: number | "new" | null) {
  const props = { onSelect: vi.fn(), onAdd: vi.fn(), onRemove: vi.fn() };
  render(<AccountList accounts={accounts} selected={selected} {...props} />);
  return props;
}

describe("AccountList", () => {
  it("lists each account with its kind or what it needs", () => {
    list(1);
    const nav = screen.getByRole("navigation", { name: "Accounts" });
    expect(nav.className).toContain("w-[220px]");
    expect(nav.className).toContain("bg-sidebar");
    const rows = within(nav).getAllByRole("listitem");
    expect(rows.map((r) => r.textContent)).toEqual([
      "Ann ExampleGmail",
      "ann@icloud.comiCloud · Read-only",
      "WorkSign in again",
    ]);
    expect(screen.getByText("Gmail").className).toContain("text-secondary");
    const warn = screen.getByText("Sign in again");
    expect(warn.className).toContain("text-flag-1");
    expect(warn.className).not.toContain("text-secondary");
    expect(screen.getByText("Ann Example").className).toContain("font-semibold");
  });

  it("marks the selected account and selects on click", () => {
    const { onSelect } = list(2);
    const rows = screen.getAllByRole("button").filter((b) => b.closest("li"));
    expect(rows.map((b) => b.getAttribute("aria-current"))).toEqual([null, "true", null]);
    expect(rows[1]?.className).toContain("bg-selection-sidebar");
    expect(rows[0]?.className).not.toContain("bg-selection-sidebar");
    fireEvent.click(screen.getByText("Work"));
    expect(onSelect).toHaveBeenCalledWith(3);
  });

  it("adds and removes", () => {
    const { onAdd, onRemove } = list(3);
    fireEvent.click(screen.getByRole("button", { name: "Add Account" }));
    expect(onAdd).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Remove Account" }));
    expect(onRemove).toHaveBeenCalledWith(3);
  });

  it("shows the account being added and cannot remove without a selection", () => {
    list("new");
    const added = screen.getByText("New Account");
    expect(added.closest("[aria-current='true']")).toBeTruthy();
    expect(screen.getAllByRole("listitem")).toHaveLength(4);
    expect((screen.getByRole("button", { name: "Add Account" }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Remove Account" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("has nothing to remove when nothing is selected", () => {
    list(null);
    expect((screen.getByRole("button", { name: "Remove Account" }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Add Account" }) as HTMLButtonElement).disabled).toBe(false);
  });
});
