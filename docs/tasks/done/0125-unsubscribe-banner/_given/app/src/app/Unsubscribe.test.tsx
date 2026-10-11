// CONTRACT TEST for task card T-0125 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import type { UnsubscribeMethod } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

afterEach(() => {
  vi.restoreAllMocks();
});

/** setup shows the first row's message, from a list offering methods. */
async function setup(methods: UnsubscribeMethod[], { readOnly = false, oneClickFails = false } = {}) {
  const data = mockData({ inbox: 20, now: new Date("2026-10-08T12:00:00Z") });
  for (const a of data.accounts) a.readOnly = readOnly;
  const mock = new MockTransport(data);
  mock.oneClickFails = oneClickFails;
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
  const m = mock.message(id);
  if (!m) throw new Error("no message");
  if (methods.length > 0)
    m.unsubscribe = {
      methods,
      list: "Weekly",
      host: "list.example",
      address: "leave@list.example",
      url: "https://list.example/u/1",
    };
  fireEvent.click(row);
  const calls = () => mock.calls.filter((c) => c.method === "message.unsubscribe").map((c) => c.params);
  return { mock, id, calls };
}

async function confirm() {
  fireEvent.click(await screen.findByRole("button", { name: "Unsubscribe" }));
  const dialog = screen.getByRole("dialog", { name: "Unsubscribe from Weekly?" });
  fireEvent.click(within(dialog).getByRole("button", { name: "Unsubscribe" }));
  return dialog;
}

describe("the reader's unsubscribe banner", () => {
  it("unsubscribes with one click after asking, then says so", async () => {
    const { id, calls } = await setup(["oneclick", "mail", "web"]);
    await screen.findByText("This message is from the mailing list Weekly.");
    await confirm();
    await screen.findByText("You unsubscribed from Weekly.");
    expect(calls()).toEqual([{ id, method: "oneclick" }]);
    expect(screen.queryByRole("button", { name: "Unsubscribe" })).toBeNull();
  });

  it("falls back to mail when one click fails", async () => {
    const { id, calls } = await setup(["oneclick", "mail"], { oneClickFails: true });
    await confirm();
    await screen.findByText("You unsubscribed from Weekly.");
    expect(calls()).toEqual([
      { id, method: "oneclick" },
      { id, method: "mail" },
    ]);
  });

  it("skips mail on a read-only account and opens the page", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const { id, calls } = await setup(["mail", "web"], { readOnly: true });
    fireEvent.click(await screen.findByRole("button", { name: "Unsubscribe" }));
    expect(screen.getByRole("dialog").textContent).toContain(
      "The list's page on list.example will open in your browser.",
    );
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Unsubscribe" }));
    await screen.findByText("You unsubscribed from Weekly.");
    expect(calls()).toEqual([{ id, method: "web" }]);
    expect(open).toHaveBeenCalledWith("https://list.example/u/1", "_blank", "noopener,noreferrer");
  });

  it("is disabled when only mail is offered on a read-only account", async () => {
    await setup(["mail"], { readOnly: true });
    await screen.findByText("This message is from the mailing list Weekly.");
    expect((screen.getByRole("button", { name: "Unsubscribe" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("says when every method failed, and can try again", async () => {
    const { mock, calls } = await setup(["oneclick"], { oneClickFails: true });
    await confirm();
    await screen.findByText("This message is from the mailing list Weekly. Couldn't unsubscribe.");
    mock.oneClickFails = false;
    await confirm();
    await screen.findByText("You unsubscribed from Weekly.");
    expect(calls()).toHaveLength(2);
  });

  it("does not ask maild about a message from no list", async () => {
    const { mock } = await setup([]);
    await waitFor(() => expect(mock.calls.some((c) => c.method === "message.render")).toBe(true));
    await waitFor(() => expect(mock.calls.some((c) => c.method === "message.get")).toBe(true));
    expect(mock.calls.filter((c) => c.method === "message.unsubscribeInfo")).toEqual([]);
    expect(screen.queryByText(/mailing list/)).toBeNull();
  });
});
