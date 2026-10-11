// CONTRACT TEST for task card T-0129 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

beforeEach(() => {
  useMail.setState({ vips: [], blocked: [], settings: null });
});

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 1, now: new Date("2026-10-07T12:00:00Z") }));
  await mock.call("block.add", { addresses: ["spam@y.test"] });
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <SettingsWindow />
    </Session>,
  );
  await screen.findByRole("navigation", { name: "Accounts" });
  fireEvent.click(screen.getByRole("button", { name: "General" }));
  return mock;
}

const calls = (mock: MockTransport, method: string) =>
  mock.calls.filter((c) => c.method === method).map((c) => c.params);

describe("the General pane's blocked senders", () => {
  it("sets the block action, lists blocked senders and unblocks them", async () => {
    const mock = await setup();
    const action = (await screen.findByLabelText("Blocked mail:")) as HTMLSelectElement;
    await waitFor(() => expect(action.disabled).toBe(false));
    fireEvent.change(action, { target: { value: "trash" } });
    await waitFor(() => expect(calls(mock, "settings.set")).toContainEqual({ blockAction: "trash" }));
    await waitFor(() => expect((screen.getByLabelText("Blocked mail:") as HTMLSelectElement).value).toBe("trash"));

    const list = await screen.findByRole("list", { name: "Blocked senders" });
    await act(() => mock.call("block.add", { addresses: ["deals@shop.test"] }));
    await waitFor(() => expect(within(list).getAllByRole("listitem")).toHaveLength(2));
    fireEvent.click(within(list).getByRole("button", { name: "Unblock spam@y.test" }));
    await waitFor(() => expect(calls(mock, "block.remove")).toEqual([{ addresses: ["spam@y.test"] }]));
    await waitFor(() => expect(within(list).getAllByRole("listitem")).toHaveLength(1));
  });
});
