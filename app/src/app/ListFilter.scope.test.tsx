import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import type { ViewQuery } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { type MockData, MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

function setup(change: (data: MockData) => void = () => {}) {
  const data = mockData({ inbox: 30, now: new Date("2026-10-08T12:00:00Z") });
  change(data);
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

describe("an empty filtered list", () => {
  it("names its source and offers every flagged message", async () => {
    // Flagged mail only outside the inbox, as when it was archived.
    const { data, mock } = setup((data) => {
      for (const m of data.messages) {
        const inbox = m.summary.mailboxIds.includes(FIXTURE.inbox);
        if (inbox) m.summary.flags = { ...m.summary.flags, flagged: false, flagColor: 0 };
      }
      const archived = data.messages.find((m) => m.summary.mailboxIds.includes(FIXTURE.archive));
      if (!archived) throw new Error("no archived message");
      archived.summary.flags = { ...archived.summary.flags, flagged: true, flagColor: 1 };
    });
    const flagged = data.messages.filter((m) => m.summary.flags.flagged).map((m) => m.summary.id);
    expect(flagged.length).toBeGreaterThan(0);
    fireEvent.click(within(await bar()).getByRole("button", { name: "Flagged" }));
    await waitFor(() => expect(lastQuery(mock)).toEqual({ role: "inbox", threads: true, flagged: true }));
    expect(await screen.findByText("No Flagged Messages in All Inboxes")).toBeDefined();

    fireEvent.click(screen.getByRole("button", { name: "Show All Flagged" }));
    expect(useUI.getState().source).toEqual({ kind: "flagged" });
    expect(useUI.getState().listFilter).toBe("all");
    await waitFor(() => expect(lastQuery(mock)).toEqual({ flagged: true, threads: true }));
    await waitFor(() => expect(rows().length).toBeGreaterThan(0));
    for (const row of rows()) expect(flagged).toContain(Number(row.getAttribute("data-message-id")));
  });

  it("names no source while searching", async () => {
    setup();
    fireEvent.click(within(await bar()).getByRole("button", { name: "Unread" }));
    act(() => useUI.getState().commitSearch("zzzqqq"));
    expect(await screen.findByText("No Unread Messages")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Show All Flagged" })).toBeNull();
  });

  it("says no more than No Flagged Messages in Flagged", async () => {
    setup((data) => {
      for (const m of data.messages) m.summary.flags = { ...m.summary.flags, flagged: false, flagColor: 0 };
    });
    await bar();
    act(() => useUI.getState().setSource({ kind: "flagged" }));
    fireEvent.click(within(await bar()).getByRole("button", { name: "Flagged" }));
    expect(await screen.findByText("No Flagged Messages")).toBeDefined();
    expect(screen.getByRole("button", { name: "Show All" })).toBeDefined();
    expect(screen.queryByRole("button", { name: "Show All Flagged" })).toBeNull();
  });
});
