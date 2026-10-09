// CONTRACT TEST for task card T-0074 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

async function setup() {
  const data = mockData({ inbox: 8, now: new Date("2026-10-08T12:00:00Z") });
  const fromAnn = data.messages.find((m) => m.summary.from.address === "ann.smith@northwind.test");
  if (!fromAnn) throw new Error("fixture lacks Ann's message");
  render(
    <Session connect={() => Promise.resolve(new MockTransport(data))} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  return fromAnn.summary;
}

async function expectLunchInCalendar() {
  await waitFor(() => expect(useUI.getState().module).toBe("calendar"));
  expect(useUI.getState().calendarSelected).toEqual({ eventId: 303, recurrenceId: "" });
  expect(useUI.getState().calendarDate).toBe("2026-10-09");
  expect(await screen.findByRole("heading", { name: "Lunch with Ann" })).toBeTruthy();
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
});

describe("upcoming events", () => {
  it("opens one from the contact card in Calendar", async () => {
    const ann = await setup();
    act(() => useUI.getState().select([ann.id], ann.id));
    const names = await screen.findAllByRole("button", { name: ann.from.address });
    fireEvent.click(names[0] as HTMLElement);
    const card = await screen.findByRole("dialog", { name: "Ann Smith" });
    const upcoming = await within(card).findByRole("region", { name: "Upcoming" });
    fireEvent.click(within(upcoming).getByRole("button", { name: "Lunch with Ann, Fri" }));
    await expectLunchInCalendar();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("opens one from the person pane in Calendar", async () => {
    await setup();
    fireEvent.keyDown(window, { key: "3", ctrlKey: true });
    const contacts = await screen.findByRole("listbox", { name: "Contacts" });
    fireEvent.click(await within(contacts).findByRole("option", { name: /Ann Smith/ }));
    const upcoming = await screen.findByRole("region", { name: "Upcoming" });
    expect(screen.getByRole("region", { name: "Recent Mail" })).toBeTruthy();
    fireEvent.click(within(upcoming).getByRole("button", { name: "Lunch with Ann, Fri" }));
    await expectLunchInCalendar();
  });
});
