// CONTRACT TEST for task card T-0120 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useMail.setState({ vips: [], settings: null, smarts: [], rules: [] });
});

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  await waitFor(() => expect(keyed(`mailbox:${FIXTURE.receipts}`)).not.toBeNull());
  const sets = () => mock.calls.filter((c) => c.method === "settings.set").map((c) => c.params);
  return { mock, sets };
}

const keyed = (key: string) =>
  screen.getByRole("tree", { name: "Mailboxes" }).querySelector<HTMLElement>(`[data-key="${key}"]`);

async function menuOf(key: string): Promise<HTMLElement> {
  fireEvent.contextMenu(keyed(key) as HTMLElement, { clientX: 40, clientY: 40 });
  return screen.findByRole("menu");
}

function item(menu: HTMLElement, label: string): HTMLElement {
  const found = Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(
    (el) => el.querySelector(".flex-1")?.textContent === label,
  );
  if (!found) throw new Error(`no menu item ${label}`);
  return found;
}

const favoriteKeys = () =>
  Array.from(screen.getByRole("tree", { name: "Mailboxes" }).querySelectorAll("[data-key^='favorite:']")).map((el) =>
    el.getAttribute("data-key"),
  );

describe("Favorites in the sidebar", () => {
  it("adds, reorders, opens and removes favorite mailboxes", async () => {
    const { sets } = await setup();
    fireEvent.click(item(await menuOf(`mailbox:${FIXTURE.receipts}`), "Add to Favorites"));
    await waitFor(() => expect(favoriteKeys()).toEqual([`favorite:${FIXTURE.receipts}`]));
    fireEvent.click(item(await menuOf(`mailbox:${FIXTURE.projects}`), "Add to Favorites"));
    await waitFor(() =>
      expect(favoriteKeys()).toEqual([`favorite:${FIXTURE.receipts}`, `favorite:${FIXTURE.projects}`]),
    );
    expect(sets()).toEqual([{ favorites: [FIXTURE.receipts] }, { favorites: [FIXTURE.receipts, FIXTURE.projects] }]);

    const menu = await menuOf(`favorite:${FIXTURE.projects}`);
    expect(item(menu, "Move Down").getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(item(menu, "Move Up"));
    await waitFor(() =>
      expect(favoriteKeys()).toEqual([`favorite:${FIXTURE.projects}`, `favorite:${FIXTURE.receipts}`]),
    );

    fireEvent.click(keyed(`favorite:${FIXTURE.receipts}`) as HTMLElement);
    await waitFor(() => expect(useUI.getState().source).toEqual({ kind: "mailbox", mailboxId: FIXTURE.receipts }));

    fireEvent.click(item(await menuOf(`mailbox:${FIXTURE.projects}`), "Remove from Favorites"));
    await waitFor(() => expect(favoriteKeys()).toEqual([`favorite:${FIXTURE.receipts}`]));
    fireEvent.click(item(await menuOf(`favorite:${FIXTURE.receipts}`), "Remove from Favorites"));
    await waitFor(() => expect(favoriteKeys()).toEqual([]));
    expect(sets().at(-1)).toEqual({ favorites: [] });
  });
});
