// CONTRACT TEST for task card T-0116 (docs/tasks). Do not edit.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { atLocal } from "../lib/later";
import { Client, type OutboxItem } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { ComposeWindow } from "./ComposeWindow";

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 3, now: new Date("2026-10-07T12:00:00Z") }), { undoMs: 50 });
  const client = new Client(mock);
  const draft = await client.draft.create({ kind: "new" });
  await client.draft.update({
    id: draft.id,
    content: { ...draft.content, subject: "Later", to: [{ name: "Bob", address: "bob@x.test" }] },
  });
  const close = vi.spyOn(window, "close").mockImplementation(() => {});
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <ComposeWindow draftId={draft.id} fresh={false} />
    </Session>,
  );
  await screen.findByRole("toolbar", { name: "Compose" });
  await waitFor(() => expect((screen.getByRole("button", { name: "Send" }) as HTMLButtonElement).disabled).toBe(false));
  const sends = () => mock.calls.filter((c) => c.method === "draft.send").map((c) => c.params);
  return { mock, draft, close, sends };
}

afterEach(() => {
  vi.restoreAllMocks();
  document.title = "";
});

describe("Send Later in the compose window", () => {
  it("sends at a time from the menu", async () => {
    const user = userEvent.setup();
    const { draft, close, sends, mock } = await setup();
    await user.click(screen.getByRole("button", { name: "Send Later" }));
    const tomorrow = within(screen.getByRole("menu")).getByText(/^Send .+ Tomorrow$/);
    await user.click(tomorrow);
    await waitFor(() => expect(close).toHaveBeenCalled());
    const [params] = sends() as { id: number; sendAt: string }[];
    expect(params?.id).toBe(draft.id);
    expect(Date.parse(params?.sendAt ?? "")).toBeGreaterThan(Date.now());
    const [item] = await mock.call<OutboxItem[]>("outbox.list", {});
    expect(item?.scheduled).toBe(true);
  });

  it("sends at a time chosen in the time sheet", async () => {
    const user = userEvent.setup();
    const { draft, close, sends } = await setup();
    await user.click(screen.getByRole("button", { name: "Send Later" }));
    await user.click(within(screen.getByRole("menu")).getByText("Send Later…"));
    const sheet = screen.getByRole("dialog", { name: "Send Later" });
    const date = within(sheet).getByLabelText("Date");
    await user.clear(date);
    await user.type(date, "2030-01-02");
    const time = within(sheet).getByLabelText("Time");
    await user.clear(time);
    await user.type(time, "09:30");
    await user.click(within(sheet).getByRole("button", { name: "OK" }));
    await waitFor(() => expect(close).toHaveBeenCalled());
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    expect(sends()).toEqual([{ id: draft.id, sendAt: atLocal("2030-01-02", 9, 30, zone).toISOString() }]);
  });

  it("does nothing when the time sheet is cancelled", async () => {
    const user = userEvent.setup();
    const { close, sends } = await setup();
    await user.click(screen.getByRole("button", { name: "Send Later" }));
    await user.click(within(screen.getByRole("menu")).getByText("Send Later…"));
    await user.click(
      within(screen.getByRole("dialog", { name: "Send Later" })).getByRole("button", { name: "Cancel" }),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(sends()).toEqual([]);
    expect(close).not.toHaveBeenCalled();
  });
});
