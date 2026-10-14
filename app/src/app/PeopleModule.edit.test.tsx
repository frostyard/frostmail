// CONTRACT TEST for task card T-0136 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { mock, calls };
}

const ctrl = (key: string) => fireEvent.keyDown(window, { key, ctrlKey: true });
const contacts = () => screen.getByRole("listbox", { name: "Contacts" });
const pane = () => screen.getByRole("region", { name: "Contact details" });

async function showPerson(name: string) {
  ctrl("3");
  await screen.findByRole("tree", { name: "Address Books" });
  const row = await within(contacts()).findByRole("option", { name: new RegExp(name) });
  fireEvent.click(row);
  await within(pane()).findByRole("heading", { name, level: 2 });
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
});

describe("editing contacts", () => {
  it("offers Edit on writable contacts only", async () => {
    await setup();
    await showPerson("Elif Tanaka");
    expect(within(pane()).getAllByRole("button", { name: "Edit Contact" })).toHaveLength(1);
  });

  it("edits a contact and sends what changed", async () => {
    const { calls } = await setup();
    await showPerson("Ann Smith");
    fireEvent.click(within(pane()).getByRole("button", { name: "Edit Contact" }));
    const company = within(pane()).getByRole("textbox", { name: "Company" });
    fireEvent.change(company, { target: { value: "Northwind Traders" } });
    fireEvent.click(within(pane()).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(calls("people.update")).toHaveLength(1));
    expect(calls("people.update")[0]?.params).toEqual({ id: 201, fields: { organization: "Northwind Traders" } });
    await waitFor(() => expect(within(pane()).queryByRole("button", { name: "Done" })).toBeNull());
    await within(pane()).findByText("Head of Operations · Northwind Traders");
    expect(useUI.getState().peopleEditing).toBeNull();
  });

  it("closes without a call when nothing changed, and on Cancel", async () => {
    const { calls } = await setup();
    await showPerson("Ann Smith");
    fireEvent.click(within(pane()).getByRole("button", { name: "Edit Contact" }));
    fireEvent.click(within(pane()).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(within(pane()).queryByRole("button", { name: "Done" })).toBeNull());
    fireEvent.click(within(pane()).getByRole("button", { name: "Edit Contact" }));
    fireEvent.click(within(pane()).getByRole("button", { name: "Cancel" }));
    expect(within(pane()).queryByRole("button", { name: "Done" })).toBeNull();
    expect(calls("people.update")).toHaveLength(0);
  });

  it("holds People while editing", async () => {
    await setup();
    await showPerson("Ann Smith");
    fireEvent.click(within(pane()).getByRole("button", { name: "Edit Contact" }));
    expect(contacts().closest("[inert]")).not.toBeNull();
    expect(screen.getByRole("group", { name: "People sidebar" }).hasAttribute("inert")).toBe(true);
    expect(screen.getByPlaceholderText("Search Contacts").closest("[inert]")).not.toBeNull();
    ctrl("1");
    expect(useUI.getState().module).toBe("people");
    fireEvent.click(within(pane()).getByRole("button", { name: "Cancel" }));
    expect(contacts().closest("[inert]")).toBeNull();
    ctrl("1");
    expect(useUI.getState().module).toBe("mail");
  });

  it("makes a new contact from the toolbar and selects them", async () => {
    const { calls } = await setup();
    await showPerson("Ann Smith");
    fireEvent.click(screen.getByRole("button", { name: "New Contact" }));
    await within(pane()).findByRole("heading", { name: "New Contact", level: 2 });
    fireEvent.change(within(pane()).getByRole("textbox", { name: "First" }), { target: { value: "Kai" } });
    fireEvent.change(within(pane()).getByRole("textbox", { name: "Last" }), { target: { value: "Berg" } });
    fireEvent.click(within(pane()).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(calls("people.create")).toHaveLength(1));
    // The only writable book, Contacts, is named even without a choice.
    expect(calls("people.create")[0]?.params).toEqual({
      collectionId: 101,
      fields: { givenName: "Kai", familyName: "Berg" },
    });
    await within(pane()).findByRole("heading", { name: "Kai Berg", level: 2 });
    await within(contacts()).findByRole("option", { name: /Kai Berg/ });
  });

  it("starts a new contact with Ctrl+N, and Cancel returns to the person", async () => {
    const { calls } = await setup();
    await showPerson("Ann Smith");
    ctrl("n");
    await within(pane()).findByRole("heading", { name: "New Contact", level: 2 });
    expect(calls("draft.create")).toHaveLength(0);
    fireEvent.click(within(pane()).getByRole("button", { name: "Cancel" }));
    await within(pane()).findByRole("heading", { name: "Ann Smith", level: 2 });
  });
});
