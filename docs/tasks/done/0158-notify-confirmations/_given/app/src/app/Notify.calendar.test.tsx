// CONTRACT TEST for task card T-0158 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { EventWindow } from "./EventWindow";
import { MainWindow } from "./MainWindow";
import { usePimUndo } from "./pimUndo";

// The fixture's week is October 4–10, 2026 (UTC): 302 Design review,
// Thursday 14:00–15:30, is the user's and invites two people. Here the
// writable calendars invite, as on Google or iCloud, so a change
// invitees would see waits for the notify sheet. The test DOM has no
// layout: every element is 100 wide from the page's top-left.
function inviting() {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
  for (const col of data.pim?.collections ?? []) col.invites = col.kind === "calendar" && !col.readOnly;
  return new MockTransport(data);
}

const day = (name: string) => screen.getByRole("group", { name });
const thursday = () => day("Thursday, October 8, 2026");
const review = () => within(thursday()).queryByRole("button", { name: /^Design review/ });
const eventPane = () => within(screen.getByRole("region", { name: "Event details" }));
const sheet = async (name: string) => within(await screen.findByRole("dialog", { name }));
const told = (mock: MockTransport, method: string) =>
  mock.calls.filter((c) => c.method === method && (c.params as { notify?: boolean }).notify === true);

async function mainWindow(mock: MockTransport) {
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  fireEvent.keyDown(window, { key: "2", ctrlKey: true });
  await screen.findByRole("group", { name: "Thursday, October 8, 2026" });
  await within(thursday()).findByRole("button", { name: /^Design review/ });
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
  useUI.setState({ calendarDate: "2026-10-08" });
  usePimUndo.setState(usePimUndo.getInitialState());
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    x: 0,
    y: 0,
    left: 0,
    top: 0,
    width: 100,
    height: 1152,
    right: 100,
    bottom: 1152,
    toJSON: () => ({}),
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("asking before invitees hear", () => {
  it("asks before a drop updates the invitees", async () => {
    const mock = inviting();
    await mainWindow(mock);
    const drag = () => {
      const block = review();
      if (!block) throw new Error("no Design review on Thursday");
      fireEvent.pointerDown(block, { button: 0, clientX: 500, clientY: 700, pointerId: 1 });
      fireEvent.pointerMove(window, { clientX: 500, clientY: 748, pointerId: 1 }); // an hour later
      fireEvent.pointerUp(window, { clientX: 500, clientY: 748, pointerId: 1 });
    };
    drag();
    const first = await sheet("Send Updates?");
    expect(first.getByText("2 invitees will get it by email.")).toBeTruthy();
    fireEvent.click(first.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(told(mock, "calendar.update")).toHaveLength(0);
    expect(usePimUndo.getState().calendar).toHaveLength(0);

    drag();
    fireEvent.click((await sheet("Send Updates?")).getByRole("button", { name: "Send" }));
    await waitFor(() =>
      expect(told(mock, "calendar.update").at(-1)?.params).toEqual({
        id: 302,
        fields: { allDay: false, start: "2026-10-08T15:00:00.000Z", end: "2026-10-08T16:30:00.000Z", timeZone: "UTC" },
        notify: true,
      }),
    );
    await waitFor(() => expect(usePimUndo.getState().calendar).toHaveLength(1));
  });

  it("asks before the pane's Delete cancels, and again before Ctrl+Z invites again", async () => {
    const mock = inviting();
    await mainWindow(mock);
    fireEvent.click(review() as HTMLElement);
    await eventPane().findByRole("heading", { name: /^Design review/ });
    fireEvent.click(eventPane().getByRole("button", { name: "Delete Event" }));
    const cancelling = await sheet("Send Cancellations?");
    expect(cancelling.getByText("2 invitees will get it by email.")).toBeTruthy();
    fireEvent.click(cancelling.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(told(mock, "calendar.delete").at(-1)?.params).toEqual({ id: 302, notify: true }));
    await waitFor(() => expect(review()).toBeNull());
    const restoreId = usePimUndo.getState().calendar.at(-1);
    expect(restoreId).toBeTruthy();

    fireEvent.keyDown(window, { key: "z", ctrlKey: true });
    fireEvent.click((await sheet("Send Invitations?")).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(told(mock, "calendar.restore")).toHaveLength(0);
    expect(usePimUndo.getState().calendar).toEqual([restoreId]);

    fireEvent.keyDown(window, { key: "z", ctrlKey: true });
    fireEvent.click((await sheet("Send Invitations?")).getByRole("button", { name: "Send" }));
    await waitFor(() => expect(told(mock, "calendar.restore").at(-1)?.params).toEqual({ restoreId, notify: true }));
    await within(thursday()).findByRole("button", { name: /^Design review/ });
    expect(usePimUndo.getState().calendar).toHaveLength(0);
  });

  it("does not ask when nobody is invited", async () => {
    const mock = inviting();
    await new Client(mock).calendar.create({
      fields: {
        summary: "Focus",
        allDay: false,
        start: "2026-10-08T09:00:00.000Z",
        end: "2026-10-08T10:00:00.000Z",
        timeZone: "UTC",
      },
    });
    await mainWindow(mock);
    fireEvent.click(await within(thursday()).findByRole("button", { name: /^Focus/ }));
    await eventPane().findByRole("heading", { name: /^Focus/ });
    fireEvent.click(eventPane().getByRole("button", { name: "Delete Event" }));
    await waitFor(() => expect(within(thursday()).queryByRole("button", { name: /^Focus/ })).toBeNull());
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("asks before the editor's Save and Delete Event, and stays open on Cancel", async () => {
    const mock = inviting();
    const close = vi.spyOn(window, "close").mockImplementation(() => {});
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <EventWindow target={{ kind: "edit", id: 302, recurrenceId: "" }} />
      </Session>,
    );
    const title = (await screen.findByLabelText("Title")) as HTMLInputElement;
    fireEvent.change(title, { target: { value: "Calendar review" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    fireEvent.click((await sheet("Send Updates?")).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(close).not.toHaveBeenCalled();
    expect(told(mock, "calendar.update")).toHaveLength(0);
    expect((screen.getByLabelText("Title") as HTMLInputElement).value).toBe("Calendar review");

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    fireEvent.click((await sheet("Send Updates?")).getByRole("button", { name: "Send" }));
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(told(mock, "calendar.update").at(-1)?.params).toEqual({
      id: 302,
      fields: { summary: "Calendar review" },
      notify: true,
    });
  });

  it("asks before the editor deletes an event with invitees", async () => {
    const mock = inviting();
    const close = vi.spyOn(window, "close").mockImplementation(() => {});
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <EventWindow target={{ kind: "edit", id: 302, recurrenceId: "" }} />
      </Session>,
    );
    await screen.findByLabelText("Title");
    fireEvent.click(screen.getByRole("button", { name: "Delete Event" }));
    fireEvent.click((await sheet("Send Cancellations?")).getByRole("button", { name: "Send" }));
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(told(mock, "calendar.delete").at(-1)?.params).toEqual({ id: 302, notify: true });
  });
});
