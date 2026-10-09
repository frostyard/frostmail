// CONTRACT TEST for task card T-0093 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import {
  AccountForm,
  type AccountFormProps,
  type AccountFormValue,
  defaultServices,
  offeredServices,
} from "./AccountForm";

const value: AccountFormValue = {
  kind: "gmail",
  email: "ann@gmail.test",
  displayName: "Ann Example",
  auth: "oauth2",
  imap: { host: "", port: 993, tls: "tls", username: "" },
  smtp: { host: "", port: 465, tls: "tls", username: "" },
  readOnly: true,
  notify: true,
  password: "",
  services: ["contacts", "calendar", "tasks"],
};

function form(over: Partial<AccountFormProps> = {}) {
  const props: AccountFormProps = {
    mode: "add",
    value,
    onChange: vi.fn(),
    discovery: { kind: "idle" },
    onDiscover: vi.fn(),
    signedIn: false,
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

const region = () => screen.getByRole("region", { name: "Contacts, Calendars and Tasks" });
const box = (name: string) => within(region()).getByRole("checkbox", { name }) as HTMLInputElement;

describe("services by kind", () => {
  it("offers what each kind can reach, in order", () => {
    expect(offeredServices("gmail")).toEqual(["contacts", "calendar", "tasks"]);
    expect(offeredServices("imap")).toEqual(["contacts", "calendar", "tasks"]);
    expect(offeredServices("icloud")).toEqual(["contacts", "calendar"]);
    expect(offeredServices("microsoft")).toEqual([]);
  });

  it("checks them for Gmail and iCloud, and none for IMAP", () => {
    expect(defaultServices("gmail")).toEqual(["contacts", "calendar", "tasks"]);
    expect(defaultServices("icloud")).toEqual(["contacts", "calendar"]);
    expect(defaultServices("imap")).toEqual([]);
  });
});

describe("AccountForm's services in add mode", () => {
  it("lists the kind's services, checked from the value", () => {
    form({ value: { ...value, services: ["contacts", "tasks"] } });
    expect(
      within(region())
        .getAllByRole("checkbox")
        .map((b) => [b.getAttribute("aria-label") ?? b.closest("label")?.textContent, (b as HTMLInputElement).checked]),
    ).toEqual([
      ["Contacts", true],
      ["Calendars", false],
      ["Tasks", true],
    ]);
  });

  it("reports a toggled service in order", () => {
    const props = form({ value: { ...value, services: ["contacts", "tasks"] } });
    fireEvent.click(box("Calendars"));
    expect(props.onChange).toHaveBeenLastCalledWith({ ...value, services: ["contacts", "calendar", "tasks"] });
    fireEvent.click(box("Contacts"));
    expect(props.onChange).toHaveBeenLastCalledWith({ ...value, services: ["tasks"] });
  });

  it("offers iCloud no tasks", () => {
    form({ value: { ...value, kind: "icloud", auth: "password", services: ["contacts", "calendar"] } });
    expect(within(region()).queryByRole("checkbox", { name: "Tasks" })).toBeNull();
    expect(box("Calendars").checked).toBe(true);
  });

  it("starts a new kind with its default services", () => {
    const props = form();
    fireEvent.change(screen.getByLabelText("Account Type:"), { target: { value: "icloud" } });
    expect(props.onChange).toHaveBeenLastCalledWith({
      ...value,
      kind: "icloud",
      auth: "password",
      services: ["contacts", "calendar"],
    });
    fireEvent.change(screen.getByLabelText("Account Type:"), { target: { value: "imap" } });
    expect(props.onChange).toHaveBeenLastCalledWith({ ...value, kind: "imap", auth: "password", services: [] });
  });

  it("says one Google sign-in covers mail and the services", () => {
    form();
    expect(screen.getByText("Google asks once for mail and the services checked here.")).toBeTruthy();
    expect(
      screen.getByText(
        "After adding the account, sign in with Google in your browser, once for mail and the services checked below.",
      ),
    ).toBeTruthy();
  });

  it("says a password account finds them with its password", () => {
    form({ value: { ...value, kind: "icloud", auth: "password", services: ["contacts"] } });
    expect(screen.getByText("Frostmail finds them with the account's password.")).toBeTruthy();
  });

  it("disables the services while busy, and treats absent ones as none", () => {
    const { services: _, ...bare } = value;
    form({ value: bare, busy: true });
    expect(box("Contacts").checked).toBe(false);
    expect(box("Contacts").disabled).toBe(true);
  });

  it("shows no checklist in edit mode", () => {
    form({ mode: "edit", signedIn: true });
    expect(screen.queryByRole("region", { name: "Contacts, Calendars and Tasks" })).toBeNull();
  });
});
