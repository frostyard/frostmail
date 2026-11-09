// The main window's search coverage line (docs/specs/ui.md, Search coverage;
// ADR-0016, the sync window).
import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { type MockData, MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  // The line names the window's first day, so the clock is fixed.
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-08-07T12:00:00Z"));
});

afterEach(() => {
  vi.useRealTimers();
});

async function setup(change: (data: MockData) => void = () => {}) {
  const data = mockData({ inbox: 5, now: new Date("2026-08-07T12:00:00Z") });
  change(data);
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("listbox", { name: "Messages" });
  return { data, mock };
}

function withWindow(days: number) {
  return (data: MockData) => {
    const account = data.accounts[0];
    if (!account) throw new Error("no account");
    account.syncDays = days;
  };
}

describe("the search coverage line", () => {
  it("says how far back the stored mail goes during a search", async () => {
    await setup(withWindow(365));
    expect(screen.queryByText(/Searched mail/)).toBeNull();
    act(() => useUI.getState().commitSearch("zzz"));
    expect(await screen.findByText("Searched mail since August 7, 2025 on this computer.")).toBeDefined();
  });

  it("is absent when the account keeps every message", async () => {
    await setup(withWindow(0));
    act(() => useUI.getState().commitSearch("zzz"));
    expect(await screen.findByText("Search:")).toBeDefined();
    expect(screen.queryByText(/Searched mail/)).toBeNull();
  });
});
