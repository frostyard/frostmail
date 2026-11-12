// Reply entry points answer the message the window holds selected, with the
// whole message quoted (docs/specs/compose-ui.md, Entry points).
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { useFrameWindow } from "../lodge/frameWindow";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  const list = await screen.findByRole("listbox", { name: "Messages" });
  const rows = await waitFor(() => {
    const rows = within(list).queryAllByRole("option");
    if (rows.length < 2) throw new Error("no rows yet");
    return rows;
  });
  const row = rows[0];
  if (!row) throw new Error("missing row");
  fireEvent.click(row);
  const id = Number(row.getAttribute("data-message-id"));
  const calls = () => mock.calls.filter((call) => call.method === "draft.create").map((call) => call.params);
  return { row, id, calls };
}

describe("reply entry points", () => {
  it("toolbar Reply answers the selected message", async () => {
    const { id, calls } = await setup();
    fireEvent.click(screen.getByRole("button", { name: "Reply" }));
    await waitFor(() => expect(calls()).toEqual([{ kind: "reply", sourceId: id }]));
    expect(useFrameWindow.getState().open).toMatchObject({ kind: "compose", fresh: true });
  });

  it("Ctrl+R answers the selected message", async () => {
    const { id, calls } = await setup();
    fireEvent.keyDown(window, { key: "r", ctrlKey: true });
    await waitFor(() => expect(calls()).toEqual([{ kind: "reply", sourceId: id }]));
  });

  it("the list context menu Reply answers its row", async () => {
    const { row, id, calls } = await setup();
    fireEvent.contextMenu(row, { clientX: 50, clientY: 50 });
    const menu = await screen.findByRole("menu");
    const reply = within(menu)
      .getAllByRole("menuitem")
      .find((el) => el.querySelector(".flex-1")?.textContent === "Reply");
    if (!reply) throw new Error("missing Reply menu item");
    fireEvent.click(reply);
    await waitFor(() => expect(calls()).toEqual([{ kind: "reply", sourceId: id }]));
  });
});
