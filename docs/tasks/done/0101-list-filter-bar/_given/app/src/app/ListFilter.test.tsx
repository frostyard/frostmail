// CONTRACT TEST for task card T-0101 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import type { ViewQuery } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

function setup() {
  const data = mockData({ inbox: 30, now: new Date("2026-10-08T12:00:00Z") });
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  return { data, mock };
}

function lastQuery(mock: MockTransport): ViewQuery | undefined {
  const opens = mock.calls.filter((c) => c.method === "view.open");
  return (opens.at(-1)?.params as { query: ViewQuery } | undefined)?.query;
}

async function bar() {
  return screen.findByRole("toolbar", { name: "Filter messages" });
}

function rows(): HTMLElement[] {
  return within(screen.getByRole("listbox", { name: "Messages" })).queryAllByRole("option");
}

describe("the list's filter bar", () => {
  it("sits above the list and narrows its view", async () => {
    const { mock } = setup();
    const filters = await bar();
    const list = screen.getByRole("listbox", { name: "Messages" });
    expect(filters.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await waitFor(() => expect(rows().length).toBeGreaterThan(0));

    fireEvent.click(within(filters).getByRole("button", { name: "Unread" }));
    await waitFor(() => expect(lastQuery(mock)).toEqual({ role: "inbox", threads: true, unread: true }));
    await waitFor(() => {
      expect(rows().length).toBeGreaterThan(0);
      for (const row of rows()) expect(within(row).queryByRole("img", { name: "Unread" })).not.toBeNull();
    });
    expect(within(filters).getByRole("button", { name: "Unread" }).getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(within(filters).getByRole("button", { name: "Attachments" }));
    await waitFor(() => expect(lastQuery(mock)).toEqual({ role: "inbox", threads: true, hasAttachments: true }));
    await waitFor(() => {
      expect(rows().length).toBeGreaterThan(0);
      for (const row of rows()) expect(within(row).queryByLabelText("Has attachments")).not.toBeNull();
    });
  });

  it("keeps a row read under the Unread filter", async () => {
    const { mock } = setup();
    fireEvent.click(within(await bar()).getByRole("button", { name: "Unread" }));
    await waitFor(() => expect(lastQuery(mock)?.unread).toBe(true));
    await waitFor(() => expect(rows().length).toBeGreaterThan(0));
    const first = rows()[0];
    if (!first) throw new Error("no row");
    const id = Number(first.getAttribute("data-message-id"));
    fireEvent.click(first);
    await waitFor(() => expect(mock.message(id)?.summary.flags.seen).toBe(true));
    // The row stays, now read.
    await waitFor(() => {
      const row = rows().find((r) => r.getAttribute("data-message-id") === String(id));
      expect(row).toBeDefined();
      expect(row && within(row).queryByRole("img", { name: "Unread" })).toBeNull();
    });
    expect(useUI.getState().selected).toEqual([id]);
  });

  it("says when nothing matches and offers to show all", async () => {
    const { data, mock } = setup();
    const quiet = data.mailboxes.find((mb) => {
      const msgs = data.messages.filter((m) => m.summary.mailboxIds.includes(mb.id));
      return msgs.length > 0 && msgs.every((m) => !m.summary.flags.flagged);
    });
    if (!quiet) throw new Error("no mailbox without flagged messages");
    await bar();
    act(() => useUI.getState().setSource({ kind: "mailbox", mailboxId: quiet.id }));
    fireEvent.click(within(await bar()).getByRole("button", { name: "Flagged" }));
    await waitFor(() => expect(lastQuery(mock)).toEqual({ mailboxId: quiet.id, threads: true, flagged: true }));
    expect(await screen.findByText("No Flagged Messages")).toBeDefined();

    fireEvent.click(screen.getByRole("button", { name: "Show All" }));
    expect(useUI.getState().listFilter).toBe("all");
    await waitFor(() => expect(lastQuery(mock)).toEqual({ mailboxId: quiet.id, threads: true }));
    await waitFor(() => expect(rows().length).toBeGreaterThan(0));
    expect(screen.queryByText("No Flagged Messages")).toBeNull();
    expect(FIXTURE.inbox).not.toBe(quiet.id);
  });
});
