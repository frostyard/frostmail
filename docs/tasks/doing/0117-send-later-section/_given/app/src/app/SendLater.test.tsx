// CONTRACT TEST for task card T-0117 (docs/tasks). Do not edit.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import { atLocal } from "../lib/later";
import { Client } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useMail.setState({ outbox: [] });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 3, now: new Date("2026-10-08T12:00:00Z") }), { undoMs: 60_000 });
  const client = new Client(mock);
  const d = await client.draft.create({ kind: "new", accountId: FIXTURE.accountId });
  await client.draft.update({
    id: d.id,
    content: { ...d.content, subject: "Later", to: [{ name: "Bob", address: "bob@x.test" }] },
  });
  const at = new Date(Date.now() + 2 * 86_400_000);
  const item = await client.draft.send({ id: d.id, sendAt: at.toISOString() });
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  const section = await screen.findByRole("region", { name: "Send Later" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { mock, item, draft: d, section, calls };
}

describe("Send Later in the main window", () => {
  it("waits in its own section, with no undo toast", async () => {
    const { section } = await setup();
    expect(within(section).getByText("Later")).toBeTruthy();
    expect(within(section).getByText(/^Sends /)).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Outbox" })).toBeNull();
    expect(screen.queryByText("Sending “Later”…")).toBeNull();
  });

  it("changes the time in the time sheet", async () => {
    const user = userEvent.setup();
    const { section, item, calls } = await setup();
    await user.click(within(section).getByRole("button", { name: "Change Time…" }));
    const sheet = screen.getByRole("dialog", { name: "Send Later" });
    const date = within(sheet).getByLabelText("Date");
    await user.clear(date);
    await user.type(date, "2030-01-02");
    const time = within(sheet).getByLabelText("Time");
    await user.clear(time);
    await user.type(time, "10:00");
    await user.click(within(sheet).getByRole("button", { name: "OK" }));
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    await waitFor(() =>
      expect(calls("outbox.reschedule")).toEqual([
        { id: item.id, sendAt: atLocal("2030-01-02", 10, 0, zone).toISOString() },
      ]),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("sends now: the message leaves the section for its undo window", async () => {
    const user = userEvent.setup();
    const { section, item, calls } = await setup();
    const before = Date.now();
    await user.click(within(section).getByRole("button", { name: "Send Now" }));
    await waitFor(() => expect(calls("outbox.reschedule")).toHaveLength(1));
    const [params] = calls("outbox.reschedule") as { id: number; sendAt: string }[];
    expect(params?.id).toBe(item.id);
    expect(Math.abs(Date.parse(params?.sendAt ?? "") - before)).toBeLessThan(10_000);
    await waitFor(() => expect(screen.queryByRole("region", { name: "Send Later" })).toBeNull());
    expect(await screen.findByText("Sending “Later”…")).toBeTruthy();
  });

  it("edits: the message comes back as its draft", async () => {
    const user = userEvent.setup();
    const open = vi.fn();
    vi.stubGlobal("open", open);
    const { section, item, draft, calls } = await setup();
    await user.click(within(section).getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(calls("outbox.cancel")).toEqual([{ id: item.id }]));
    await waitFor(() => expect(open).toHaveBeenCalled());
    expect(String(open.mock.calls[0]?.[0])).toContain(`#/compose/${draft.id}`);
  });
});
