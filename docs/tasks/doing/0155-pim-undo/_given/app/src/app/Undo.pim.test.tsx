// CONTRACT TEST for task card T-0155 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";
import { announceRestore, usePimUndo } from "./pimUndo";

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { mock, calls };
}

const ctrl = (key: string, more: { altKey?: boolean } = {}) =>
  fireEvent.keyDown(window, { key, ctrlKey: true, ...more });
const day = (name: string) => screen.getByRole("group", { name });
const eventPane = () => within(screen.getByRole("region", { name: "Event details" }));
const contactPane = () => screen.getByRole("region", { name: "Contact details" });

async function showCalendar() {
  ctrl("2");
  await screen.findByRole("group", { name: "Thursday, October 8, 2026" });
  await within(day("Thursday, October 8, 2026")).findByRole("button", { name: /^Design review/ });
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
  useUI.setState({ calendarDate: "2026-10-08" });
  usePimUndo.setState(usePimUndo.getInitialState());
});

describe("undo in Calendar and People", () => {
  it("brings a deleted event back with Ctrl+Z", async () => {
    const { calls } = await setup();
    await showCalendar();
    fireEvent.click(within(day("Thursday, October 8, 2026")).getByRole("button", { name: /^Design review/ }));
    await eventPane().findByRole("heading", { name: /^Design review/ });
    fireEvent.click(eventPane().getByRole("button", { name: "Delete Event" }));
    await waitFor(() =>
      expect(within(day("Thursday, October 8, 2026")).queryByRole("button", { name: /^Design review/ })).toBeNull(),
    );
    const restoreId = usePimUndo.getState().calendar.at(-1);
    expect(restoreId).toBeTruthy();
    ctrl("z");
    await waitFor(() => expect(calls("calendar.restore").at(-1)?.params).toEqual({ restoreId }));
    await within(day("Thursday, October 8, 2026")).findByRole("button", { name: /^Design review/ });
    expect(usePimUndo.getState().calendar).toHaveLength(0);
    ctrl("z"); // nothing left to undo
    await Promise.resolve();
    expect(calls("calendar.restore")).toHaveLength(1);
  });

  it("undoes a write the event editor window made", async () => {
    const { mock, calls } = await setup();
    await showCalendar();
    // Another window's write, announced as the editor does.
    const w = await new Client(mock).calendar.update({ id: 302, fields: { summary: "Review moved" } });
    await announceRestore({ module: "calendar", restoreId: w.restoreId });
    await waitFor(() => expect(usePimUndo.getState().calendar).toEqual([w.restoreId]));
    ctrl("z");
    await waitFor(() => expect(calls("calendar.restore").at(-1)?.params).toEqual({ restoreId: w.restoreId }));
  });

  it("brings a contact's earlier fields back, and keeps People's list apart from Calendar's", async () => {
    const { calls } = await setup();
    ctrl("3");
    await screen.findByRole("tree", { name: "Address Books" });
    fireEvent.click(
      await within(screen.getByRole("listbox", { name: "Contacts" })).findByRole("option", { name: /Ann Smith/ }),
    );
    await within(contactPane()).findByRole("heading", { name: "Ann Smith", level: 2 });
    fireEvent.click(within(contactPane()).getByRole("button", { name: "Edit Contact" }));
    fireEvent.change(within(contactPane()).getByRole("textbox", { name: "Company" }), {
      target: { value: "Northwind Traders" },
    });
    fireEvent.click(within(contactPane()).getByRole("button", { name: "Done" }));
    await within(contactPane()).findByText("Head of Operations · Northwind Traders");
    expect(usePimUndo.getState().people).toHaveLength(1);
    expect(usePimUndo.getState().calendar).toHaveLength(0);

    ctrl("z");
    await waitFor(() => expect(calls("people.restore")).toHaveLength(1));
    await within(contactPane()).findByText("Head of Operations · Northwind");
    expect(calls("calendar.restore")).toHaveLength(0);
  });

  it("keeps a deleted contact for undo", async () => {
    vi.stubGlobal(
      "confirm",
      vi.fn((_message?: string) => true),
    );
    const { calls } = await setup();
    ctrl("3");
    await screen.findByRole("tree", { name: "Address Books" });
    fireEvent.click(
      await within(screen.getByRole("listbox", { name: "Contacts" })).findByRole("option", { name: /Bob Okafor/ }),
    );
    await within(contactPane()).findByRole("heading", { name: "Bob Okafor", level: 2 });
    fireEvent.click(within(contactPane()).getByRole("button", { name: "Edit Contact" }));
    fireEvent.click(within(contactPane()).getByRole("button", { name: "Delete Contact" }));
    await waitFor(() => expect(calls("people.delete")).toHaveLength(1));
    await waitFor(() => expect(usePimUndo.getState().people).toHaveLength(1));
    ctrl("z");
    await waitFor(() => expect(calls("people.restore")).toHaveLength(1));
    await within(screen.getByRole("listbox", { name: "Contacts" })).findByRole("option", { name: /Bob Okafor/ });
    vi.unstubAllGlobals();
  });
});
