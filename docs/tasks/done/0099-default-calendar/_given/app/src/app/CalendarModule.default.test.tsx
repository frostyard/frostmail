// CONTRACT TEST for task card T-0099 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { CALENDARS, mockData } from "../rpc/mock/fixture";
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
  fireEvent.keyDown(window, { key: "2", ctrlKey: true });
  await screen.findByRole("group", { name: "Thursday, October 8, 2026" });
  return { mock, calls: (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params) };
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
  useUI.setState({ calendarDate: "2026-10-08" });
});

const calendars = () => screen.getByRole("group", { name: "Calendars" });
const item = () => screen.getByRole("menuitemcheckbox", { name: "Use as Default Calendar" });

// checked opens a calendar's menu, reads its item, and closes it again: a
// menu keeps the items it opened with.
function checked(name: RegExp): string | null {
  fireEvent.contextMenu(within(calendars()).getByRole("checkbox", { name }));
  try {
    return item().getAttribute("aria-checked");
  } finally {
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
  }
}

describe("choosing the default calendar", () => {
  it("makes Home the default and shows it checked", async () => {
    const { calls } = await setup();
    const home = await within(calendars()).findByRole("checkbox", { name: /^Home/ });
    fireEvent.contextMenu(home);
    expect(item().getAttribute("aria-checked")).toBe("false");
    const loads = calls("account.collections").length;
    fireEvent.click(item());
    await waitFor(() => expect(calls("account.setCollection")).toEqual([{ id: CALENDARS.home, isDefault: true }]));
    // account.changed reloads the calendars; let the answer render.
    await waitFor(() => expect(calls("account.collections").length).toBeGreaterThan(loads));
    await act(async () => {});
    expect(checked(/^Home/)).toBe("true");
    expect(checked(/^Work/)).toBe("false");
    expect(screen.queryByRole("menu")).toBeNull();
  });
});
