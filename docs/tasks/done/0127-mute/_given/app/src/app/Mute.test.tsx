// CONTRACT TEST for task card T-0127 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
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
  const calls = () => mock.calls.filter((c) => c.method === "message.mute").map((c) => c.params);
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

function entries(menu: HTMLElement): string[] {
  return Array.from(menu.children)
    .filter((el) => el.tagName === "HR" || el.tagName === "BUTTON")
    .map((el) => (el.tagName === "HR" ? "—" : (el.querySelector(".flex-1")?.textContent ?? "")));
}

function item(menu: HTMLElement, label: string): HTMLElement {
  const found = Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(
    (el) => el.querySelector(".flex-1")?.textContent === label,
  );
  if (!found) throw new Error(`no menu item ${label}`);
  return found;
}

async function openMenu(row: HTMLElement): Promise<HTMLElement> {
  fireEvent.contextMenu(row, { clientX: 50, clientY: 50 });
  return screen.findByRole("menu");
}

describe("Mute", () => {
  it("mutes from the menu, shows the bell, and unmutes", async () => {
    const { calls } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    const menu = await openMenu(row);
    const labels = entries(menu);
    expect(labels.indexOf("Mute")).toBe(labels.indexOf("Remind Me") - 1);
    fireEvent.click(item(menu, "Mute"));
    await waitFor(() => expect(calls()).toEqual([{ ids: [id], muted: true }]));
    const muted = await waitFor(() => {
      const r = screen.getByRole("listbox", { name: "Messages" }).querySelector(`[data-message-id="${id}"]`);
      if (!r || !within(r as HTMLElement).queryByRole("img", { name: "Muted" })) throw new Error("no bell yet");
      return r as HTMLElement;
    });
    fireEvent.click(item(await openMenu(muted), "Unmute"));
    await waitFor(() =>
      expect(calls()).toEqual([
        { ids: [id], muted: true },
        { ids: [id], muted: false },
      ]),
    );
    await waitFor(() => expect(within(muted).queryByRole("img", { name: "Muted" })).toBeNull());
  });

  it("is disabled on a read-only account", async () => {
    setup(true);
    const menu = await openMenu(await firstRow());
    expect(item(menu, "Mute").getAttribute("aria-disabled")).toBe("true");
  });
});
