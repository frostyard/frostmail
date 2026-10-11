// CONTRACT TEST for task card T-0128 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
});

const banner = "This message is from a blocked sender.";

async function setup() {
  const data = mockData({ inbox: 8, now: new Date("2026-10-08T12:00:00Z") });
  const ann = data.messages.find((m) => m.summary.from.address === "ann.smith@northwind.test")?.summary;
  if (!ann) throw new Error("fixture lacks Ann's message");
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  act(() => useUI.getState().select([ann.id], ann.id));
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { mock, ann, calls };
}

async function openCard(address: string): Promise<HTMLElement> {
  const buttons = await screen.findAllByRole("button", { name: address });
  fireEvent.click(buttons[0] as HTMLElement);
  return screen.findByRole("dialog");
}

describe("blocking a sender", () => {
  it("blocks from the contact card, marks the mail, and unblocks from the banner", async () => {
    const { ann, calls } = await setup();
    const card = await openCard(ann.from.address);
    fireEvent.click(within(card).getByRole("button", { name: "Block Contact" }));
    await waitFor(() => expect(calls("block.add")).toEqual([{ addresses: [ann.from.address] }]));
    await screen.findAllByText(banner);
    await waitFor(() => expect(within(card).getByRole("button", { name: "Unblock Contact" })).toBeTruthy());
    expect(useMail.getState().blocked.map((b) => b.address)).toEqual([ann.from.address]);
    const status = screen.getAllByText(banner)[0]?.closest('[role="status"]') as HTMLElement;
    fireEvent.click(within(status).getByRole("button", { name: "Unblock" }));
    await waitFor(() => expect(calls("block.remove")).toEqual([{ addresses: [ann.from.address] }]));
    await waitFor(() => expect(screen.queryByText(banner)).toBeNull());
  });

  it("marks mail from an address blocked elsewhere, whatever its case", async () => {
    const { mock, ann } = await setup();
    await screen.findAllByRole("button", { name: ann.from.address });
    expect(screen.queryByText(banner)).toBeNull();
    await act(() => mock.call("block.add", { addresses: [ann.from.address.toUpperCase()] }));
    await screen.findAllByText(banner);
  });
});
