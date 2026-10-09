// CONTRACT TEST for task card T-0065 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

const now = new Date("2026-10-08T12:00:00Z");

async function setup() {
  const data = mockData({ inbox: 8, now });
  const known = new Set(
    (data.pim?.people ?? []).flatMap((p) => p.contacts.flatMap((c) => c.emails.map((e) => e.value))),
  );
  const fromAnn = data.messages.find((m) => m.summary.from.address === "ann.smith@northwind.test");
  const fromStranger = data.messages.find(
    (m) => !known.has(m.summary.from.address) && m.summary.from.address !== "test1@mailtest.test",
  );
  if (!fromAnn || !fromStranger) throw new Error("fixture lacks the messages");
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  const open = async (id: number, address: string) => {
    act(() => useUI.getState().select([id], id));
    const buttons = await screen.findAllByRole("button", { name: address });
    fireEvent.click(buttons[0] as HTMLElement);
    return screen.findByRole("dialog");
  };
  return { calls, open, ann: fromAnn.summary, stranger: fromStranger.summary };
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
});

describe("contact card", () => {
  it("shows a known person and opens them in People", async () => {
    const { calls, open, ann } = await setup();
    await open(ann.id, ann.from.address);
    await waitFor(() => expect(screen.getByRole("dialog", { name: "Ann Smith" })).toBeTruthy());
    expect(calls("people.card").at(-1)?.params).toEqual({ email: "ann.smith@northwind.test" });
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Open in People" }));
    await screen.findByRole("tree", { name: "Address Books" });
    expect(useUI.getState().module).toBe("people");
    expect(await screen.findByRole("heading", { level: 2, name: "Ann Smith" })).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("writes to the address", async () => {
    const { calls, open, ann } = await setup();
    await open(ann.id, ann.from.address);
    await waitFor(() => expect(screen.getByRole("dialog", { name: "Ann Smith" })).toBeTruthy());
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Message" }));
    await waitFor(() => expect(calls("draft.create")).toHaveLength(1));
    expect(calls("draft.create")[0]?.params).toMatchObject({
      kind: "new",
      to: [{ address: "ann.smith@northwind.test" }],
    });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("adds a stranger to contacts", async () => {
    const { calls, open, stranger } = await setup();
    const dialog = await open(stranger.id, stranger.from.address);
    const add = await within(dialog).findByRole("button", { name: "Add to Contacts" });
    fireEvent.click(add);
    await within(screen.getByRole("dialog")).findByText("Added");
    expect(calls("people.add")[0]?.params).toMatchObject({ email: stranger.from.address, name: stranger.from.name });
    expect(within(screen.getByRole("dialog")).getByRole("button", { name: "Open in People" })).toBeTruthy();
  });

  it("closes with Escape", async () => {
    const { open, ann } = await setup();
    await open(ann.id, ann.from.address);
    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});
