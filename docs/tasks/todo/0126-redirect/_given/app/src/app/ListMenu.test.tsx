// CONTRACT TEST for task cards T-0103, T-0118 and T-0126 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

function setup(readOnly = false) {
  const data = mockData({ inbox: 20, now: new Date("2026-10-08T12:00:00Z") });
  for (const a of data.accounts) a.readOnly = readOnly;
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  return { data, mock };
}

async function firstRow(): Promise<HTMLElement> {
  const list = await screen.findByRole("listbox", { name: "Messages" });
  return waitFor(() => {
    const row = within(list).queryAllByRole("option")[0];
    if (!row) throw new Error("no rows yet");
    return row;
  });
}

/** openMenu right-clicks a row and returns the menu. */
async function openMenu(row: HTMLElement): Promise<HTMLElement> {
  fireEvent.contextMenu(row, { clientX: 50, clientY: 50 });
  return screen.findByRole("menu");
}

/** entries lists a menu's rows: labels, and "—" for separators. */
function entries(menu: HTMLElement): string[] {
  return Array.from(menu.children)
    .filter((el) => el.tagName === "HR" || el.tagName === "BUTTON")
    .map((el) => (el.tagName === "HR" ? "—" : (el.querySelector(".flex-1")?.textContent ?? "")));
}

/** menuItems lists a menu's items, checkable or not, open submenus' too. */
function menuItems(menu: HTMLElement): HTMLElement[] {
  return Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"], [role="menuitemcheckbox"]'));
}

function item(menu: HTMLElement, label: string): HTMLElement {
  const found = menuItems(menu).find((el) => el.querySelector(".flex-1")?.textContent === label);
  if (!found) throw new Error(`no menu item ${label}`);
  return found;
}

function calls(mock: MockTransport, method: string) {
  return mock.calls.filter((c) => c.method === method).map((c) => c.params);
}

describe("the message list's context menu", () => {
  it("lists the actions in groups", async () => {
    const { mock } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    const flags = mock.message(id)?.summary.flags;
    const menu = await openMenu(row);
    expect(entries(menu)).toEqual([
      "Reply",
      "Reply All",
      "Forward",
      "Forward as Attachment",
      "Redirect…",
      "—",
      "Archive",
      "Delete",
      "Mark as Spam",
      "—",
      "Move to",
      "Copy to",
      "—",
      flags?.flagged ? "Unflag" : "Flag",
      "Flag Color",
      flags?.seen ? "Mark as Unread" : "Mark as Read",
      "Remind Me",
    ]);
    expect(item(menu, "Mark as Spam").textContent).toContain("Ctrl+Shift+J");
    expect(item(menu, flags?.flagged ? "Unflag" : "Flag").textContent).toContain("Ctrl+Shift+L");
    expect(item(menu, "Copy to").getAttribute("aria-haspopup")).toBe("menu");
    expect(item(menu, "Flag Color").getAttribute("aria-haspopup")).toBe("menu");
    expect(useUI.getState().selected).toEqual([id]);
  });

  it("marks as spam into Junk", async () => {
    const { mock } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    fireEvent.click(item(await openMenu(row), "Mark as Spam"));
    await waitFor(() => expect(calls(mock, "message.move")).toEqual([{ ids: [id], mailboxId: FIXTURE.junk }]));
  });

  it("copies to a mailbox from the Copy to submenu", async () => {
    const { mock } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    const menu = await openMenu(row);
    fireEvent.click(item(menu, "Copy to"));
    const sub = await waitFor(() => {
      const menus = screen.getAllByRole("menu");
      expect(menus).toHaveLength(2);
      return menus[1] as HTMLElement;
    });
    expect(entries(sub)).toEqual([
      "Drafts",
      "Sent",
      "Junk",
      "Trash",
      "Archive",
      "Projects",
      "Projects/Frost",
      "Receipts",
    ]);
    fireEvent.click(item(sub, "Receipts"));
    await waitFor(() => expect(calls(mock, "message.copy")).toEqual([{ ids: [id], mailboxId: FIXTURE.receipts }]));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(mock.message(id)?.summary.mailboxIds).toEqual([FIXTURE.inbox]);
  });

  it("toggles the flag in one step", async () => {
    const { mock } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    const flagged = mock.message(id)?.summary.flags.flagged ?? false;
    fireEvent.click(item(await openMenu(row), flagged ? "Unflag" : "Flag"));
    await waitFor(() =>
      expect(calls(mock, "message.setFlags")).toContainEqual({ ids: [id], changes: { flagColor: flagged ? 0 : 1 } }),
    );
  });

  it("offers Not Spam in Junk, back to the inbox", async () => {
    const { mock } = setup();
    await firstRow();
    act(() => useUI.getState().setSource({ kind: "mailbox", mailboxId: FIXTURE.junk }));
    const row = await waitFor(() => {
      const r = within(screen.getByRole("listbox", { name: "Messages" })).queryAllByRole("option")[0];
      const id = Number(r?.getAttribute("data-message-id"));
      if (!r || !mock.message(id)?.summary.mailboxIds.includes(FIXTURE.junk)) throw new Error("no junk row yet");
      return r;
    });
    const id = Number(row.getAttribute("data-message-id"));
    const menu = await openMenu(row);
    expect(entries(menu)).toContain("Not Spam");
    expect(entries(menu)).not.toContain("Mark as Spam");
    fireEvent.click(item(menu, "Not Spam"));
    await waitFor(() =>
      expect(calls(mock, "message.move")).toEqual([
        { ids: [id], mailboxId: FIXTURE.inbox, fromMailboxId: FIXTURE.junk },
      ]),
    );
  });

  it("only replies and forwards on a read-only account", async () => {
    setup(true);
    const menu = await openMenu(await firstRow());
    const enabled = menuItems(menu)
      .filter((el) => el.getAttribute("aria-disabled") !== "true")
      .map((el) => el.querySelector(".flex-1")?.textContent);
    expect(enabled).toEqual(["Reply", "Reply All", "Forward", "Forward as Attachment"]);
  });

  it("marks the selection as spam with Ctrl+Shift+J", async () => {
    const { mock } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    fireEvent.click(row);
    await waitFor(() => expect(useUI.getState().selected).toEqual([id]));
    fireEvent.keyDown(window, { key: "J", ctrlKey: true, shiftKey: true });
    await waitFor(() => expect(calls(mock, "message.move")).toEqual([{ ids: [id], mailboxId: FIXTURE.junk }]));
  });
});
