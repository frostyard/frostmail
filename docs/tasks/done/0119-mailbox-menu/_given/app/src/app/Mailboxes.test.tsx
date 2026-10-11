// CONTRACT TEST for task card T-0119 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import type { Mailbox } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useMail.setState({ vips: [], settings: null, smarts: [], rules: [] });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

async function setup(readOnly = false) {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
  for (const a of data.accounts) a.readOnly = readOnly;
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  await waitFor(() => expect(row(FIXTURE.receipts)).toBeTruthy());
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { mock, calls };
}

function row(id: number): HTMLElement {
  const el = screen.getByRole("tree", { name: "Mailboxes" }).querySelector<HTMLElement>(`[data-key="mailbox:${id}"]`);
  if (!el) throw new Error(`no row for mailbox ${id}`);
  return el;
}

async function menuOf(id: number): Promise<HTMLElement> {
  fireEvent.contextMenu(row(id), { clientX: 40, clientY: 40 });
  return screen.findByRole("menu");
}

/** entries lists a menu's rows: labels, and "—" for separators. */
function entries(menu: HTMLElement): string[] {
  return Array.from(menu.children)
    .filter((el) => el.tagName === "HR" || el.tagName === "BUTTON")
    .map((el) => (el.tagName === "HR" ? "—" : (el.querySelector(".flex-1")?.textContent ?? "")));
}

function item(menu: HTMLElement, label: string): HTMLElement {
  const found = Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"], [role="menuitemcheckbox"]')).find(
    (el) => el.querySelector(".flex-1")?.textContent === label,
  );
  if (!found) throw new Error(`no menu item ${label}`);
  return found;
}

const disabled = (menu: HTMLElement, label: string) => item(menu, label).getAttribute("aria-disabled") === "true";

