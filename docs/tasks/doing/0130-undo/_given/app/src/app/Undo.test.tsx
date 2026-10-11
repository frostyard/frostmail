// CONTRACT TEST for task card T-0130 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";
import { useUndo } from "./undo";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useUndo.setState({ entries: [] });
});

function setup() {
  const data = mockData({ inbox: 20, now: new Date("2026-10-08T12:00:00Z") });
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { mock, calls };
}

async function firstRow(): Promise<HTMLElement> {
  const list = await screen.findByRole("listbox", { name: "Messages" });
  return waitFor(() => {
    const row = within(list).queryAllByRole("option")[0];
    if (!row) throw new Error("no rows yet");
    return row;
  });
}

function item(menu: HTMLElement, label: string): HTMLElement {
  const found = Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(
    (el) => el.querySelector(".flex-1")?.textContent === label,
  );
  if (!found) throw new Error(`no menu item ${label}`);
  return found;
}

async function choose(row: HTMLElement, ...path: string[]) {
  fireEvent.contextMenu(row, { clientX: 50, clientY: 50 });
  let menu = await screen.findByRole("menu");
  for (const [i, label] of path.entries()) {
    fireEvent.click(item(menu, label));
    if (i < path.length - 1) {
      menu = await waitFor(() => {
        const menus = screen.getAllByRole("menu");
        expect(menus).toHaveLength(i + 2);
        return menus.at(-1) as HTMLElement;
      });
    }
  }
}

const ctrlZ = (target: Element | Window = window) => fireEvent.keyDown(target, { key: "z", ctrlKey: true });

describe("Undo", () => {
  it("moves a message back with Ctrl+Z", async () => {
    const { mock, calls } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    await choose(row, "Move to", "Receipts");
    await waitFor(() => expect(useUndo.getState().entries).toHaveLength(1));
    expect(mock.message(id)?.summary.mailboxIds).toEqual([FIXTURE.receipts]);
    ctrlZ();
    await waitFor(() =>
      expect(calls("message.move").at(-1)).toEqual({
        ids: [id],
        mailboxId: FIXTURE.inbox,
        fromMailboxId: FIXTURE.receipts,
      }),
    );
    await waitFor(() => expect(mock.message(id)?.summary.mailboxIds).toEqual([FIXTURE.inbox]));
    expect(useUndo.getState().entries).toEqual([]);
  });

  it("brings a deleted message back from the Trash", async () => {
    const { mock } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    await choose(row, "Delete");
    await waitFor(() => expect(useUndo.getState().entries).toHaveLength(1));
    expect(mock.message(id)?.summary.mailboxIds).toEqual([FIXTURE.trash]);
    ctrlZ();
    await waitFor(() => expect(mock.message(id)?.summary.mailboxIds).toEqual([FIXTURE.inbox]));
  });

  it("takes back flag and read changes newest first", async () => {
    const { mock } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    const before = mock.message(id)?.summary.flags;
    fireEvent.click(row);
    await waitFor(() => expect(useUI.getState().selected).toEqual([id]));
    // Opening the message marks it read; that is not undone.
    await waitFor(() => expect(mock.message(id)?.summary.flags.seen).toBe(true));
    fireEvent.keyDown(window, { key: "L", ctrlKey: true, shiftKey: true });
    await waitFor(() => expect(useUndo.getState().entries).toHaveLength(1));
    fireEvent.keyDown(window, { key: "U", ctrlKey: true, shiftKey: true });
    await waitFor(() => expect(useUndo.getState().entries).toHaveLength(2));
    expect(mock.message(id)?.summary.flags.seen).toBe(false);
    ctrlZ();
    await waitFor(() => expect(mock.message(id)?.summary.flags.seen).toBe(true));
    expect(mock.message(id)?.summary.flags.flagged).toBe(!before?.flagged);
    ctrlZ();
    await waitFor(() => expect(mock.message(id)?.summary.flags.flagged).toBe(before?.flagged));
    expect(mock.message(id)?.summary.flags.flagColor).toBe(before?.flagColor);
  });

  it("leaves Ctrl+Z in a text field to the field", async () => {
    const { calls } = setup();
    const row = await firstRow();
    await choose(row, "Move to", "Receipts");
    await waitFor(() => expect(useUndo.getState().entries).toHaveLength(1));
    const moves = calls("message.move").length;
    ctrlZ(screen.getByRole("searchbox", { name: "Search" }));
    expect(useUndo.getState().entries).toHaveLength(1);
    expect(calls("message.move")).toHaveLength(moves);
  });
});
