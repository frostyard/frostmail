// Mark All Messages as Read, in a mailbox's menu (docs/specs/organize-ui.md).
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

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
  await waitFor(() => expect(mailbox(FIXTURE.inbox)).toBeDefined());
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { mock, calls };
}

const mailbox = (id: number) => useMail.getState().mailboxes.find((m) => m.id === id);

async function menuItem(key: string): Promise<HTMLElement> {
  const row = screen.getByRole("tree", { name: "Mailboxes" }).querySelector<HTMLElement>(`[data-key="${key}"]`);
  if (!row) throw new Error(`no row ${key}`);
  fireEvent.contextMenu(row, { clientX: 40, clientY: 40 });
  const menu = await screen.findByRole("menu");
  const found = Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(
    (el) => el.querySelector(".flex-1")?.textContent === "Mark All Messages as Read",
  );
  if (!found) throw new Error("the menu has no Mark All Messages as Read");
  return found;
}

const off = (item: HTMLElement) => item.getAttribute("aria-disabled") === "true";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

describe("Mark All Messages as Read", () => {
  it("marks every unread message of the mailbox, and is off once none is left", async () => {
    const { calls } = await setup();
    const before = mailbox(FIXTURE.inbox)?.unread ?? 0;
    expect(before).toBeGreaterThan(0);
    const item = await menuItem(`mailbox:${FIXTURE.inbox}`);
    expect(off(item)).toBe(false);
    fireEvent.click(item);
    await waitFor(() => expect(mailbox(FIXTURE.inbox)?.unread).toBe(0));
    expect(calls("mailbox.markRead")).toEqual([{ id: FIXTURE.inbox }]);
    expect(off(await menuItem(`mailbox:${FIXTURE.inbox}`))).toBe(true);
  });

  it("is off in a read-only account", async () => {
    await setup(true);
    expect(mailbox(FIXTURE.inbox)?.unread).toBeGreaterThan(0);
    expect(off(await menuItem(`mailbox:${FIXTURE.inbox}`))).toBe(true);
  });

  it("is in a favorite's menu too, and acts on its mailbox", async () => {
    const { mock, calls } = await setup();
    await mock.call("settings.set", { favorites: [FIXTURE.inbox] });
    await waitFor(() =>
      expect(
        screen.getByRole("tree", { name: "Mailboxes" }).querySelector(`[data-key="favorite:${FIXTURE.inbox}"]`),
      ).not.toBeNull(),
    );
    fireEvent.click(await menuItem(`favorite:${FIXTURE.inbox}`));
    await waitFor(() => expect(mailbox(FIXTURE.inbox)?.unread).toBe(0));
    expect(calls("mailbox.markRead")).toEqual([{ id: FIXTURE.inbox }]);
  });
});