describe("the sidebar's mailbox menu", () => {
  it("offers what an ordinary mailbox, the inbox and Trash take", async () => {
    await setup();
    expect(entries(await menuOf(FIXTURE.receipts))).toEqual([
      "New Mailbox…",
      "Rename Mailbox…",
      "Move Mailbox…",
      "Delete Mailbox…",
      "—",
      "Use This Mailbox For",
      "—",
      "Add to Favorites",
    ]);
    fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    const inbox = await menuOf(FIXTURE.inbox);
    expect(entries(inbox)).toEqual([
      "New Mailbox…",
      "Rename Mailbox…",
      "Move Mailbox…",
      "Delete Mailbox…",
      "—",
      "Add to Favorites",
    ]);
    expect(["Rename Mailbox…", "Move Mailbox…", "Delete Mailbox…"].map((l) => disabled(inbox, l))).toEqual([
      true,
      true,
      true,
    ]);
    fireEvent.keyDown(inbox, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(entries(await menuOf(FIXTURE.trash))).toContain("Erase Deleted Items…");
  });

  it("makes, renames and moves mailboxes in the mailbox sheet", async () => {
    const { calls, mock } = await setup();
    fireEvent.click(item(await menuOf(FIXTURE.projects), "New Mailbox…"));
    let sheet = screen.getByRole("dialog", { name: "New Mailbox" });
    expect((within(sheet).getByLabelText("Location:") as HTMLSelectElement).value).toBe(String(FIXTURE.projects));
    fireEvent.change(within(sheet).getByLabelText("Name:"), { target: { value: "Taxes" } });
    fireEvent.click(within(sheet).getByRole("button", { name: "OK" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(calls("mailbox.create")).toEqual([
      { accountId: FIXTURE.accountId, name: "Taxes", parentId: FIXTURE.projects },
    ]);
    const taxes = (await mock.call<Mailbox[]>("mailbox.list", {})).find((mb) => mb.path === "Projects/Taxes");
    await waitFor(() => expect(row(taxes?.id ?? 0)).toBeTruthy());

    fireEvent.click(item(await menuOf(taxes?.id ?? 0), "Rename Mailbox…"));
    sheet = screen.getByRole("dialog", { name: "Rename Mailbox" });
    expect((within(sheet).getByLabelText("Name:") as HTMLInputElement).value).toBe("Taxes");
    fireEvent.change(within(sheet).getByLabelText("Name:"), { target: { value: "Tax 2026" } });
    fireEvent.click(within(sheet).getByRole("button", { name: "OK" }));
    await waitFor(() => expect(calls("mailbox.rename")).toEqual([{ id: taxes?.id, name: "Tax 2026" }]));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    fireEvent.click(item(await menuOf(taxes?.id ?? 0), "Move Mailbox…"));
    sheet = screen.getByRole("dialog", { name: "Move Mailbox" });
    expect((within(sheet).getByLabelText("Location:") as HTMLSelectElement).value).toBe(String(FIXTURE.projects));
    fireEvent.change(within(sheet).getByLabelText("Location:"), { target: { value: "" } });
    fireEvent.click(within(sheet).getByRole("button", { name: "OK" }));
    await waitFor(() => expect(calls("mailbox.move")).toEqual([{ id: taxes?.id }]));
  });

  it("keeps the sheet open with maild's refusal", async () => {
    await setup();
    fireEvent.click(item(await menuOf(FIXTURE.projects), "New Mailbox…"));
    const sheet = screen.getByRole("dialog", { name: "New Mailbox" });
    fireEvent.change(within(sheet).getByLabelText("Name:"), { target: { value: "Frost" } });
    fireEvent.click(within(sheet).getByRole("button", { name: "OK" }));
    expect((await within(sheet).findByRole("alert")).textContent).toContain("already exists");
    expect(screen.getByRole("dialog", { name: "New Mailbox" })).toBe(sheet);
  });

  it("makes one at the top level from the account's header", async () => {
    const { calls } = await setup();
    fireEvent.click(screen.getByRole("button", { name: "New Mailbox" }));
    const sheet = screen.getByRole("dialog", { name: "New Mailbox" });
    expect((within(sheet).getByLabelText("Location:") as HTMLSelectElement).value).toBe("");
    fireEvent.change(within(sheet).getByLabelText("Name:"), { target: { value: "Later" } });
    fireEvent.click(within(sheet).getByRole("button", { name: "OK" }));
    await waitFor(() => expect(calls("mailbox.create")).toEqual([{ accountId: FIXTURE.accountId, name: "Later" }]));
  });

  it("deletes after asking, and leaves the deleted mailbox", async () => {
    const { calls } = await setup();
    fireEvent.click(row(FIXTURE.receipts));
    await waitFor(() => expect(useUI.getState().source).toEqual({ kind: "mailbox", mailboxId: FIXTURE.receipts }));
    vi.stubGlobal("confirm", () => false);
    fireEvent.click(item(await menuOf(FIXTURE.receipts), "Delete Mailbox…"));
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(calls("mailbox.delete")).toEqual([]);
    const asked = vi.fn(() => true);
    vi.stubGlobal("confirm", asked);
    fireEvent.click(item(await menuOf(FIXTURE.receipts), "Delete Mailbox…"));
    await waitFor(() => expect(calls("mailbox.delete")).toEqual([{ id: FIXTURE.receipts }]));
    expect(asked).toHaveBeenCalledWith('Delete the mailbox "Receipts" and the messages in it?');
    await waitFor(() => expect(useUI.getState().source).toEqual({ kind: "allInboxes" }));
  });

  it("uses a mailbox for a role, and erases Trash after asking", async () => {
    const { calls, mock } = await setup();
    const menu = await menuOf(FIXTURE.receipts);
    fireEvent.click(item(menu, "Use This Mailbox For"));
    const sub = await waitFor(() => {
      const menus = screen.getAllByRole("menu");
      expect(menus).toHaveLength(2);
      return menus[1] as HTMLElement;
    });
    expect(entries(sub)).toEqual(["Drafts", "Sent", "Junk", "Trash", "Archive"]);
    fireEvent.click(item(sub, "Archive"));
    await waitFor(() => expect(calls("mailbox.setRole")).toEqual([{ id: FIXTURE.receipts, role: "archive" }]));

    const trash = (await mock.call<Mailbox[]>("mailbox.list", {})).find((mb) => mb.id === FIXTURE.trash);
    const asked = vi.fn(() => true);
    vi.stubGlobal("confirm", asked);
    fireEvent.click(item(await menuOf(FIXTURE.trash), "Erase Deleted Items…"));
    await waitFor(() => expect(calls("mailbox.erase")).toEqual([{ id: FIXTURE.trash }]));
    expect(asked).toHaveBeenCalledWith(`Erase the ${trash?.total} messages in "Trash"? They cannot be recovered.`);
  });

  it("changes nothing on a read-only account", async () => {
    await setup(true);
    const menu = await menuOf(FIXTURE.receipts);
    for (const label of [
      "New Mailbox…",
      "Rename Mailbox…",
      "Move Mailbox…",
      "Delete Mailbox…",
      "Use This Mailbox For",
    ]) {
      expect(disabled(menu, label)).toBe(true);
    }
    expect(disabled(menu, "Add to Favorites")).toBe(false);
    expect(screen.queryByRole("button", { name: "New Mailbox" })).toBeNull();
  });
});
