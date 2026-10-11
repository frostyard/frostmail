// CONTRACT TEST for task card T-0113 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

const everything = { match: "all", conditions: [] };

async function setup(rules: { enabled: boolean }[], readOnly = false) {
  const data = mockData({ inbox: 20, now: new Date("2026-10-08T12:00:00Z") });
  for (const a of data.accounts) a.readOnly = readOnly;
  const mock = new MockTransport(data);
  for (const [i, r] of rules.entries()) {
    await mock.call("rule.create", {
      name: `Rule ${i + 1}`,
      conditions: everything,
      actions: [{ kind: "move", mailboxId: FIXTURE.receipts }],
      enabled: r.enabled,
    });
  }
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await waitFor(() => expect(useMail.getState().rules).toHaveLength(rules.length));
  const list = await screen.findByRole("listbox", { name: "Messages" });
  const row = await waitFor(() => {
    const r = within(list).queryAllByRole("option")[0];
    if (!r) throw new Error("no rows yet");
    return r;
  });
  fireEvent.contextMenu(row, { clientX: 50, clientY: 50 });
  const menu = await screen.findByRole("menu");
  return { mock, menu, id: Number(row.getAttribute("data-message-id")) };
}

/** entries lists a menu's rows: labels, and "—" for separators. */
function entries(menu: HTMLElement): string[] {
  return Array.from(menu.children)
    .filter((el) => el.tagName === "HR" || el.tagName === "BUTTON")
    .map((el) => (el.tagName === "HR" ? "—" : (el.querySelector(".flex-1")?.textContent ?? "")));
}

function applyItem(menu: HTMLElement): HTMLElement {
  const found = Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(
    (el) => el.querySelector(".flex-1")?.textContent === "Apply Rules",
  );
  if (!found) throw new Error("no Apply Rules");
  return found;
}

describe("Apply Rules in the message list's context menu", () => {
  it("runs the enabled rules on the menu's messages", async () => {
    const { mock, menu, id } = await setup([{ enabled: true }]);
    expect(entries(menu).slice(-2)).toEqual(["—", "Apply Rules"]);
    expect(applyItem(menu).textContent).toBe("Apply Rules");
    fireEvent.click(applyItem(menu));
    await waitFor(() =>
      expect(mock.calls.filter((c) => c.method === "rule.apply").map((c) => c.params)).toEqual([{ ids: [id] }]),
    );
    await waitFor(() => expect(mock.message(id)?.summary.mailboxIds).toEqual([FIXTURE.receipts]));
  });

  it("is not offered when no rule is on", async () => {
    const { menu } = await setup([{ enabled: false }]);
    expect(entries(menu)).not.toContain("Apply Rules");
    expect(entries(menu).at(-1)).toMatch(/^Mark as (Read|Unread)$/);
  });

  it("is disabled on a read-only account", async () => {
    const { menu } = await setup([{ enabled: true }], true);
    expect(applyItem(menu).getAttribute("aria-disabled")).toBe("true");
  });
});
