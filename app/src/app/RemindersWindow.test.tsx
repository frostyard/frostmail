// CONTRACT TEST for task card T-0076 (docs/tasks). Do not edit.
// Amended by the lodge-only change: closing the window goes back to the main
// window in the frame (lodge/frameWindow.ts), not window.close.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { openFrameWindow } from "../lodge/frameWindow";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { closeWatch } from "../testing/frame";
import { RemindersWindow } from "./RemindersWindow";

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z"), reminders: true }));
  openFrameWindow({ kind: "reminders" });
  const close = closeWatch();
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <RemindersWindow />
    </Session>,
  );
  const list = await screen.findByRole("list", { name: "Reminders" });
  await waitFor(() => expect(within(list).getAllByRole("listitem")).toHaveLength(2));
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { mock, close, calls, list };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("reminder window", () => {
  it("lists due reminders, dismisses them and closes when none are left", async () => {
    const { close, calls, list } = await setup();
    const [first] = within(list).getAllByRole("listitem");
    fireEvent.click(within(first as HTMLElement).getByRole("button", { name: "Dismiss" }));
    await waitFor(() => expect(within(list).getAllByRole("listitem")).toHaveLength(1));
    expect(calls("calendar.dismiss")).toHaveLength(1);
    expect(close).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    await waitFor(() => expect(close).toHaveBeenCalled());
  });

  it("snoozes a reminder", async () => {
    const { calls, list } = await setup();
    const [first] = within(list).getAllByRole("listitem");
    fireEvent.click(within(first as HTMLElement).getByRole("button", { name: "Snooze" }));
    fireEvent.click(within(screen.getByRole("menu")).getByRole("menuitem", { name: "10 minutes" }));
    await waitFor(() => expect(within(list).getAllByRole("listitem")).toHaveLength(1));
    const params = calls("calendar.snooze").at(-1)?.params as { ids: string[]; until: string };
    expect(params.ids).toHaveLength(1);
    expect(Date.parse(params.until)).toBeGreaterThan(Date.now() + 9 * 60_000);
  });

  it("closes on Escape and from its title strip", async () => {
    const { close } = await setup();
    fireEvent.keyDown(window, { key: "Escape" });
    expect(close).toHaveBeenCalledTimes(1);
    // The window in the frame again, as the main window raises it.
    openFrameWindow({ kind: "reminders" });
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(close).toHaveBeenCalledTimes(2);
  });
});
