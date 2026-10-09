// CONTRACT TEST for task card T-0076 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { RemindersWindow } from "./RemindersWindow";

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z"), reminders: true }));
  const close = vi.spyOn(window, "close").mockImplementation(() => {});
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
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(close).toHaveBeenCalledTimes(2);
  });
});
