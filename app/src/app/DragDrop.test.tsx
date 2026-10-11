// CONTRACT TEST for task card T-0121 (docs/tasks). Do not edit.
import { createEvent, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

const TYPE = "application/x-frostmail-messages";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useMail.setState({ vips: [], settings: null, smarts: [], rules: [] });
});

/** transfer is a DataTransfer that keeps what dragstart puts in it. */
function transfer() {
  const data = new Map<string, string>();
  return {
    get types() {
      return [...data.keys()];
    },
    setData: vi.fn((type: string, value: string) => void data.set(type, value)),
    getData: (type: string) => data.get(type) ?? "",
    dropEffect: "none",
    effectAllowed: "all",
  };
}

async function setup(readOnly = false) {
  const data = mockData({ inbox: 10, now: new Date("2026-10-08T12:00:00Z") });
  for (const a of data.accounts) a.readOnly = readOnly;
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await waitFor(() => expect(rows().length).toBeGreaterThanOrEqual(3));
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { mock, calls };
}

const rows = () => within(screen.getByRole("listbox", { name: "Messages" })).queryAllByRole("option");

const target = (id: number) =>
  screen
    .getByRole("tree", { name: "Mailboxes" })
    .querySelector<HTMLElement>(`[data-key="mailbox:${id}"]`) as HTMLElement;

const idOf = (row: HTMLElement) => Number(row.getAttribute("data-message-id"));

/** dropAlt drops with Alt held, which the test DOM's DragEvent does not take in its init. */
function dropAlt(el: HTMLElement, dataTransfer: unknown) {
  const event = createEvent.drop(el, { dataTransfer });
  Object.defineProperty(event, "altKey", { value: true });
  fireEvent(el, event);
}

describe("dragging messages to a mailbox", () => {
  it("moves a dragged row", async () => {
    const { calls } = await setup();
    const row = rows()[0] as HTMLElement;
    const id = idOf(row);
    expect(row.getAttribute("draggable")).toBe("true");
    const dt = transfer();
    fireEvent.dragStart(row, { dataTransfer: dt });
    expect(JSON.parse(dt.getData(TYPE))).toEqual({ accountId: FIXTURE.accountId, ids: [id] });
    fireEvent.dragOver(target(FIXTURE.receipts), { dataTransfer: dt });
    fireEvent.drop(target(FIXTURE.receipts), { dataTransfer: dt });
    await waitFor(() => expect(calls("message.move")).toHaveLength(1));
    expect(calls("message.move")[0]).toMatchObject({ ids: [id], mailboxId: FIXTURE.receipts });
  });

  it("copies with Alt held", async () => {
    const { calls } = await setup();
    const row = rows()[1] as HTMLElement;
    const id = idOf(row);
    const dt = transfer();
    fireEvent.dragStart(row, { dataTransfer: dt });
    fireEvent.dragOver(target(FIXTURE.archive), { dataTransfer: dt });
    dropAlt(target(FIXTURE.archive), dt);
    await waitFor(() => expect(calls("message.copy")).toEqual([{ ids: [id], mailboxId: FIXTURE.archive }]));
    expect(calls("message.move")).toEqual([]);
  });

  it("drags the whole selection when the row is in it", async () => {
    const { calls } = await setup();
    const [a, b] = rows() as HTMLElement[];
    const ids = [idOf(a as HTMLElement), idOf(b as HTMLElement)].sort();
    fireEvent.click(a as HTMLElement);
    fireEvent.click(b as HTMLElement, { ctrlKey: true });
    await waitFor(() => expect(useUI.getState().selected).toHaveLength(2));
    const dt = transfer();
    fireEvent.dragStart(b as HTMLElement, { dataTransfer: dt });
    expect([...JSON.parse(dt.getData(TYPE)).ids].sort()).toEqual(ids);
    fireEvent.drop(target(FIXTURE.receipts), { dataTransfer: dt });
    await waitFor(() => expect(calls("message.move")).toHaveLength(1));
  });

  it("changes nothing on a read-only account", async () => {
    const { calls } = await setup(true);
    const dt = transfer();
    fireEvent.dragStart(rows()[0] as HTMLElement, { dataTransfer: dt });
    fireEvent.drop(target(FIXTURE.receipts), { dataTransfer: dt });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(calls("message.move")).toEqual([]);
  });
});
