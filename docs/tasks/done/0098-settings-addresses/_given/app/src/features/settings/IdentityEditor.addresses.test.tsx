// CONTRACT TEST for task card T-0098 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Account, Identity } from "../../rpc/gen/api";
import { IdentityEditor, type IdentityEditorProps } from "./IdentityEditor";

const server = { host: "h", port: 993, tls: "tls" as const, username: "u" };

function account(id: number, email: string): Account {
  return {
    id,
    kind: "imap",
    email,
    displayName: "",
    auth: "password",
    imap: server,
    smtp: server,
    createdAt: "2026-10-01T00:00:00Z",
    readOnly: false,
    notify: true,
    syncDays: 0,
    signedIn: true,
  };
}

function identity(over: Partial<Identity>): Identity {
  return { id: 1, accountId: 1, name: "", email: "", replyTo: "", signatureHtml: "", isDefault: true, ...over };
}

const accounts = [account(1, "ann@gmail.com"), account(2, "ann@icloud.com")];
const identities = [
  identity({ id: 10, accountId: 1, name: "Ann Example", email: "ann@gmail.com" }),
  identity({ id: 20, accountId: 2, name: "Ann", email: "ann@icloud.com" }),
  identity({ id: 21, accountId: 2, name: "Ann", email: "mail@ann.example", isDefault: false }),
];

function editor(over: Partial<IdentityEditorProps> = {}) {
  const props: IdentityEditorProps = {
    accounts,
    identities,
    busy: false,
    error: null,
    onSave: vi.fn(),
    onAdd: vi.fn(),
    onRemove: vi.fn(),
    ...over,
  };
  const view = render(<IdentityEditor {...props} />);
  return { props, ...view };
}

const button = (name: string) => screen.getByRole("button", { name }) as HTMLButtonElement;
const pick = (email: string) =>
  fireEvent.click(screen.getByRole("button", { name: new RegExp(email.replace(".", "\\.")) }));

function openAdd() {
  fireEvent.click(button("Add Address"));
}

describe("IdentityEditor addresses", () => {
  it("removes only an address added to an account", () => {
    const { props } = editor();
    expect(button("Remove Address").disabled).toBe(true);
    pick("mail@ann.example");
    expect(button("Remove Address").disabled).toBe(false);
    fireEvent.click(button("Remove Address"));
    expect(props.onRemove).toHaveBeenCalledWith(21);
    expect(button("Add Address").className).toContain("h-6");
  });

  it("does nothing while busy", () => {
    editor({ busy: true });
    pick("mail@ann.example");
    expect(button("Remove Address").disabled).toBe(true);
    expect(button("Add Address").disabled).toBe(true);
  });

  it("adds an address to the selected identity's account", () => {
    const { props } = editor();
    pick("ann@icloud.com");
    openAdd();
    expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("Add Address");
    expect(button("Add Address").disabled).toBe(true);
    expect(button("Remove Address").disabled).toBe(true);
    const which = screen.getByLabelText("Account:") as HTMLSelectElement;
    expect(which.value).toBe("2");
    expect(Array.from(which.options).map((o) => o.text)).toEqual(["ann@gmail.com", "ann@icloud.com"]);
    const email = screen.getByLabelText("Email Address:") as HTMLInputElement;
    const name = screen.getByLabelText("Full Name:") as HTMLInputElement;
    expect(email.type).toBe("email");
    expect(name.placeholder).toBe("Ann");
    expect(
      screen.getByText(
        "An address your provider delivers to this account, such as an alias or your own domain. Frostmail sends from it and answers invitations to it.",
      ).className,
    ).toContain("text-secondary");
    const add = button("Add");
    expect(add.type).toBe("submit");
    expect(add.disabled).toBe(true);
    fireEvent.change(email, { target: { value: "mail@" } });
    expect(add.disabled).toBe(true);
    fireEvent.change(email, { target: { value: "  team@ann.example " } });
    fireEvent.change(name, { target: { value: " Ann Team " } });
    expect(add.disabled).toBe(false);
    fireEvent.click(add);
    expect(props.onAdd).toHaveBeenCalledWith(2, "team@ann.example", "Ann Team");
  });

  it("adds to another account, its name in the placeholder", () => {
    const { props } = editor();
    openAdd();
    fireEvent.change(screen.getByLabelText("Account:"), { target: { value: "1" } });
    expect((screen.getByLabelText("Full Name:") as HTMLInputElement).placeholder).toBe("Ann Example");
    fireEvent.change(screen.getByLabelText("Email Address:"), { target: { value: "x@gmail.com" } });
    fireEvent.click(button("Add"));
    expect(props.onAdd).toHaveBeenCalledWith(1, "x@gmail.com", "");
  });

  it("cancels back to the selected identity", () => {
    editor();
    openAdd();
    fireEvent.click(button("Cancel"));
    expect(screen.queryByRole("heading", { name: "Add Address" })).toBeNull();
    expect((screen.getByLabelText("Name:") as HTMLInputElement).value).toBe("Ann Example");
    expect(button("Add Address").disabled).toBe(false);
  });

  it("shows maild's error in the add form", () => {
    editor({ error: "team@ann.example is already an address of account 2" });
    openAdd();
    expect(screen.getByRole("alert").textContent).toBe("team@ann.example is already an address of account 2");
  });

  it("selects the address it added once the list has it", () => {
    const { props, rerender } = editor();
    pick("ann@icloud.com");
    openAdd();
    fireEvent.change(screen.getByLabelText("Email Address:"), { target: { value: "TEAM@ann.example" } });
    fireEvent.click(button("Add"));
    const added = identity({ id: 22, accountId: 2, name: "Ann", email: "team@ann.example", isDefault: false });
    rerender(<IdentityEditor {...props} identities={[...identities, added]} />);
    expect(screen.queryByRole("heading", { name: "Add Address" })).toBeNull();
    expect(screen.getByRole("button", { name: /team@ann\.example/ }).getAttribute("aria-current")).toBe("true");
  });
});
