// CONTRACT TEST for task card T-0064 (docs/tasks), amended by T-0073 for
// the Calendar module. Do not edit.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { Client } from "../rpc/gen/api";
import { BOOKS, FIXTURE, mockData } from "../rpc/mock/fixture";
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
const names = () =>
  within(contacts())
    .getAllByRole("option")
    .map((o) => o.querySelector(".font-semibold:not([aria-hidden])")?.textContent);

async function showPeople() {
  ctrl("3");
  await screen.findByRole("tree", { name: "Address Books" });
  await waitFor(() => expect(within(contacts()).getAllByRole("option").length).toBeGreaterThan(0));
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
});

describe("People module", () => {
  it("opens with Ctrl+3 and returns to Mail with Ctrl+1", async () => {
    await setup();
    expect(useUI.getState().module).toBe("mail");
    await showPeople();
    expect(useUI.getState().module).toBe("people");
    expect(screen.queryByRole("tree", { name: "Mailboxes" })).toBeNull();
    ctrl("1");
    await screen.findByRole("tree", { name: "Mailboxes" });
    expect(useUI.getState().module).toBe("mail");
    expect(useUI.getState().source).toEqual({ kind: "allInboxes" });
  });

  it("switches from the module bar under each sidebar", async () => {
    await setup();
    const bar = screen.getByRole("toolbar", { name: "Modules" });
    expect(within(bar).getByRole("button", { name: "Mail" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(within(bar).getByRole("button", { name: "People" }));
    await screen.findByRole("tree", { name: "Address Books" });
    const peopleBar = screen.getByRole("toolbar", { name: "Modules" });
    expect(within(peopleBar).getByRole("button", { name: "People" }).getAttribute("aria-pressed")).toBe("true");
    expect(within(peopleBar).getByRole("button", { name: "Calendar" })).toBeTruthy();
    expect(within(peopleBar).queryByRole("button", { name: "Tasks" })).toBeNull();
  });

  it("lists people with letter headers, and the toolbar counts them", async () => {
    await setup();
    await showPeople();
    expect(names().slice(0, 3)).toEqual(["42 Club", "Farah Garcia", "Hiro Haddad"]);
    expect(within(contacts()).getByText("#")).toBeTruthy();
    expect(screen.getByText("All Contacts", { selector: ".text-toolbar-title" })).toBeTruthy();
    expect(screen.getByText("12 contacts")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Delete" })).toBeNull();
  });

  it("shows the selected person, and keeps them while Mail is open", async () => {
    const { calls } = await setup();
    await showPeople();
    expect(screen.getByText("No Contact Selected")).toBeTruthy();
    fireEvent.click(within(contacts()).getByRole("option", { name: /Ann Smith/ }));
    expect(await screen.findByRole("heading", { level: 2, name: "Ann Smith" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "Contacts — test1@mailtest.test" })).toBeTruthy();
    await screen.findByRole("region", { name: "Recent Mail" });
    expect(calls("people.card").at(-1)?.params).toEqual({ email: "ann.smith@northwind.test" });
    ctrl("1");
    await screen.findByRole("tree", { name: "Mailboxes" });
    await showPeople();
    expect(await screen.findByRole("heading", { level: 2, name: "Ann Smith" })).toBeTruthy();
    expect(
      within(contacts())
        .getByRole("option", { name: /Ann Smith/ })
        .getAttribute("aria-selected"),
    ).toBe("true");
  });

  it("moves the selection with the arrow keys", async () => {
    await setup();
    await showPeople();
    fireEvent.click(within(contacts()).getByRole("option", { name: /Farah Garcia/ }));
    await screen.findByRole("heading", { level: 2, name: "Farah Garcia" });
    fireEvent.keyDown(contacts(), { key: "ArrowDown" });
    expect(await screen.findByRole("heading", { level: 2, name: "Hiro Haddad" })).toBeTruthy();
    fireEvent.keyDown(contacts(), { key: "ArrowUp" });
    expect(await screen.findByRole("heading", { level: 2, name: "Farah Garcia" })).toBeTruthy();
  });

  it("filters by address book", async () => {
    const { calls } = await setup();
    await showPeople();
    fireEvent.click(screen.getByRole("treeitem", { name: /^Shared/ }));
    await waitFor(() => expect(names()).toEqual(["Elif Tanaka"]));
    expect(calls("people.list").at(-1)?.params).toEqual({ collectionId: BOOKS.shared });
    expect(screen.getByText("Shared", { selector: ".text-toolbar-title" })).toBeTruthy();
    expect(screen.getByText("1 contact")).toBeTruthy();
  });

  it("searches as typed and clears with Escape", async () => {
    const user = userEvent.setup();
    const { calls } = await setup();
    await showPeople();
    const field = screen.getByPlaceholderText("Search Contacts");
    await user.type(field, "north");
    await waitFor(() => expect(names()).toEqual(["Farah Garcia", "Ann Smith"]));
    expect(calls("people.list").at(-1)?.params).toEqual({ query: "north" });
    expect(screen.getByText("2 results")).toBeTruthy();
    fireEvent.keyDown(field, { key: "Escape" });
    await waitFor(() => expect(names().length).toBeGreaterThan(2));
    expect(screen.getByText("12 contacts")).toBeTruthy();
  });

  it("follows people.changed", async () => {
    const { mock } = await setup();
    await showPeople();
    await act(async () => {
      await new Client(mock).people.add({ email: "aaron@example.test", name: "Aaron Aardvark" });
    });
    await waitFor(() => expect(names()).toContain("Aaron Aardvark"));
    expect(screen.getByText("13 contacts")).toBeTruthy();
  });

  it("writes to the person and opens their mail in Mail", async () => {
    const { mock, calls } = await setup();
    await showPeople();
    fireEvent.click(within(contacts()).getByRole("option", { name: /Ann Smith/ }));
    await screen.findByRole("heading", { level: 2, name: "Ann Smith" });
    fireEvent.click(screen.getByRole("button", { name: "Message" }));
    await waitFor(() => expect(calls("draft.create")).toHaveLength(1));
    expect(calls("draft.create")[0]?.params).toMatchObject({
      kind: "new",
      to: [{ address: "ann.smith@northwind.test" }],
    });
    const recent = await screen.findByRole("region", { name: "Recent Mail" });
    fireEvent.click(within(recent).getByRole("button", { name: /^Re: Offsite plan/ }));
    await screen.findByRole("tree", { name: "Mailboxes" });
    const ui = useUI.getState();
    expect(ui.module).toBe("mail");
    expect(ui.source).toEqual({ kind: "mailbox", mailboxId: FIXTURE.inbox });
    expect(ui.selected).toHaveLength(1);
    expect(mock.message(ui.selected[0] as number)?.summary.subject).toBe("Re: Offsite plan");
  });
});
