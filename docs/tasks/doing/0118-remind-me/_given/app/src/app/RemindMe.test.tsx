// CONTRACT TEST for task card T-0118 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { appLocale } from "../lib/calendarDates";
import { atLocal, remindChoices, whenText } from "../lib/later";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;

function setup(readOnly = false) {
  const data = mockData({ inbox: 20, now: new Date("2026-10-08T12:00:00Z") });
  for (const a of data.accounts) a.readOnly = readOnly;
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  const calls = () => mock.calls.filter((c) => c.method === "message.remind").map((c) => c.params);
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

/** remindMenu opens a row's context menu and its Remind Me submenu. */
async function remindMenu(row: HTMLElement): Promise<HTMLElement> {
  fireEvent.contextMenu(row, { clientX: 50, clientY: 50 });
  const menu = await screen.findByRole("menu");
  fireEvent.click(item(menu, "Remind Me"));
  return waitFor(() => {
    const menus = screen.getAllByRole("menu");
    expect(menus).toHaveLength(2);
    return menus[1] as HTMLElement;
  });
}

describe("Remind Me", () => {
  it("sets a reminder from the menu, shows it, and clears it in the reader", async () => {
    const { calls } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    const sub = await remindMenu(row);
    const choices = remindChoices(new Date(), zone);
    expect(entries(sub)).toEqual([...choices.map((c) => c.label), "—", "Remind Me Later…"]);
    fireEvent.click(item(sub, "Remind Me Tomorrow"));
    await waitFor(() => expect(calls()).toHaveLength(1));
    const [params] = calls() as { ids: number[]; at: string }[];
    expect(params?.ids).toEqual([id]);
    expect(params?.at).toBe(choices.at(-1)?.at.toISOString());

    const at = new Date(params?.at ?? "");
    const label = `Reminder ${whenText(at, new Date(), zone, appLocale(navigator.language))}`;
    await waitFor(() => expect(within(row).getByRole("img", { name: label })).toBeTruthy());

    fireEvent.click(row);
    const banner = await screen.findByText(
      `Remind Me: ${whenText(at, new Date(), zone, appLocale(navigator.language))}`,
    );
    fireEvent.click(within(banner.closest('[role="status"]') as HTMLElement).getByRole("button", { name: "Clear" }));
    await waitFor(() => expect(calls().at(-1)).toEqual({ ids: [id] }));
    await waitFor(() => expect(screen.queryByText(/^Remind Me: /)).toBeNull());
    await waitFor(() => expect(within(row).queryByRole("img", { name: /^Reminder / })).toBeNull());
  });

  it("sets a chosen time, and clears from the menu", async () => {
    const { calls } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    fireEvent.click(item(await remindMenu(row), "Remind Me Later…"));
    const sheet = screen.getByRole("dialog", { name: "Remind Me" });
    fireEvent.change(within(sheet).getByLabelText("Date"), { target: { value: "2030-01-02" } });
    fireEvent.change(within(sheet).getByLabelText("Time"), { target: { value: "07:15" } });
    fireEvent.click(within(sheet).getByRole("button", { name: "OK" }));
    await waitFor(() => expect(calls()).toEqual([{ ids: [id], at: atLocal("2030-01-02", 7, 15, zone).toISOString() }]));
    expect(screen.queryByRole("dialog")).toBeNull();

    await waitFor(() => expect(within(row).queryByRole("img", { name: /^Reminder / })).not.toBeNull());
    const sub = await remindMenu(row);
    expect(entries(sub).at(-1)).toBe("Clear Reminder");
    fireEvent.click(item(sub, "Clear Reminder"));
    await waitFor(() => expect(calls().at(-1)).toEqual({ ids: [id] }));
  });

  it("is disabled on a read-only account", async () => {
    setup(true);
    fireEvent.contextMenu(await firstRow(), { clientX: 50, clientY: 50 });
    const menu = await screen.findByRole("menu");
    expect(item(menu, "Remind Me").getAttribute("aria-disabled")).toBe("true");
  });
});
