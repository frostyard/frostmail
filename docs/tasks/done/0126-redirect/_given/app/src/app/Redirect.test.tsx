// CONTRACT TEST for task card T-0126 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

afterEach(() => {
  vi.restoreAllMocks();
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
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { data, mock, calls };
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

async function choose(row: HTMLElement, label: string) {
  fireEvent.contextMenu(row, { clientX: 50, clientY: 50 });
  fireEvent.click(item(await screen.findByRole("menu"), label));
}

describe("Forward as Attachment and Redirect", () => {
  it("forwards the message as an attachment in a compose window", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const { calls } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    await choose(row, "Forward as Attachment");
    await waitFor(() => expect(calls("draft.create")).toEqual([{ kind: "attached", sourceId: id }]));
    await waitFor(() => expect(open).toHaveBeenCalledTimes(1));
    expect(String(open.mock.calls[0]?.[0])).toMatch(/#\/compose\/\d+\?fresh$/);
  });

  it("redirects from the sheet, which closes", async () => {
    const { mock, calls } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    await choose(row, "Redirect…");
    const dialog = await screen.findByRole("dialog", { name: "Redirect" });
    expect(dialog.textContent).toContain(mock.message(id)?.summary.subject ?? "?");
    const to = within(dialog).getByRole("combobox", { name: "To" });
    fireEvent.change(to, { target: { value: "bob@x.test" } });
    fireEvent.keyDown(to, { key: "Enter" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Redirect" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(calls("message.redirect")).toEqual([{ id, to: [{ name: "", address: "bob@x.test" }] }]);
  });

  it("keeps the sheet open with maild's complaint", async () => {
    const { data, calls } = setup();
    await choose(await firstRow(), "Redirect…");
    const dialog = await screen.findByRole("dialog", { name: "Redirect" });
    for (const a of data.accounts) a.readOnly = true;
    const to = within(dialog).getByRole("combobox", { name: "To" });
    fireEvent.change(to, { target: { value: "bob@x.test" } });
    fireEvent.keyDown(to, { key: "Enter" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Redirect" }));
    expect((await within(dialog).findByRole("alert")).textContent).toContain(
      `account ${FIXTURE.accountId} is read-only`,
    );
    expect(calls("message.redirect")).toHaveLength(1);
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("forwards as an attachment but does not redirect on a read-only account", async () => {
    setup(true);
    fireEvent.contextMenu(await firstRow(), { clientX: 50, clientY: 50 });
    const menu = await screen.findByRole("menu");
    expect(item(menu, "Forward as Attachment").getAttribute("aria-disabled")).not.toBe("true");
    expect(item(menu, "Redirect…").getAttribute("aria-disabled")).toBe("true");
  });
});
