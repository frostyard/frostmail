// CONTRACT TEST for task card T-0102 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

function setup(readOnly = false) {
  const data = mockData({ inbox: 20, now: new Date("2026-10-08T12:00:00Z") });
  for (const a of data.accounts) a.readOnly = readOnly;
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  return { data, mock };
}

async function firstRow(): Promise<HTMLElement> {
  const list = await screen.findByRole("listbox", { name: "Messages" });
  return waitFor(() => {
    const row = within(list).queryAllByRole("option")[0];
    if (!row) throw new Error("no rows yet");
    return row;
  });
}

function calls(mock: MockTransport, method: string) {
  return mock.calls.filter((c) => c.method === method).map((c) => c.params);
}

describe("row actions in the message list", () => {
  // The actions are a mouse-only duplicate of the row's keyboard commands.
  const action = (row: HTMLElement, name: string) => {
    const found = row.querySelector<HTMLElement>(`[data-action="${name}"]`);
    if (!found) throw new Error(`no ${name} action`);
    return found;
  };

  it("archive, flag and delete the row's message alone", async () => {
    const { mock } = setup();
    const row = await firstRow();
    const id = Number(row.getAttribute("data-message-id"));
    const flagged = mock.message(id)?.summary.flags.flagged ?? false;

    fireEvent.click(action(row, "flag"));
    await waitFor(() =>
      expect(calls(mock, "message.setFlags")).toEqual([{ ids: [id], changes: { flagColor: flagged ? 0 : 1 } }]),
    );

    fireEvent.click(action(row, "archive"));
    await waitFor(() => expect(calls(mock, "message.move")).toEqual([{ ids: [id], mailboxId: FIXTURE.archive }]));
    expect(useUI.getState().selected).toEqual([]);

    const next = await firstRow();
    const nextId = Number(next.getAttribute("data-message-id"));
    expect(nextId).not.toBe(id);
    fireEvent.click(action(next, "delete"));
    await waitFor(() => expect(calls(mock, "message.delete")).toEqual([{ ids: [nextId] }]));
    expect(useUI.getState().selected).toEqual([]);
  });

  it("are absent on a read-only account's rows", async () => {
    setup(true);
    const row = await firstRow();
    expect(row.querySelector("[data-action]")).toBeNull();
  });
});
