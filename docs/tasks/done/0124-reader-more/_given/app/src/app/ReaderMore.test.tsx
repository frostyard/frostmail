// CONTRACT TEST for task card T-0124 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import type { MessageSummary } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  const list = await screen.findByRole("listbox", { name: "Messages" });
  const row = await waitFor(() => {
    const r = within(list).queryAllByRole("option")[0];
    if (!r) throw new Error("no rows yet");
    return r;
  });
  const id = Number(row.getAttribute("data-message-id"));
  const [summary] = await mock.call<MessageSummary[]>("message.summaries", { ids: [id] });
  fireEvent.click(row);
  const reader = screen.getByRole("region", { name: "Message" });
  await waitFor(() =>
    expect(within(reader).getAllByRole("button", { name: "More Actions" }).length).toBeGreaterThan(0),
  );
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { mock, id, summary, reader, calls };
}

function more(reader: HTMLElement): HTMLElement {
  fireEvent.click(within(reader).getAllByRole("button", { name: "More Actions" })[0] as HTMLElement);
  return screen.getByRole("menu");
}

/** entries lists a menu's rows: labels, and "—" for separators. */
function entries(menu: HTMLElement): string[] {
  return Array.from(menu.children)
    .filter((el) => el.tagName === "HR" || el.tagName === "BUTTON")
    .map((el) => (el.tagName === "HR" ? "—" : (el.querySelector(".flex-1")?.textContent ?? "")));
}

describe("the reader's More Actions", () => {
  it("shows and hides all headers", async () => {
    const { reader, summary, calls, id } = await setup();
    const menu = more(reader);
    expect(entries(menu)).toEqual(["Show All Headers", "Raw Source…", "—", "Save As…", "Print…"]);
    fireEvent.click(within(menu).getByText("Show All Headers"));
    const block = await within(reader).findByLabelText("All Headers");
    expect(block.textContent).toContain(`Subject: ${summary?.subject}`);
    expect(calls("message.source")).toEqual([{ id }]);
    const again = more(reader);
    expect(entries(again)[0]).toBe("Hide All Headers");
    fireEvent.click(within(again).getByText("Hide All Headers"));
    await waitFor(() => expect(within(reader).queryByLabelText("All Headers")).toBeNull());
  });

  it("shows the raw source in a sheet", async () => {
    const { reader, summary } = await setup();
    fireEvent.click(within(more(reader)).getByText("Raw Source…"));
    const sheet = await screen.findByRole("dialog", { name: "Raw Source" });
    expect(sheet.querySelector("pre")?.textContent).toContain(`Subject: ${summary?.subject}`);
    fireEvent.click(within(sheet).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("prints, and saves nothing outside the app", async () => {
    const { reader, calls } = await setup();
    const print = vi.fn();
    vi.stubGlobal("print", print);
    fireEvent.click(within(more(reader)).getByText("Print…"));
    await waitFor(() => expect(print).toHaveBeenCalledTimes(1));
    fireEvent.click(within(more(reader)).getByText("Save As…"));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(calls("message.save")).toEqual([]);
    expect(reader.hasAttribute("data-print-area")).toBe(true);
  });
});
