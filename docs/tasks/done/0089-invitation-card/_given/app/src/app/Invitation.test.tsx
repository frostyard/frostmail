// CONTRACT TEST for task card T-0089 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

async function setup(invitation = true) {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z"), invitation });
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  // The newest Inbox message with the subject (the list is virtual, so the
  // tests select by ID, as a click would).
  const idOf = (subject: string) => {
    const found = data.messages.filter(
      (m) => m.summary.subject === subject && m.summary.mailboxIds.includes(FIXTURE.inbox),
    );
    const last = found.at(-1);
    if (!last) throw new Error(`no ${subject}`);
    return last.summary.id;
  };
  return { mock, calls, idOf };
}

function openMessage(id: number) {
  act(() => useUI.getState().select([id], id));
}

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

describe("Invitation card", () => {
  it("shows an invitation above the message, without its .ics attachment", async () => {
    const { calls, idOf } = await setup();
    const id = idOf("Invitation: Lunch with Ann");
    openMessage(id);
    const card = await screen.findByRole("region", { name: "Invitation" });
    expect(within(card).getByText("Lunch with Ann")).toBeTruthy();
    expect(within(card).getByText("Cafe Nord · Ann Smith (organizer)")).toBeTruthy();
    expect(within(card).getByText("Before: Standup")).toBeTruthy();
    expect(calls("calendar.invitation")).toContainEqual({ messageId: id });
    await waitFor(() => expect(screen.queryByRole("button", { name: /invite\.ics/ })).toBeNull());
    expect(screen.queryByText("invite.ics")).toBeNull();
  });

  it("answers, and shows the answer", async () => {
    const { calls, idOf } = await setup();
    const id = idOf("Invitation: Lunch with Ann");
    openMessage(id);
    const card = await screen.findByRole("region", { name: "Invitation" });
    fireEvent.click(within(card).getByRole("button", { name: "Accept" }));
    await waitFor(() => expect(calls("calendar.respond")).toContainEqual({ messageId: id, answer: "accepted" }));
    await waitFor(() =>
      expect(
        within(screen.getByRole("region", { name: "Invitation" }))
          .getByRole("button", { name: "Accept" })
          .getAttribute("aria-pressed"),
      ).toBe("true"),
    );
    const before = calls("calendar.invitation").length;
    expect(before).toBeGreaterThanOrEqual(2);
  });

  it("opens the event in Calendar", async () => {
    const { idOf } = await setup();
    openMessage(idOf("Invitation: Lunch with Ann"));
    const card = await screen.findByRole("region", { name: "Invitation" });
    fireEvent.click(within(card).getByRole("button", { name: "Show in Calendar" }));
    await waitFor(() => expect(useUI.getState().module).toBe("calendar"));
    expect(useUI.getState().calendarDate).toBe("2026-10-09");
    expect(useUI.getState().calendarSelected).toEqual({ eventId: 303, recurrenceId: "" });
  });

  it("asks nothing for a message without an invitation", async () => {
    const { calls, idOf } = await setup(false);
    openMessage(idOf("Re: Offsite plan"));
    await screen.findAllByRole("article", { name: "Re: Offsite plan" });
    expect(screen.queryByRole("region", { name: "Invitation" })).toBeNull();
    expect(calls("calendar.invitation")).toEqual([]);
  });
});
