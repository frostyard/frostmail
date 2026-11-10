// Reply entry points must preserve reader selections, except the list's row menu.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

const bodies: HTMLElement[] = [];

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  vi.spyOn(window, "open").mockReturnValue(null);
});

afterEach(() => {
  document.getSelection()?.removeAllRanges();
  for (const body of bodies.splice(0)) body.remove();
  vi.restoreAllMocks();
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
  const other = rows[1];
  if (!row || !other) throw new Error("missing rows");
  fireEvent.click(row);
  const id = Number(row.getAttribute("data-message-id"));
  const otherId = Number(other.getAttribute("data-message-id"));
  const calls = () => mock.calls.filter((call) => call.method === "draft.create").map((call) => call.params);
  return { row, id, otherId, calls };
}

function frame(id: number, text: string): HTMLIFrameElement {
  const el = document.createElement("iframe");
  el.srcdoc = "<p></p>";
  el.dataset.quoteSource = String(id);
  el.tabIndex = 0;
  document.body.append(el);
  // Control the selection independently of happy-dom's frame document.
  Object.defineProperty(el, "contentWindow", { value: { getSelection: () => ({ toString: () => text }) } });
  bodies.push(el);
  return el;
}

function plain(id: number, text: string): void {
  const el = document.createElement("div");
  el.dataset.quoteSource = String(id);
  el.textContent = text;
  document.body.append(el);
  bodies.push(el);
  const range = document.createRange();
  range.selectNodeContents(el);
  document.getSelection()?.removeAllRanges();
  document.getSelection()?.addRange(range);
}

describe("reply selection entry points", () => {
  it("toolbar Reply answers frame B when it had focus before the press, not older frame A", async () => {
    const { id, otherId, calls } = await setup();
    frame(id, "older frame A selection");
    const b = frame(otherId, "frame B selection");
    b.focus();
    expect(document.activeElement).toBe(b);
    const reply = screen.getByRole("button", { name: "Reply" });
    fireEvent.pointerDown(reply);
    // fireEvent does not perform the browser's default pointer-down focus change.
    reply.focus();
    expect(document.activeElement).toBe(reply);
    fireEvent.click(reply);
    await waitFor(() => expect(calls()).toEqual([{ kind: "reply", sourceId: otherId, quote: "frame B selection" }]));
  });

  it("Ctrl+R answers the message that holds the reader's plain-text selection", async () => {
    const { otherId, calls } = await setup();
    plain(otherId, "keyboard selection");
    fireEvent.keyDown(window, { key: "r", ctrlKey: true });
    await waitFor(() => expect(calls()).toEqual([{ kind: "reply", sourceId: otherId, quote: "keyboard selection" }]));
  });

  it("the list context menu Reply answers its row and sends no quote", async () => {
    const { row, id, otherId, calls } = await setup();
    const selected = frame(otherId, "reader selection must not enter this draft");
    selected.focus();
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
